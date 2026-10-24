package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"thehivesociety/api/internal/mail"

	"thehivesociety/api/internal/seats"
)

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r, 25, 100)
	status := r.URL.Query().Get("status")
	if status != "" && status != "pending" && status != "approved" && status != "declined" {
		s.fail(w, r, 400, "invalid_status", "invalid status filter")
		return
	}
	like := likePattern(p.Query)
	const where = `WHERE ($1='' OR status=$1) AND ($2='' OR first_name ILIKE $3 OR display_name ILIKE $3 OR email ILIKE $3)`
	var total int
	if err := s.db.QueryRowContext(r.Context(), "SELECT count(*) FROM users "+where, status, p.Query, like).Scan(&total); err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id::text,email,first_name,status,is_admin,email_verified_at IS NOT NULL,created_at::text FROM users `+where+`
ORDER BY (status='pending') DESC, created_at DESC, id LIMIT $4 OFFSET $5`, status, p.Query, like, p.Size, p.offset())
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, email, name, st, created string
		var admin, verified bool
		if err := rows.Scan(&id, &email, &name, &st, &admin, &verified, &created); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "email": email, "first_name": name, "status": st, "is_admin": admin, "email_verified": verified, "created_at": created})
	}
	reply(w, 200, paged(items, p, total))
}

// memberStatus changes approval. Moving a member out of "approved" releases
// their seats and promotes the waitlist in the same transaction.
func (s *Server) memberStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v struct{ Status string }
	if !s.parse(w, r, &v) {
		return
	}
	if v.Status != "pending" && v.Status != "approved" && v.Status != "declined" {
		s.fail(w, r, 400, "invalid_status", "invalid status")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	defer tx.Rollback()
	// Lock the member row; admins are managed only with the admin CLI.
	var current, email, name string
	var verified bool
	err = tx.QueryRowContext(r.Context(), "SELECT status, email, first_name, email_verified_at IS NOT NULL FROM users WHERE id=$1 AND is_admin=false FOR UPDATE", id).
		Scan(&current, &email, &name, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 404, "not_found", "member not found")
		return
	}
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if v.Status == "approved" && !verified {
		s.fail(w, r, 409, "email_unverified", "the member has not confirmed their email address yet")
		return
	}
	if v.Status != "approved" {
		// Leaving "approved" releases the member's places and promotes the waitlist.
		var promoted []seats.Promotion
		promoted, err = seats.ReleaseUser(r.Context(), tx, id)
		if err == nil {
			err = s.notifyPromotions(r.Context(), tx, promoted)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE users SET status=$1 WHERE id=$2", v.Status, id)
	}
	if err == nil && v.Status == "approved" && current != "approved" {
		err = mail.Enqueue(r.Context(), tx, id, mail.TemplateMemberApproved, mail.MemberApproved(email, name, s.appURL))
	}
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	s.audit(r.Context(), tx, own(r).ID, "member.status", id, v.Status)
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) adminGatherings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
SELECT g.id::text,g.title,g.type,g.description,g.starts_on::text,g.starts_at::text,g.place,g.capacity,
  count(*) FILTER (WHERE r.status='confirmed'), count(*) FILTER (WHERE r.status='waitlist'),
  (SELECT count(*) FROM attendance a WHERE a.gathering_id=g.id AND a.attended),
  ((g.starts_on + g.starts_at) AT TIME ZONE 'Asia/Dubai') <= now()
FROM gatherings g LEFT JOIN reservations r ON r.gathering_id=g.id
GROUP BY g.id ORDER BY g.starts_on DESC, g.starts_at DESC LIMIT 500`)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place string
		var capacity, confirmed, waiting, attended int
		var started bool
		if err := rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &capacity, &confirmed, &waiting, &attended, &started); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place,
			"capacity": capacity, "confirmed": confirmed, "waitlisted": waiting, "attended": attended, "started": started})
	}
	reply(w, 200, out)
}

// attendees is the admin roster for one gathering: confirmed members first,
// then the waitlist in queue order, with check-in state and a flag when a
// safety block exists between two people on the list.
func (s *Server) attendees(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT u.id::text, u.first_name, u.email, r.status,
  CASE WHEN r.status='waitlist' THEN row_number() OVER (PARTITION BY r.status ORDER BY r.created_at, r.id) END,
  COALESCE(a.attended,false),
  EXISTS(SELECT 1 FROM reservations o JOIN member_blocks b
    ON (b.blocker_id=u.id AND b.blocked_id=o.user_id) OR (b.blocked_id=u.id AND b.blocker_id=o.user_id)
    WHERE o.gathering_id=r.gathering_id)
FROM reservations r JOIN users u ON u.id=r.user_id
LEFT JOIN attendance a ON a.gathering_id=r.gathering_id AND a.user_id=u.id
WHERE r.gathering_id=$1 ORDER BY (r.status='confirmed') DESC, r.created_at, r.id`, id)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var uid, name, email, status string
		var position sql.NullInt64
		var attended, conflict bool
		if err := rows.Scan(&uid, &name, &email, &status, &position, &attended, &conflict); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		row := map[string]any{"user_id": uid, "first_name": name, "email": email, "status": status, "attended": attended, "block_conflict": conflict}
		if position.Valid {
			row["waitlist_position"] = position.Int64
		}
		out = append(out, row)
	}
	reply(w, 200, out)
}

// checkin records attendance only for confirmed reservations and reports
// failure when nothing was written.
func (s *Server) checkin(w http.ResponseWriter, r *http.Request) {
	var v struct {
		GatheringID, UserID string
		Attended            bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	if !validUUID(v.GatheringID) || !validUUID(v.UserID) {
		s.fail(w, r, 400, "invalid_checkin", "invalid checkin")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `INSERT INTO attendance(gathering_id,user_id,attended,checked_at) SELECT $1,$2,$3,now() WHERE EXISTS(SELECT 1 FROM reservations WHERE gathering_id=$1 AND user_id=$2 AND status='confirmed') ON CONFLICT(gathering_id,user_id) DO UPDATE SET attended=excluded.attended,checked_at=now()`, v.GatheringID, v.UserID, v.Attended)
	if err != nil {
		s.internal(w, r, "checkin failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 409, "not_confirmed", "only members with a confirmed place can be checked in")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "attendance.checkin", v.UserID, v.GatheringID)
	reply(w, 200, map[string]any{"ok": true})
}

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// deleteQueries maps allowed tables to fixed statements (no string building).
var deleteQueries = map[string]string{
	"announcements": "DELETE FROM announcements WHERE id=$1",
	"partners":      "DELETE FROM partners WHERE id=$1",
}

func (s *Server) deleteByID(w http.ResponseWriter, r *http.Request, table string) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), deleteQueries[table], id)
	if err != nil {
		s.internal(w, r, "delete failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, strings.TrimSuffix(table, "s")+".delete", id, "")
	reply(w, 200, map[string]any{"ok": true})
}
