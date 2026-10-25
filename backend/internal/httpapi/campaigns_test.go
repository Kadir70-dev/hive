package httpapi_test

import (
	"strings"
	"testing"
)

// This integration test runs against disposable PostgreSQL in CI. It exercises
// the actual HTTP signup, verification and MFA-gated admin reporting paths.
func TestCampaignAttributionEndToEnd(t *testing.T) {
	e := newEnv(t, nil, 3)
	adminC, _ := e.admin("campaign-admin@example.test")

	input := map[string]any{
		"email": "campaign-new@example.test", "password": goodPass, "firstName": "Noor",
		"utmSource": "instagram", "utmMedium": "paid_social",
		"utmCampaign": "abu_dhabi_gatherings",
		"utmContent": "creative_b", "utmTerm": "culture",
	}
	expect(t, e.do("POST", "/v1/signup", input), 202, "campaign signup")
	var verified bool
	if err := e.db.QueryRow("SELECT verified_at IS NOT NULL FROM signup_attribution WHERE user_id=$1", e.userID("campaign-new@example.test")).Scan(&verified); err != nil || verified {
		t.Fatalf("campaign must remain unverified until email confirmation: verified=%v err=%v", verified, err)
	}

	// An unrelated account must never see the private reporting endpoint.
	memberC := e.signup("campaign-member@example.test")
	expect(t, e.do("GET", "/v1/admin/campaigns", nil, session(memberC)), 403, "member must not see campaigns")

	before := e.do("GET", "/v1/admin/campaigns", nil, session(adminC))
	expect(t, before, 200, "admin campaigns before verification")
	if before.body["tracked_started"] != float64(1) || before.body["tracked_verified"] != float64(0) {
		t.Fatalf("expected 1 started, 0 verified: %s", before.raw)
	}

	token := e.lastToken("campaign-new@example.test", "verify_email")
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": token}), 200, "verify campaign signup")
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": token}), 400, "replay must not double-count")
	after := e.do("GET", "/v1/admin/campaigns", nil, session(adminC))
	expect(t, after, 200, "admin campaigns after verification")
	if after.body["tracked_started"] != float64(1) || after.body["tracked_verified"] != float64(1) {
		t.Fatalf("expected 1 verified signup: %s", after.raw)
	}
	for _, pii := range []string{"campaign-new@example.test", "campaign-member@example.test", "Noor", e.userID("campaign-new@example.test")} {
		if strings.Contains(after.raw, pii) {
			t.Fatalf("campaign report leaked member information")
		}
	}
	for _, item := range []string{"instagram", "paid_social", "abu_dhabi_gatherings"} {
		if !strings.Contains(after.raw, item) {
			t.Fatalf("report missing source %q: %s", item, after.raw)
		}
	}

	// Duplicate requests must not modify or reattribute an existing account.
	input["utmSource"] = "spoofed_second_source"
	expect(t, e.do("POST", "/v1/signup", input), 202, "duplicate signup accepted without enumeration")
	unchanged := e.do("GET", "/v1/admin/campaigns", nil, session(adminC))
	expect(t, unchanged, 200, "report after duplicate request")
	if unchanged.body["tracked_started"] != float64(1) || strings.Contains(unchanged.raw, "spoofed_second_source") {
		t.Fatalf("duplicate signup changed campaign attribution: %s", unchanged.raw)
	}

	// Deletion removes per-account campaign records via ON DELETE CASCADE.
	signedIn := e.login("campaign-new@example.test", goodPass)
	expect(t, signedIn, 200, "campaign user login")
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": goodPass}, session(signedIn.cookie)), 200, "account deletion")
	afterDelete := e.do("GET", "/v1/admin/campaigns", nil, session(adminC))
	expect(t, afterDelete, 200, "campaign report after deletion")
	if afterDelete.body["tracked_started"] != float64(0) || afterDelete.body["tracked_verified"] != float64(0) {
		t.Fatalf("deleted member must leave no attribution: %s", afterDelete.raw)
	}
}

func TestCampaignAttributionInputBoundedAndOptional(t *testing.T) {
	e := newEnv(t, nil, 2)
	invalid := e.do("POST", "/v1/signup", map[string]any{
		"email": "bad-campaign@example.test", "password": goodPass, "firstName": "Noor",
		"utmCampaign": strings.Repeat("a", 81),
	})
	if invalid.code != 400 || invalid.body["code"] != "invalid_campaign" {
		t.Fatalf("oversized UTM field should be rejected: %d %s", invalid.code, invalid.raw)
	}
	var n int
	if err := e.db.QueryRow("SELECT count(*) FROM users WHERE email='bad-campaign@example.test'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("invalid campaign created an account: %d %v", n, err)
	}
	e.rawSignup("organic-signup@example.test")
	if err := e.db.QueryRow("SELECT count(*) FROM signup_attribution").Scan(&n); err != nil || n != 0 {
		t.Fatalf("signup without UTM should not store attribution: %d %v", n, err)
	}
}
