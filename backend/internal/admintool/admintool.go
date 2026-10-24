// Package admintool implements the operator-only admin bootstrap commands.
// Admin rights are never granted over HTTP: only from a shell on the server.
package admintool

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"thehivesociety/api/internal/mail"
)

var (
	ErrNotAllowlisted = errors.New("email is not in HIVE_ADMIN_EMAILS")
	ErrNoAccount      = errors.New("no account with that email")
	ErrUnverified     = errors.New("account has not confirmed its email address; ask the owner to verify first")
)

// Allowlist parses HIVE_ADMIN_EMAILS (comma separated).
func Allowlist(raw string) map[string]bool {
	out := map[string]bool{}
	for _, e := range strings.Split(raw, ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			out[e] = true
		}
	}
	return out
}

func audit(ctx context.Context, tx *sql.Tx, action, target, detail string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_events(actor_id,action,target_id,detail) VALUES(NULL,$1,$2,$3)`, action, target, detail)
	return err
}

func lookup(ctx context.Context, tx *sql.Tx, email string) (id string, verified bool, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id::text, email_verified_at IS NOT NULL FROM users WHERE email=$1 FOR UPDATE`, email).Scan(&id, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNoAccount
	}
	return
}

// Promote grants admin to an allow-listed account whose email is verified.
// The member must still enrol TOTP before any admin route works.
func Promote(ctx context.Context, db *sql.DB, allow map[string]bool, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if !allow[email] {
		return ErrNotAllowlisted
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, verified, err := lookup(ctx, tx, email)
	if err != nil {
		return err
	}
	if !verified {
		return ErrUnverified
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET is_admin=true, status='approved' WHERE id=$1`, id); err != nil {
		return err
	}
	if err = audit(ctx, tx, "admin.promote", id, "via admin CLI"); err != nil {
		return err
	}
	return tx.Commit()
}

// Demote removes admin rights and signs the account out everywhere.
func Demote(ctx context.Context, db *sql.DB, email string) error {
	return change(ctx, db, email, "admin.demote", `UPDATE users SET is_admin=false WHERE id=$1`)
}

// ResetMFA clears TOTP and recovery codes (lost device) and signs the account out.
func ResetMFA(ctx context.Context, db *sql.DB, email string) error {
	return change(ctx, db, email, "mfa.reset",
		`UPDATE users SET totp_secret_enc=NULL, totp_pending_enc=NULL, totp_enabled_at=NULL, totp_last_step=NULL WHERE id=$1`,
		`DELETE FROM mfa_recovery_codes WHERE user_id=$1`)
}

func change(ctx context.Context, db *sql.DB, email, action string, stmts ...string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	id, _, err := lookup(ctx, tx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	for _, q := range append(stmts, `DELETE FROM sessions WHERE user_id=$1`) {
		if _, err = tx.ExecContext(ctx, q, id); err != nil {
			return err
		}
	}
	if err = audit(ctx, tx, action, id, "via admin CLI"); err != nil {
		return err
	}
	return tx.Commit()
}

// List returns one line per admin account.
func List(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT email, email_verified_at IS NOT NULL, totp_enabled_at IS NOT NULL FROM users WHERE is_admin ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var email string
		var verified, mfa bool
		if err := rows.Scan(&email, &verified, &mfa); err != nil {
			return nil, err
		}
		out = append(out, fmt.Sprintf("%s verified=%t mfa=%t", email, verified, mfa))
	}
	return out, rows.Err()
}

// TestEmail queues a delivery test through the normal outbox, so it exercises
// the configured provider (e.g. Resend) exactly like real mail.
func TestEmail(ctx context.Context, db *sql.DB, to string) error {
	to = strings.TrimSpace(to)
	if !strings.Contains(to, "@") {
		return errors.New("--email must be an email address")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	msg := mail.Message{To: to, Subject: "The Hive Society email delivery test",
		Text: "This is a delivery test sent by an administrator from the server.",
		HTML: "<p>This is a delivery test sent by an administrator from the server.</p>"}
	if err = mail.Enqueue(ctx, tx, "", "delivery_test", msg); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_events(actor_id,action,detail) VALUES(NULL,'mail.test','via admin CLI')`); err != nil {
		return err
	}
	return tx.Commit()
}
