package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

func templates(t *testing.T, e *env, email string) map[string]int {
	t.Helper()
	rows, err := e.db.Query(`SELECT template, count(*) FROM email_outbox WHERE to_addr=$1 GROUP BY template`, email)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var tpl string
		var n int
		_ = rows.Scan(&tpl, &n)
		out[tpl] = n
	}
	return out
}

func gatheringID(t *testing.T, e *env, title string) string {
	t.Helper()
	var id string
	if err := e.db.QueryRow(`SELECT id::text FROM gatherings WHERE title=$1`, title).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func approve(t *testing.T, e *env, adminC string, emails ...string) {
	t.Helper()
	for _, em := range emails {
		expect(t, e.do("PATCH", "/v1/admin/members/"+e.userID(em), map[string]any{"status": "approved"}, session(adminC)), 200, "approve "+em)
	}
}

func TestMembershipAndBookingNotifications(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	a := e.signup("a@example.test") // confirming the email alerts the admin
	if templates(t, e, "owner@example.test")["new_member_alert"] != 1 {
		t.Fatalf("admin alert missing: %v", templates(t, e, "owner@example.test"))
	}
	b := e.signup("b@example.test")
	approve(t, e, adminC, "a@example.test", "b@example.test")
	approve(t, e, adminC, "a@example.test") // re-approving must not re-send the welcome
	if n := templates(t, e, "a@example.test")["member_approved"]; n != 1 {
		t.Fatalf("approval emails = %d", n)
	}
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Majlis", "startsOn": "2031-03-01", "startsAt": "19:00", "place": "Al Bateen", "capacity": 1}, session(adminC)), 201, "create")
	g := gatheringID(t, e, "Majlis")

	e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(a))
	e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(a)) // double click
	e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(b))
	if n := templates(t, e, "a@example.test")["booking_confirmation"]; n != 1 {
		t.Fatalf("booking confirmations to a = %d (double clicks must not re-send)", n)
	}
	if templates(t, e, "b@example.test")["booking_confirmation"] != 1 {
		t.Fatal("waitlist confirmation missing")
	}
	expect(t, e.do("POST", "/v1/gatherings/"+g+"/cancel", nil, session(a)), 200, "cancel")
	if templates(t, e, "a@example.test")["booking_cancelled"] != 1 || templates(t, e, "b@example.test")["waitlist_promoted"] != 1 {
		t.Fatalf("cancel/promotion emails: a=%v b=%v", templates(t, e, "a@example.test"), templates(t, e, "b@example.test"))
	}
	var body string
	_ = e.db.QueryRow(`SELECT text_body FROM email_outbox WHERE to_addr='b@example.test' AND template='waitlist_promoted'`).Scan(&body)
	if !containsAll(body, "Majlis", "Al Bateen", "19:00") {
		t.Fatalf("promotion email lacks gathering details: %s", body)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func TestGatheringEditCapacityAndNotifications(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	a, b, c := e.signup("a@example.test"), e.signup("b@example.test"), e.signup("c@example.test")
	approve(t, e, adminC, "a@example.test", "b@example.test", "c@example.test")
	base := map[string]any{"title": "Walk", "type": "wander", "startsOn": "2031-04-01", "startsAt": "07:00", "place": "Corniche", "capacity": 1}
	expect(t, e.do("POST", "/v1/admin/gatherings", base, session(adminC)), 201, "create")
	g := gatheringID(t, e, "Walk")
	for _, s := range []string{a, b, c} {
		e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(s))
	}
	edit := func(changes map[string]any) resp {
		body := map[string]any{}
		for k, v := range base {
			body[k] = v
		}
		for k, v := range changes {
			body[k] = v
		}
		return e.do("PUT", "/v1/admin/gatherings/"+g, body, session(adminC))
	}
	r := edit(map[string]any{"capacity": 2})
	expect(t, r, 200, "raise capacity")
	if r.body["promoted"].(float64) != 1 || templates(t, e, "b@example.test")["waitlist_promoted"] != 1 || templates(t, e, "c@example.test")["waitlist_promoted"] != 0 {
		t.Fatalf("FIFO promotion on capacity raise: %s", r.raw)
	}
	r = edit(map[string]any{"capacity": 1})
	if r.code != 409 || r.body["code"] != "capacity_below_confirmed" {
		t.Fatalf("lowering below confirmed must be refused: %d %s", r.code, r.raw)
	}
	expect(t, edit(map[string]any{"capacity": 2, "description": "Bring water"}), 200, "description-only edit")
	if templates(t, e, "a@example.test")["gathering_updated"] != 0 {
		t.Fatal("description edits must not email members")
	}
	expect(t, edit(map[string]any{"capacity": 2, "place": "Saadiyat Beach"}), 200, "venue change")
	for _, em := range []string{"a@example.test", "b@example.test", "c@example.test"} {
		if templates(t, e, em)["gathering_updated"] != 1 {
			t.Fatalf("%s not told about the venue change", em)
		}
	}
	expect(t, e.do("PUT", "/v1/admin/gatherings/"+g, map[string]any{"title": ""}, session(adminC)), 400, "invalid edit")
	expect(t, e.do("PUT", "/v1/admin/gatherings/00000000-0000-0000-0000-000000000000", base, session(adminC)), 404, "missing gathering")
	expect(t, e.do("PUT", "/v1/admin/gatherings/"+g, base, session(a)), 403, "member cannot edit")

	expect(t, e.do("DELETE", "/v1/admin/gatherings/"+g, nil, session(adminC)), 200, "cancel gathering")
	if templates(t, e, "c@example.test")["gathering_cancelled"] != 1 {
		t.Fatal("waitlisted member not told about the cancellation")
	}
	var audited int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action IN ('gathering.create','gathering.update','gathering.delete')`).Scan(&audited)
	if audited != 5 {
		t.Fatalf("gathering audit events = %d, want 5", audited)
	}
}

func TestPastGatheringsCannotBeBooked(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	a := e.signup("a@example.test")
	approve(t, e, adminC, "a@example.test")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Old", "startsOn": "2020-01-01", "startsAt": "10:00", "place": "x", "capacity": 5}, session(adminC)), 201, "create")
	r := e.do("POST", "/v1/gatherings/"+gatheringID(t, e, "Old")+"/keep", nil, session(a))
	if r.code != 409 || r.body["code"] != "gathering_started" {
		t.Fatalf("past booking: %d %s", r.code, r.raw)
	}
}

// Privilege-escalation and unauthorised-access attempts.
func TestAccessControlAndEscalationAttempts(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	m := e.signup("m@example.test")
	other := e.signup("other@example.test")
	approve(t, e, adminC, "m@example.test", "other@example.test")

	// Mass assignment: extra JSON fields cannot change role or status.
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "M", "isAdmin": true, "is_admin": true, "status": "approved", "IsAdmin": true, "email": "x@evil.test"}, session(m)), 200, "profile update")
	var isAdmin bool
	var email string
	_ = e.db.QueryRow(`SELECT is_admin, email FROM users WHERE id=$1`, e.userID("m@example.test")).Scan(&isAdmin, &email)
	if isAdmin || email != "m@example.test" {
		t.Fatalf("mass assignment: admin=%v email=%s", isAdmin, email)
	}
	// Signup cannot request admin either.
	r := e.do("POST", "/v1/signup", map[string]any{"email": "sneaky@example.test", "password": goodPass, "firstName": "S", "isAdmin": true, "status": "approved"})
	expect(t, r, 202, "signup with extra fields")
	var sneakyAdmin bool
	var sneakyStatus string
	_ = e.db.QueryRow(`SELECT is_admin, status FROM users WHERE email='sneaky@example.test'`).Scan(&sneakyAdmin, &sneakyStatus)
	if sneakyAdmin || sneakyStatus != "pending" {
		t.Fatal("signup escalated privileges")
	}

	adminRoutes := []struct{ method, path string }{
		{"GET", "/v1/admin/members"}, {"PATCH", "/v1/admin/members/" + e.userID("other@example.test")},
		{"GET", "/v1/admin/gatherings"}, {"POST", "/v1/admin/gatherings"}, {"POST", "/v1/admin/checkin"},
		{"GET", "/v1/admin/reports"}, {"POST", "/v1/admin/announcements"}, {"POST", "/v1/admin/partners"},
		{"DELETE", "/v1/admin/partners/00000000-0000-0000-0000-000000000000"},
	}
	for _, rt := range adminRoutes {
		if got := e.do(rt.method, rt.path, map[string]any{"status": "declined"}, session(m)); got.code != 403 {
			t.Errorf("member %s %s: %d", rt.method, rt.path, got.code)
		}
		if got := e.do(rt.method, rt.path, map[string]any{"status": "declined"}); got.code != 401 {
			t.Errorf("anonymous %s %s: %d", rt.method, rt.path, got.code)
		}
	}
	// A forged or stolen-looking cookie value gets nothing.
	expect(t, e.do("GET", "/v1/me", nil, session("forged-session-value")), 401, "forged session")
	// Admins cannot be declined or approved through the API (CLI only).
	expect(t, e.do("PATCH", "/v1/admin/members/"+e.userID("owner@example.test"), map[string]any{"status": "declined"}, session(adminC)), 404, "admin status via API")
	// Members only ever see their own record.
	me := e.do("GET", "/v1/me", nil, session(other))
	if me.body["email"] != "other@example.test" {
		t.Fatalf("me leaked another account: %s", me.raw)
	}
	// Gatherings list never exposes other members' identities.
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "G", "startsOn": "2031-01-01", "startsAt": "10:00", "place": "x", "capacity": 3}, session(adminC)), 201, "create")
	e.do("POST", "/v1/gatherings/"+gatheringID(t, e, "G")+"/keep", nil, session(other))
	list := e.do("GET", "/v1/gatherings", nil, session(m))
	if strings.Contains(list.raw, "other@example.test") || strings.Contains(list.raw, e.userID("other@example.test")) {
		t.Fatal("gatherings list leaks member identities")
	}
}

func TestCSRFOriginEnforcement(t *testing.T) {
	e := newEnv(t, nil, 4)
	c := e.signup("csrf@example.test")
	noOrigin := func(r *http.Request) { r.Header.Del("Origin") }
	r := e.do("PATCH", "/v1/me", map[string]any{"firstName": "X"}, session(c), noOrigin)
	if r.code != 403 || r.body["code"] != "origin_denied" {
		t.Fatalf("cookie request without Origin: %d %s", r.code, r.raw)
	}
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "X"}, session(c), header("Origin", "https://evil.example")), 403, "foreign origin")
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "X", "area": "Al Reem Island"}, session(c)), 200, "same origin")
	expect(t, e.do("GET", "/v1/me", nil, session(c), noOrigin), 200, "safe methods need no Origin")
	expect(t, e.do("POST", "/v1/password/forgot", map[string]any{"email": "a@b.test"}, noOrigin), 202, "cookie-less calls without Origin")
}

func TestPartnerLinksMustBeHTTPS(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	for _, bad := range []string{"javascript:alert(1)", "data:text/html,x", "http://plain.example", "//evil.example", "https://user:pw@x.example", "https://x.example/ space"} {
		r := e.do("POST", "/v1/admin/partners", map[string]any{"name": "P", "websiteURL": bad}, session(adminC))
		if r.code != 400 {
			t.Errorf("accepted %q: %d", bad, r.code)
		}
	}
	expect(t, e.do("POST", "/v1/admin/partners", map[string]any{"name": "P", "websiteURL": "https://partner.example/page", "logoURL": ""}, session(adminC)), 201, "valid partner")
}

func TestAuditTrailCoversSecurityEvents(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	m := e.signup("m@example.test")
	approve(t, e, adminC, "m@example.test")
	e.do("POST", "/v1/admin/announcements", map[string]any{"title": "T", "body": "B"}, session(adminC))
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": goodPass}, session(m)), 200, "delete")
	rows, _ := e.db.Query(`SELECT DISTINCT action FROM audit_events`)
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		seen[a] = true
	}
	for _, want := range []string{"email.verified", "login.success", "mfa.enabled", "member.status", "announcement.create", "account.delete"} {
		if !seen[want] {
			t.Errorf("audit event %q missing (have %v)", want, seen)
		}
	}
	var withEmail int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE detail LIKE '%@%'`).Scan(&withEmail)
	if withEmail != 0 {
		t.Fatal("audit details must not contain email addresses")
	}
}
