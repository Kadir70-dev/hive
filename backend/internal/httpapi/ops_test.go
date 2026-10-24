package httpapi_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"thehivesociety/api/internal/httpapi"
	"thehivesociety/api/internal/storage"
)

func metricsReq(e *env, token string) (int, string) {
	req := httptest.NewRequest("GET", "/internal/metrics", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestMetricsEndpoint(t *testing.T) {
	e := newEnvStore(t, nil, 4, true, storage.NewMemory())
	if code, _ := metricsReq(e, ""); code != 401 {
		t.Fatalf("anonymous metrics: %d", code)
	}
	if code, _ := metricsReq(e, "wrong-token-wrong-token-wrong-token"); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	e.rawSignup("queued@example.test") // leaves one email in the outbox
	code, body := metricsReq(e, testMetricsToken)
	if code != 200 {
		t.Fatalf("metrics: %d %s", code, body)
	}
	for _, want := range []string{"hive_up 1", "hive_db_up 1", "hive_email_outbox_pending 1", "hive_storage_configured 1", "hive_ratelimit_redis_degraded 0", "hive_safety_conflicts_open 0"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "@") {
		t.Fatal("metrics must not contain personal data")
	}
	r := e.do("GET", "/ready", nil)
	if r.body["storage"] != "ok" || r.body["database"] != "ok" {
		t.Fatalf("readiness: %s", r.raw)
	}
}

func TestMetricsDisabledWithoutToken(t *testing.T) {
	// No METRICS_TOKEN: the endpoint does not exist, even with a bearer header.
	h := httpapi.New(httpapi.Config{})
	for _, auth := range []string{"", "Bearer ", "Bearer anything"} {
		req := httptest.NewRequest("GET", "/internal/metrics", nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 404 {
			t.Fatalf("metrics without a configured token (%q): %d", auth, rec.Code)
		}
	}
}

// Sensitive values must never appear in API logs or audit records.
func TestLogsAndAuditAreRedacted(t *testing.T) {
	e := newEnvStore(t, nil, 4, true, storage.NewMemory())
	const pw = "a-long-enough-password"
	adminC, _ := e.admin("owner@example.test")
	m := e.signup("secret.member@example.test")
	approve(t, e, adminC, "secret.member@example.test")
	e.login("secret.member@example.test", "wrong-password-xyz")
	e.do("POST", "/v1/password/forgot", map[string]any{"email": "secret.member@example.test"})
	resetToken := e.lastToken("secret.member@example.test", "reset_password")
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "Zainab", "phone": "+971 55 765 4321", "emailUpdates": true}, session(m)), 200, "profile")
	e.do("POST", "/v1/reports", map[string]any{"reason": "very private incident description", "subjectDescription": "tall woman in blue"}, session(m))
	expect(t, e.upload(m, "photo", photoJPEG(t, 80, 80)), 200, "photo")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": resetToken, "password": "another-secret-pass"}), 200, "reset")
	e.do("GET", "/v1/nothing?email=secret.member@example.test", nil) // query strings are never logged
	logs := e.logs.String()
	if !strings.Contains(logs, `"route"`) {
		t.Fatalf("expected request logs to be captured: %.300s", logs)
	}
	var audit strings.Builder
	rows, _ := e.db.Query(`SELECT action || ' ' || COALESCE(detail,'') FROM audit_events`)
	for rows.Next() {
		var line string
		_ = rows.Scan(&line)
		audit.WriteString(line + "\n")
	}
	rows.Close()
	for _, secret := range []string{"secret.member@example.test", "owner@example.test", pw, "wrong-password-xyz", "another-secret-pass",
		resetToken, "+971 55 765 4321", "very private incident", "tall woman in blue", m, "Zainab"} {
		if strings.Contains(logs, secret) {
			t.Errorf("API logs contain sensitive value %q", secret)
		}
		if strings.Contains(audit.String(), secret) {
			t.Errorf("audit trail contains sensitive value %q", secret)
		}
	}
}
