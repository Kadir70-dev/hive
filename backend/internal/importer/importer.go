// Package importer moves members (and optionally gatherings and bookings)
// from offline exports of the old Supabase-based site into PostgreSQL.
//
// Safety properties:
//   - Inputs are CSV files the operator exported and provided explicitly; the
//     live Supabase project is never contacted.
//   - Plan (dry run) executes exactly the same code as Apply inside a
//     transaction that is always rolled back, so the report is exact and
//     nothing is written.
//   - Imports are idempotent: legacy ids are recorded in legacy_*_map tables
//     and re-running skips anything already imported.
//   - Old password hashes are never imported. Imported accounts have an
//     unusable password until the member follows an invitation (a
//     password-reset link sent by Invite).
//   - Reports contain counts, row numbers and reason codes only — never names,
//     emails or phone numbers.
package importer

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/tokens"
)

// Issue points at a problem row without repeating its contents.
type Issue struct {
	File string `json:"file"`
	Row  int    `json:"row"`
	Code string `json:"code"`
}

// Report summarises a plan or an import.
type Report struct {
	Mode       string         `json:"mode"` // "dry-run" or "applied"
	Members    map[string]int `json:"members"`
	Gatherings map[string]int `json:"gatherings"`
	Bookings   map[string]int `json:"bookings"`
	Errors     []Issue        `json:"errors"`   // rows skipped
	Warnings   []Issue        `json:"warnings"` // rows imported with adjustments
}

func newReport(mode string) *Report {
	return &Report{Mode: mode, Members: map[string]int{}, Gatherings: map[string]int{}, Bookings: map[string]int{}, Errors: []Issue{}, Warnings: []Issue{}}
}

// UnusablePrefix marks imported password hashes. Such values can never match
// a password (the verifier requires the "salt.key" format).
const UnusablePrefix = "!imported:"

func unusableHash() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return UnusablePrefix + hex.EncodeToString(b), nil
}

// Plan reports what Apply would do, writing nothing.
func Plan(ctx context.Context, db *sql.DB, in *Input) (*Report, error) {
	return run(ctx, db, in, false)
}

// Apply imports everything in one transaction (all or nothing).
func Apply(ctx context.Context, db *sql.DB, in *Input) (*Report, error) {
	return run(ctx, db, in, true)
}

func run(ctx context.Context, db *sql.DB, in *Input, commit bool) (*Report, error) {
	rep := newReport("dry-run")
	if commit {
		rep.Mode = "applied"
	}
	rep.Errors = append(rep.Errors, in.Issues...)
	rep.Warnings = append(rep.Warnings, in.Warnings...)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// One import at a time.
	if _, err := tx.ExecContext(ctx, `LOCK TABLE legacy_member_map, legacy_gathering_map IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return nil, err
	}
	members, err := importMembers(ctx, tx, in, rep)
	if err != nil {
		return nil, err
	}
	gatherings, err := importGatherings(ctx, tx, in, rep)
	if err != nil {
		return nil, err
	}
	if err := importBookings(ctx, tx, in, rep, members, gatherings); err != nil {
		return nil, err
	}
	if commit {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

type memberRef struct {
	id     string
	status string
}

func importMembers(ctx context.Context, tx *sql.Tx, in *Input, rep *Report) (map[string]memberRef, error) {
	refs := map[string]memberRef{}
	for _, m := range in.Members {
		var id, status string
		err := tx.QueryRowContext(ctx, `SELECT u.id::text, u.status FROM legacy_member_map l JOIN users u ON u.id=l.user_id WHERE l.legacy_id=$1`, m.LegacyID).Scan(&id, &status)
		if err == nil {
			refs[m.LegacyID] = memberRef{id, status}
			rep.Members["already_imported"]++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		var exists bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE email=$1)`, m.Email).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			// Someone already registered on the new platform with this address.
			// Never merge automatically; an operator decides.
			rep.Members["email_exists"]++
			rep.Errors = append(rep.Errors, Issue{"members.csv", m.Row, "email_already_registered"})
			continue
		}
		hash, err := unusableHash()
		if err != nil {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `
INSERT INTO users(email,password_hash,first_name,display_name,phone,area,interests,status,email_updates,email_verified_at,created_at,imported_at)
VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,CASE WHEN $10 THEN now() END,COALESCE($11,now()),now()) RETURNING id::text`,
			m.Email, hash, m.FirstName, m.DisplayName, m.Phone, m.Area, m.Interests, m.Status, m.EmailUpdates, m.EmailConfirmed, m.CreatedAt).Scan(&id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO legacy_member_map(legacy_id,user_id) VALUES($1,$2)`, m.LegacyID, id); err != nil {
			return nil, err
		}
		refs[m.LegacyID] = memberRef{id, m.Status}
		rep.Members["created"]++
	}
	return refs, nil
}

func importGatherings(ctx context.Context, tx *sql.Tx, in *Input, rep *Report) (map[string]string, error) {
	refs := map[string]string{}
	for _, g := range in.Gatherings {
		var id string
		err := tx.QueryRowContext(ctx, `SELECT gathering_id::text FROM legacy_gathering_map WHERE legacy_id=$1`, g.LegacyID).Scan(&id)
		if err == nil {
			refs[g.LegacyID] = id
			rep.Gatherings["already_imported"]++
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO gatherings(title,type,description,starts_on,starts_at,place,capacity) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text`,
			g.Title, g.Type, g.Description, g.StartsOn, g.StartsAt, g.Place, g.Capacity).Scan(&id)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO legacy_gathering_map(legacy_id,gathering_id) VALUES($1,$2)`, g.LegacyID, id); err != nil {
			return nil, err
		}
		refs[g.LegacyID] = id
		rep.Gatherings["created"]++
	}
	// Bookings may also reference gatherings imported in an earlier run.
	rows, err := tx.QueryContext(ctx, `SELECT legacy_id, gathering_id::text FROM legacy_gathering_map`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var l, id string
		if err := rows.Scan(&l, &id); err != nil {
			return nil, err
		}
		if _, ok := refs[l]; !ok {
			refs[l] = id
		}
	}
	return refs, rows.Err()
}

// importBookings preserves confirmed/waitlist status and queue order, but
// never overbooks: confirmed bookings beyond capacity join the waitlist.
func importBookings(ctx context.Context, tx *sql.Tx, in *Input, rep *Report, members map[string]memberRef, gatherings map[string]string) error {
	byGathering := map[string][]Booking{}
	for _, b := range in.Bookings {
		m, okM := members[b.MemberLegacyID]
		g, okG := gatherings[b.GatheringLegacyID]
		switch {
		case !okM:
			rep.Bookings["skipped"]++
			rep.Errors = append(rep.Errors, Issue{"bookings.csv", b.Row, "member_not_imported"})
		case !okG:
			rep.Bookings["skipped"]++
			rep.Errors = append(rep.Errors, Issue{"bookings.csv", b.Row, "gathering_not_imported"})
		case m.status != "approved":
			rep.Bookings["skipped"]++
			rep.Errors = append(rep.Errors, Issue{"bookings.csv", b.Row, "member_not_approved"})
		default:
			byGathering[g] = append(byGathering[g], b)
		}
	}
	ids := make([]string, 0, len(byGathering))
	for id := range byGathering {
		ids = append(ids, id)
	}
	sort.Strings(ids) // lock order
	for _, gid := range ids {
		list := byGathering[gid]
		sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })
		var capacity, confirmed int
		if err := tx.QueryRowContext(ctx, `SELECT capacity FROM gatherings WHERE id=$1 FOR UPDATE`, gid).Scan(&capacity); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'`, gid).Scan(&confirmed); err != nil {
			return err
		}
		for _, b := range list {
			status := b.Status
			if status == "confirmed" && confirmed >= capacity {
				status = "waitlist"
				rep.Warnings = append(rep.Warnings, Issue{"bookings.csv", b.Row, "over_capacity_moved_to_waitlist"})
			}
			res, err := tx.ExecContext(ctx, `INSERT INTO reservations(gathering_id,user_id,status,created_at) VALUES($1,$2,$3,$4) ON CONFLICT (gathering_id,user_id) DO NOTHING`,
				gid, members[b.MemberLegacyID].id, status, b.CreatedAt)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				rep.Bookings["already_present"]++
				continue
			}
			if status == "confirmed" {
				confirmed++
			}
			rep.Bookings["created_"+status]++
		}
	}
	return nil
}

