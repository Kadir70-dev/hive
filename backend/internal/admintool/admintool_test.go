package admintool_test

import (
	"context"
	"errors"
	"testing"

	"thehivesociety/api/internal/admintool"
	"thehivesociety/api/internal/testdb"
)

func TestPromoteRequiresAllowlistAndVerifiedEmail(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	allow := admintool.Allowlist(" Owner@Example.test , second@example.test ")
	id := testdb.User(t, db, "owner@example.test", "pending", false)
	testdb.User(t, db, "intruder@example.test", "approved", false)

	if err := admintool.Promote(ctx, db, allow, "intruder@example.test"); !errors.Is(err, admintool.ErrNotAllowlisted) {
		t.Fatalf("non-allowlisted: %v", err)
	}
	if err := admintool.Promote(ctx, db, allow, "second@example.test"); !errors.Is(err, admintool.ErrNoAccount) {
		t.Fatalf("no account: %v", err)
	}
	if err := admintool.Promote(ctx, db, allow, "owner@example.test"); !errors.Is(err, admintool.ErrUnverified) {
		t.Fatalf("unverified: %v", err)
	}
	_, _ = db.Exec(`UPDATE users SET email_verified_at=now() WHERE id=$1`, id)
	if err := admintool.Promote(ctx, db, allow, "OWNER@example.test"); err != nil {
		t.Fatal(err)
	}
	var admin bool
	var status string
	_ = db.QueryRow(`SELECT is_admin, status FROM users WHERE id=$1`, id).Scan(&admin, &status)
	if !admin || status != "approved" {
		t.Fatalf("promote: admin=%v status=%s", admin, status)
	}

	_, _ = db.Exec(`INSERT INTO sessions(id_hash,user_id,expires_at) VALUES('s1',$1,now()+interval '1 day')`, id)
	_, _ = db.Exec(`UPDATE users SET totp_enabled_at=now(), totp_secret_enc='\x00' WHERE id=$1`, id)
	_, _ = db.Exec(`INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,'h')`, id)
	if err := admintool.ResetMFA(ctx, db, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	var mfaOn bool
	var sessions, codes int
	_ = db.QueryRow(`SELECT totp_enabled_at IS NOT NULL FROM users WHERE id=$1`, id).Scan(&mfaOn)
	_ = db.QueryRow(`SELECT count(*) FROM sessions WHERE user_id=$1`, id).Scan(&sessions)
	_ = db.QueryRow(`SELECT count(*) FROM mfa_recovery_codes WHERE user_id=$1`, id).Scan(&codes)
	if mfaOn || sessions != 0 || codes != 0 {
		t.Fatalf("reset-mfa: mfa=%v sessions=%d codes=%d", mfaOn, sessions, codes)
	}

	lines, _ := admintool.List(ctx, db)
	if len(lines) != 1 {
		t.Fatalf("list: %v", lines)
	}
	if err := admintool.Demote(ctx, db, "owner@example.test"); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRow(`SELECT is_admin FROM users WHERE id=$1`, id).Scan(&admin)
	var audits int
	_ = db.QueryRow(`SELECT count(*) FROM audit_events WHERE target_id=$1`, id).Scan(&audits)
	if admin || audits != 3 {
		t.Fatalf("demote: admin=%v audits=%d", admin, audits)
	}
}

func TestTestEmailQueuesThroughOutbox(t *testing.T) {
	db := testdb.Open(t)
	if err := admintool.TestEmail(context.Background(), db, "not-an-address"); err == nil {
		t.Fatal("invalid address accepted")
	}
	if err := admintool.TestEmail(context.Background(), db, "ops@example.test"); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM email_outbox WHERE to_addr='ops@example.test' AND template='delivery_test' AND sent_at IS NULL`).Scan(&n)
	if n != 1 {
		t.Fatalf("queued test emails = %d", n)
	}
}
