package httpapi_test

import (
	"testing"
	"time"

	"thehivesociety/api/internal/httpapi"
	"thehivesociety/api/internal/mfa"
)

func outboxCount(t *testing.T, e *env, email, template string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(`SELECT count(*) FROM email_outbox WHERE to_addr=$1 AND template=$2`, email, template).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSignupDoesNotRevealExistingAccounts(t *testing.T) {
	e := newEnv(t, nil, 4)
	first := e.rawSignup("dup@example.test")
	second := e.rawSignup("dup@example.test")
	if first.raw != second.raw {
		t.Fatalf("responses differ:\n%s\n%s", first.raw, second.raw)
	}
	if outboxCount(t, e, "dup@example.test", "verify_email") != 1 || outboxCount(t, e, "dup@example.test", "account_exists") != 1 {
		t.Fatal("expected one verification email and one account-exists notice")
	}
	e.rawSignup("dup@example.test")
	if n := outboxCount(t, e, "dup@example.test", "account_exists"); n != 1 {
		t.Fatalf("account-exists notices must be throttled, got %d", n)
	}
	var users int
	_ = e.db.QueryRow(`SELECT count(*) FROM users WHERE email='dup@example.test'`).Scan(&users)
	if users != 1 {
		t.Fatalf("users = %d", users)
	}
}

func TestEmailVerificationFlow(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.rawSignup("v@example.test")
	r := e.login("v@example.test", goodPass)
	expect(t, r, 403, "login before verification")
	if r.body["code"] != "email_unverified" || r.cookie != "" {
		t.Fatalf("unexpected: %s", r.raw)
	}
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": "not-a-real-token-but-long-enough"}), 400, "bad token")
	tok := e.lastToken("v@example.test", "verify_email")
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": tok}), 200, "verify")
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": tok}), 400, "token reuse")
	expect(t, e.login("v@example.test", goodPass), 200, "login after verification")
	var stored int
	_ = e.db.QueryRow(`SELECT count(*) FROM email_tokens WHERE token_hash=$1`, tok).Scan(&stored)
	if stored != 0 {
		t.Fatal("raw tokens must never be stored")
	}
}

func TestVerificationExpiryAndResend(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.rawSignup("exp@example.test")
	old := e.lastToken("exp@example.test", "verify_email")
	if _, err := e.db.Exec(`UPDATE email_tokens SET expires_at=now()-interval '1 minute'`); err != nil {
		t.Fatal(err)
	}
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": old}), 400, "expired token")

	unknown := e.do("POST", "/v1/verify-email/resend", map[string]any{"email": "nobody@example.test"})
	known := e.do("POST", "/v1/verify-email/resend", map[string]any{"email": "exp@example.test"})
	expect(t, unknown, 202, "resend unknown")
	expect(t, known, 202, "resend known")
	if unknown.raw != known.raw {
		t.Fatal("resend responses must not reveal accounts")
	}
	if outboxCount(t, e, "nobody@example.test", "verify_email") != 0 {
		t.Fatal("no email for unknown address")
	}
	fresh := e.lastToken("exp@example.test", "verify_email")
	if fresh == old {
		t.Fatal("resend should issue a new token")
	}
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": fresh}), 200, "fresh token")
	// At most three emails per address per hour.
	for i := 0; i < 5; i++ {
		e.do("POST", "/v1/verify-email/resend", map[string]any{"email": "spam@example.test"})
	}
	e.rawSignup("limit@example.test")
	for i := 0; i < 5; i++ {
		e.do("POST", "/v1/verify-email/resend", map[string]any{"email": "limit@example.test"})
	}
	if n := outboxCount(t, e, "limit@example.test", "verify_email"); n != 4 { // signup + 3 resends
		t.Fatalf("verification emails = %d, want 4", n)
	}
}

func TestPasswordResetRevokesSessions(t *testing.T) {
	e := newEnv(t, nil, 4)
	s1 := e.signup("reset@example.test")
	s2 := e.login("reset@example.test", goodPass).cookie

	unknown := e.do("POST", "/v1/password/forgot", map[string]any{"email": "ghost@example.test"})
	known := e.do("POST", "/v1/password/forgot", map[string]any{"email": "Reset@Example.test"})
	expect(t, known, 202, "forgot known")
	if unknown.raw != known.raw {
		t.Fatal("forgot-password responses must not reveal accounts")
	}
	first := e.lastToken("reset@example.test", "reset_password")
	e.do("POST", "/v1/password/forgot", map[string]any{"email": "reset@example.test"})
	tok := e.lastToken("reset@example.test", "reset_password")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": first, "password": "a-brand-new-password"}), 400, "superseded token")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": tok, "password": "short"}), 400, "weak password")

	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": tok, "password": "a-brand-new-password"}), 200, "reset")
	expect(t, e.do("GET", "/v1/me", nil, session(s1)), 401, "session 1 revoked")
	expect(t, e.do("GET", "/v1/me", nil, session(s2)), 401, "session 2 revoked")
	expect(t, e.login("reset@example.test", goodPass), 401, "old password")
	expect(t, e.login("reset@example.test", "a-brand-new-password"), 200, "new password")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": tok, "password": "another-new-password"}), 400, "token reuse")
	if outboxCount(t, e, "reset@example.test", "password_changed") != 1 {
		t.Fatal("password-changed notice not queued")
	}
	var revoked int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='password.reset'`).Scan(&revoked)
	if revoked != 1 {
		t.Fatal("reset not audited")
	}
}

func TestPasswordResetExpiryVerifiesEmailAndLimits(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.rawSignup("unverified@example.test") // never clicked the confirmation link
	e.do("POST", "/v1/password/forgot", map[string]any{"email": "unverified@example.test"})
	tok := e.lastToken("unverified@example.test", "reset_password")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": tok, "password": "a-brand-new-password"}), 200, "reset")
	expect(t, e.login("unverified@example.test", "a-brand-new-password"), 200, "reset proves inbox ownership")

	e.do("POST", "/v1/password/forgot", map[string]any{"email": "unverified@example.test"})
	tok = e.lastToken("unverified@example.test", "reset_password")
	_, _ = e.db.Exec(`UPDATE email_tokens SET expires_at=now()-interval '1 second' WHERE purpose='reset_password'`)
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": tok, "password": "yet-another-password"}), 400, "expired reset token")

	for i := 0; i < 6; i++ {
		e.do("POST", "/v1/password/forgot", map[string]any{"email": "unverified@example.test"})
	}
	if n := outboxCount(t, e, "unverified@example.test", "reset_password"); n != 3 {
		t.Fatalf("reset emails = %d, want 3 per hour", n)
	}
}

func TestTokenEndpointsLimitedPerIP(t *testing.T) {
	e := newEnv(t, func(l *httpapi.Limits) { l.TokenPerIP.Limit = 3 }, 4)
	for i := 0; i < 3; i++ {
		expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": "guess-guess-guess-guess-guess"}, from("203.0.113.50")), 400, "guess")
	}
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": "guess-guess-guess-guess-guess", "password": "whatever-password"}, from("203.0.113.50")), 429, "token guessing throttled")
}

func TestAdminRoutesRequireMFA(t *testing.T) {
	e := newEnv(t, nil, 4)
	c := e.signup("boss@example.test")
	_, _ = e.db.Exec(`UPDATE users SET is_admin=true, status='approved' WHERE email='boss@example.test'`)
	r := e.do("GET", "/v1/admin/members", nil, session(c))
	if r.code != 403 || r.body["code"] != "mfa_enrollment_required" {
		t.Fatalf("expected enrolment prompt: %d %s", r.code, r.raw)
	}
	setup := e.do("POST", "/v1/mfa/totp/setup", nil, session(c))
	secret := setup.body["secret"].(string)
	expect(t, e.do("POST", "/v1/mfa/totp/enable", map[string]any{"code": "000000"}, session(c)), 401, "wrong enrolment code")
	code, _ := mfa.Code(secret, time.Now())
	en := e.do("POST", "/v1/mfa/totp/enable", map[string]any{"code": code}, session(c))
	expect(t, en, 200, "enable")
	recovery := en.body["recovery_codes"].([]any)
	if len(recovery) != 10 {
		t.Fatalf("recovery codes = %d", len(recovery))
	}
	expect(t, e.do("GET", "/v1/admin/members", nil, session(c)), 200, "enrolling session is verified")
	expect(t, e.do("POST", "/v1/mfa/totp/setup", nil, session(c)), 409, "re-setup blocked")

	var stored []byte
	_ = e.db.QueryRow(`SELECT totp_secret_enc FROM users WHERE email='boss@example.test'`).Scan(&stored)
	if len(stored) == 0 || string(stored) == secret {
		t.Fatal("TOTP secret must be stored encrypted")
	}

	// A new login needs a fresh code.
	login := e.login("boss@example.test", goodPass)
	if login.body["mfa_required"] != true {
		t.Fatalf("login should flag MFA: %s", login.raw)
	}
	c2 := login.cookie
	r = e.do("GET", "/v1/admin/members", nil, session(c2))
	if r.code != 403 || r.body["code"] != "mfa_required" {
		t.Fatalf("expected step-up prompt: %d %s", r.code, r.raw)
	}
	expect(t, e.do("POST", "/v1/mfa/verify", map[string]any{"code": code}, session(c2)), 401, "replayed enrolment code")
	expect(t, e.do("POST", "/v1/mfa/verify", map[string]any{"recoveryCode": recovery[0]}, session(c2)), 200, "recovery code")
	expect(t, e.do("GET", "/v1/admin/members", nil, session(c2)), 200, "after recovery code")
	c3 := e.login("boss@example.test", goodPass).cookie
	expect(t, e.do("POST", "/v1/mfa/verify", map[string]any{"recoveryCode": recovery[0]}, session(c3)), 401, "recovery code is single use")

	// Brute force is throttled per admin.
	for i := 0; i < 4; i++ {
		e.do("POST", "/v1/mfa/verify", map[string]any{"code": "123456"}, session(c3))
	}
	expect(t, e.do("POST", "/v1/mfa/verify", map[string]any{"code": "654321"}, session(c3)), 429, "mfa attempts throttled")

	// Step-up expires.
	_, _ = e.db.Exec(`UPDATE sessions SET mfa_verified_at=now()-interval '13 hours'`)
	r = e.do("GET", "/v1/admin/members", nil, session(c))
	if r.code != 403 || r.body["code"] != "mfa_required" {
		t.Fatalf("expired step-up should prompt again: %d %s", r.code, r.raw)
	}
}

func TestMFAEndpointsLockedForMembersAndWithoutKey(t *testing.T) {
	e := newEnvWith(t, nil, 4, false)
	member := e.signup("m@example.test")
	expect(t, e.do("POST", "/v1/mfa/totp/setup", nil, session(member)), 403, "members cannot enrol")
	_, _ = e.db.Exec(`UPDATE users SET is_admin=true WHERE email='m@example.test'`)
	expect(t, e.do("POST", "/v1/mfa/totp/setup", nil, session(member)), 503, "no key configured")
	r := e.do("GET", "/v1/admin/members", nil, session(member))
	if r.code != 403 {
		t.Fatalf("admin routes must stay locked without MFA: %d", r.code)
	}
	// An admin flag on an unverified address grants nothing.
	_, _ = e.db.Exec(`UPDATE users SET email_verified_at=NULL WHERE email='m@example.test'`)
	r = e.do("GET", "/v1/admin/members", nil, session(member))
	if r.code != 403 || r.body["code"] != "forbidden" {
		t.Fatalf("unverified admin: %d %s", r.code, r.raw)
	}
}

func TestApprovalRequiresVerifiedEmail(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("lead@example.test")
	e.rawSignup("new@example.test")
	id := e.userID("new@example.test")
	r := e.do("PATCH", "/v1/admin/members/"+id, map[string]any{"status": "approved"}, session(adminC))
	if r.code != 409 || r.body["code"] != "email_unverified" {
		t.Fatalf("approving unverified member: %d %s", r.code, r.raw)
	}
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": e.lastToken("new@example.test", "verify_email")}), 200, "verify")
	expect(t, e.do("PATCH", "/v1/admin/members/"+id, map[string]any{"status": "approved"}, session(adminC)), 200, "approve verified")
	var audited int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action='member.status' AND target_id=$1`, id).Scan(&audited)
	if audited != 1 {
		t.Fatal("approval not audited")
	}
}