// InviteReport summarises an invitation run.
type InviteReport struct {
	Mode    string `json:"mode"`
	Invited int    `json:"invited"`
	Pending int    `json:"remaining_after_this_run"`
}

// InviteTTL is how long an invitation link stays valid.
const InviteTTL = 7 * 24 * time.Hour

// Invite sends set-your-password invitations (via the outbox) to imported
// members who have not been invited and have not set a password yet. Declined
// members are never invited. limit bounds one run so sending can be paced.
func Invite(ctx context.Context, db *sql.DB, appURL string, limit int, apply bool) (*InviteReport, error) {
	rep := &InviteReport{Mode: "dry-run"}
	if apply {
		rep.Mode = "applied"
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id::text, email, first_name FROM users
WHERE imported_at IS NOT NULL AND invited_at IS NULL AND status <> 'declined' AND password_hash LIKE $1
ORDER BY created_at, id LIMIT $2 FOR UPDATE SKIP LOCKED`, UnusablePrefix+"%", limit)
	if err != nil {
		return nil, err
	}
	type target struct{ id, email, name string }
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.email, &t.name); err != nil {
			rows.Close()
			return nil, err
		}
		targets = append(targets, t)
	}
	rows.Close()
	for _, t := range targets {
		token, err := tokens.Issue(ctx, tx, t.id, tokens.PurposeReset, InviteTTL)
		if err != nil {
			return nil, err
		}
		if err := mail.Enqueue(ctx, tx, t.id, mail.TemplateInvitation, mail.Invitation(t.email, t.name, appURL, token, int(InviteTTL.Hours()/24))); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET invited_at=now() WHERE id=$1`, t.id); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(action,target_id,detail) VALUES('member.invited',$1,'import')`, t.id); err != nil {
			return nil, err
		}
	}
	rep.Invited = len(targets)
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE imported_at IS NOT NULL AND invited_at IS NULL AND status <> 'declined' AND password_hash LIKE $1`,
		UnusablePrefix+"%").Scan(&rep.Pending); err != nil {
		return nil, err
	}
	if apply {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	return rep, nil
}

// CheckDatabase guards apply runs: the operator must name the target database.
func CheckDatabase(ctx context.Context, db *sql.DB, want string) error {
	var name string
	if err := db.QueryRowContext(ctx, `SELECT current_database()`).Scan(&name); err != nil {
		return err
	}
	if want == "" || !strings.EqualFold(name, want) {
		return fmt.Errorf("--confirm-database must equal the target database name (%q)", name)
	}
	return nil
}
