package httpapi_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"thehivesociety/api/internal/httpapi"
)

func items(t *testing.T, r resp) []map[string]any {
	t.Helper()
	var out struct {
		Items []map[string]any `json:"items"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(r.raw), &out); err != nil {
		t.Fatalf("not a paged response: %s", r.raw)
	}
	return out.Items
}

func list(t *testing.T, r resp) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal([]byte(r.raw), &out); err != nil {
		t.Fatalf("not a list: %s", r.raw)
	}
	return out
}

func approvedMember(t *testing.T, e *env, adminC, email string) string {
	t.Helper()
	c := e.signup(email)
	approve(t, e, adminC, email)
	return c
}

func TestAnnouncementsLifecycleAndEmailPreferences(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	in := approvedMember(t, e, adminC, "optin@example.test")
	out := approvedMember(t, e, adminC, "optout@example.test")
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "In", "area": "Al Reem Island", "phone": "+971501234567", "emailUpdates": true}, session(in)), 200, "opt in")
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "Out", "area": "Al Reem Island", "phone": "+971501234567", "emailUpdates": false}, session(out)), 200, "opt out")
	e.rawSignup("pending@example.test") // unverified + pending: never emailed even if opted in
	_, _ = e.db.Exec(`UPDATE users SET email_updates=true WHERE email='pending@example.test'`)

	// Draft (unpublished) announcements are invisible to members and cannot be emailed.
	r := e.do("POST", "/v1/admin/announcements", map[string]any{"title": "Season two", "body": "Line one\n\nLine two", "published": false}, session(adminC))
	expect(t, r, 201, "create draft")
	id := r.body["id"].(string)
	if len(items(t, e.do("GET", "/v1/announcements", nil, session(in)))) != 0 {
		t.Fatal("draft visible to members")
	}
	r = e.do("POST", "/v1/admin/announcements/"+id+"/email", nil, session(adminC))
	if r.code != 409 || r.body["code"] != "not_published" {
		t.Fatalf("emailing a draft: %d %s", r.code, r.raw)
	}
	expect(t, e.do("PUT", "/v1/admin/announcements/"+id, map[string]any{"title": "Season two!", "body": "Line one\n\nLine two", "published": true}, session(adminC)), 200, "publish")
	feed := items(t, e.do("GET", "/v1/announcements", nil, session(in)))
	if len(feed) != 1 || feed[0]["title"] != "Season two!" {
		t.Fatalf("feed: %v", feed)
	}

	r = e.do("POST", "/v1/admin/announcements/"+id+"/email", nil, session(adminC))
	expect(t, r, 200, "email")
	if r.body["queued"].(float64) != 1 {
		t.Fatalf("queued %v, want only the opted-in member", r.body["queued"])
	}
	// Retrying never duplicates.
	r = e.do("POST", "/v1/admin/announcements/"+id+"/email", nil, session(adminC))
	if r.body["queued"].(float64) != 0 {
		t.Fatalf("retry queued %v", r.body["queued"])
	}
	if templates(t, e, "optin@example.test")["announcement"] != 1 || templates(t, e, "optout@example.test")["announcement"] != 0 ||
		templates(t, e, "pending@example.test")["announcement"] != 0 || templates(t, e, "owner@example.test")["announcement"] != 0 {
		t.Fatal("announcement email preferences not respected")
	}
	var body string
	_ = e.db.QueryRow(`SELECT text_body FROM email_outbox WHERE template='announcement'`).Scan(&body)
	if !strings.Contains(body, "turn them off") || !strings.Contains(body, "/profile") {
		t.Fatalf("announcement email lacks the opt-out explanation: %s", body)
	}

	// Unpublish hides it again; members cannot reach admin endpoints.
	expect(t, e.do("PUT", "/v1/admin/announcements/"+id, map[string]any{"title": "Season two!", "body": "x", "published": false}, session(adminC)), 200, "unpublish")
	if len(items(t, e.do("GET", "/v1/announcements", nil, session(in)))) != 0 {
		t.Fatal("unpublished announcement still visible")
	}
	for _, rt := range []struct{ m, p string }{{"GET", "/v1/admin/announcements"}, {"PUT", "/v1/admin/announcements/" + id}, {"POST", "/v1/admin/announcements/" + id + "/email"}, {"DELETE", "/v1/admin/announcements/" + id}} {
		if got := e.do(rt.m, rt.p, map[string]any{"title": "x", "body": "y", "published": true}, session(in)); got.code != 403 {
			t.Errorf("member %s %s: %d", rt.m, rt.p, got.code)
		}
	}
	expect(t, e.do("POST", "/v1/admin/announcements", map[string]any{"title": "", "body": "x"}, session(adminC)), 400, "validation")
	expect(t, e.do("POST", "/v1/admin/announcements", map[string]any{"title": "bad\x00title", "body": "x"}, session(adminC)), 400, "control characters")
	expect(t, e.do("DELETE", "/v1/admin/announcements/"+id, nil, session(adminC)), 200, "delete")
	var audits int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action LIKE 'announcement.%'`).Scan(&audits)
	if audits != 6 { // create, publish, email x2, unpublish, delete
		t.Fatalf("announcement audit events = %d", audits)
	}
}

func TestAnnouncementPagination(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	m := approvedMember(t, e, adminC, "m@example.test")
	for i := 0; i < 23; i++ {
		e.do("POST", "/v1/admin/announcements", map[string]any{"title": fmt.Sprintf("News %02d", i), "body": "b"}, session(adminC))
	}
	p1 := e.do("GET", "/v1/announcements?page=1", nil, session(m))
	p2 := e.do("GET", "/v1/announcements?page=2", nil, session(m))
	if len(items(t, p1)) != 20 || len(items(t, p2)) != 3 || p1.body["has_more"] != true || p2.body["has_more"] != false || p1.body["total"].(float64) != 23 {
		t.Fatalf("pagination: %d/%d %v %v", len(items(t, p1)), len(items(t, p2)), p1.body["has_more"], p2.body["has_more"])
	}
	search := items(t, e.do("GET", "/v1/admin/announcements?q=News%2007", nil, session(adminC)))
	if len(search) != 1 {
		t.Fatalf("admin search returned %d", len(search))
	}
	if len(items(t, e.do("GET", "/v1/admin/announcements?q=%25", nil, session(adminC)))) != 0 {
		t.Fatal("LIKE wildcards in search must be escaped")
	}
}

func TestPartnerCRUD(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	m := approvedMember(t, e, adminC, "m@example.test")
	r := e.do("POST", "/v1/admin/partners", map[string]any{"name": "Qahwa House", "websiteURL": "https://qahwa.example", "published": false}, session(adminC))
	expect(t, r, 201, "create")
	id := r.body["id"].(string)
	if len(list(t, e.do("GET", "/v1/partners", nil, session(m)))) != 0 {
		t.Fatal("unpublished partner visible")
	}
	expect(t, e.do("PUT", "/v1/admin/partners/"+id, map[string]any{"name": "Qahwa House", "description": "Coffee", "websiteURL": "https://qahwa.example/menu", "published": true}, session(adminC)), 200, "publish")
	got := list(t, e.do("GET", "/v1/partners", nil, session(m)))
	if len(got) != 1 || got[0]["website_url"] != "https://qahwa.example/menu" {
		t.Fatalf("member partner list: %v", got)
	}
	expect(t, e.do("PUT", "/v1/admin/partners/"+id, map[string]any{"name": "Q", "websiteURL": "javascript:alert(1)", "published": true}, session(adminC)), 400, "https only on edit")
	expect(t, e.do("PUT", "/v1/admin/partners/"+id, map[string]any{"name": "Q", "websiteURL": "http://plain.example", "published": true}, session(adminC)), 400, "no plain http")
	expect(t, e.do("PUT", "/v1/admin/partners/00000000-0000-0000-0000-000000000000", map[string]any{"name": "Q", "published": true}, session(adminC)), 404, "missing")
	expect(t, e.do("PUT", "/v1/admin/partners/"+id, map[string]any{"name": "Q", "published": true}, session(m)), 403, "member edit")
	if len(items(t, e.do("GET", "/v1/admin/partners?q=qahwa", nil, session(adminC)))) != 1 {
		t.Fatal("admin partner search")
	}
	expect(t, e.do("DELETE", "/v1/admin/partners/"+id, nil, session(adminC)), 200, "delete")
	expect(t, e.do("DELETE", "/v1/admin/partners/"+id, nil, session(adminC)), 404, "delete twice")
}

// Reports are admin-mediated: members never identify other members.
func TestReportingAndBlockingWorkflow(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	reporter := approvedMember(t, e, adminC, "reporter@example.test")
	subject := approvedMember(t, e, adminC, "subject@example.test")
	other := approvedMember(t, e, adminC, "other@example.test")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Majlis", "startsOn": "2031-05-01", "startsAt": "19:00", "place": "Bateen", "capacity": 10}, session(adminC)), 201, "gathering")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Walk", "startsOn": "2031-05-02", "startsAt": "07:00", "place": "Corniche", "capacity": 10}, session(adminC)), 201, "gathering 2")
	g1, g2 := gatheringID(t, e, "Majlis"), gatheringID(t, e, "Walk")
	e.do("POST", "/v1/gatherings/"+g1+"/keep", nil, session(reporter))
	e.do("POST", "/v1/gatherings/"+g1+"/keep", nil, session(subject))

	// Reports can only reference gatherings the reporter booked.
	expect(t, e.do("POST", "/v1/reports", map[string]any{"category": "harassment", "gatheringID": g2, "reason": "something happened here"}, session(reporter)), 400, "foreign gathering")
	expect(t, e.do("POST", "/v1/reports", map[string]any{"category": "nonsense", "reason": "something happened here"}, session(reporter)), 400, "bad category")
	r := e.do("POST", "/v1/reports", map[string]any{"category": "harassment", "gatheringID": g1, "subjectDescription": "woman in the green scarf",
		"reason": "Repeated unwanted comments during the majlis.", "blockRequested": true}, session(reporter))
	expect(t, r, 201, "report")
	reportID := r.body["id"].(string)

	// Reporter sees status only; the reported member and others see nothing.
	mine := list(t, e.do("GET", "/v1/reports/mine", nil, session(reporter)))
	if len(mine) != 1 || mine[0]["status"] != "open" || mine[0]["gathering_title"] != "Majlis" || mine[0]["resolution_note"] != nil {
		t.Fatalf("my reports: %v", mine)
	}
	if len(list(t, e.do("GET", "/v1/reports/mine", nil, session(subject)))) != 0 || len(list(t, e.do("GET", "/v1/reports/mine", nil, session(other)))) != 0 {
		t.Fatal("reports leaked to other members")
	}
	for _, rt := range []struct{ m, p string }{{"GET", "/v1/admin/reports"}, {"PATCH", "/v1/admin/reports/" + reportID}, {"POST", "/v1/admin/reports/" + reportID + "/block"}} {
		if got := e.do(rt.m, rt.p, map[string]any{"status": "dismissed"}, session(subject)); got.code != 403 {
			t.Errorf("member %s %s: %d", rt.m, rt.p, got.code)
		}
	}

	// Admin moderation: block requires a linked member; link from the roster, then block.
	expect(t, e.do("POST", "/v1/admin/reports/"+reportID+"/block", nil, session(adminC)), 409, "block before linking")
	expect(t, e.do("PATCH", "/v1/admin/reports/"+reportID, map[string]any{"status": "in_review", "reportedMemberID": e.userID("reporter@example.test")}, session(adminC)), 404, "cannot link reporter to own report")
	expect(t, e.do("PATCH", "/v1/admin/reports/"+reportID, map[string]any{"status": "in_review", "resolutionNote": "Spoke to host", "reportedMemberID": e.userID("subject@example.test")}, session(adminC)), 200, "link")
	expect(t, e.do("POST", "/v1/admin/reports/"+reportID+"/block", nil, session(adminC)), 200, "block")
	roster := list(t, e.do("GET", "/v1/admin/gatherings/"+g1+"/attendees", nil, session(adminC)))
	conflicts := 0
	for _, a := range roster {
		if a["block_conflict"] == true {
			conflicts++
		}
	}
	if conflicts != 2 {
		t.Fatalf("roster should flag both people in the existing conflict, got %d: %v", conflicts, roster)
	}

	// Enforcement: neither side can join a gathering the other has booked (both directions).
	e.do("POST", "/v1/gatherings/"+g2+"/keep", nil, session(subject))
	got := e.do("POST", "/v1/gatherings/"+g2+"/keep", nil, session(reporter))
	if got.code != 403 || got.body["code"] != "not_available" || strings.Contains(got.raw, "subject") || strings.Contains(got.raw, "block") {
		t.Fatalf("blocked booking must be refused generically: %d %s", got.code, got.raw)
	}
	expect(t, e.do("POST", "/v1/gatherings/"+g2+"/keep", nil, session(other)), 200, "unrelated member unaffected")

	reports := items(t, e.do("GET", "/v1/admin/reports?status=in_review", nil, session(adminC)))
	if len(reports) != 1 || reports[0]["block_applied"] != true || reports[0]["reported_member_name"] != "Noor" {
		t.Fatalf("admin report view: %v", reports)
	}
	expect(t, e.do("PATCH", "/v1/admin/reports/"+reportID, map[string]any{"status": "resolved", "resolutionNote": "Block applied"}, session(adminC)), 200, "resolve")
	mine = list(t, e.do("GET", "/v1/reports/mine", nil, session(reporter)))
	if mine[0]["status"] != "resolved" {
		t.Fatal("reporter should see the outcome status")
	}
	expect(t, e.do("DELETE", "/v1/admin/reports/"+reportID+"/block", nil, session(adminC)), 200, "lift block")
	expect(t, e.do("POST", "/v1/gatherings/"+g2+"/keep", nil, session(reporter)), 200, "booking allowed after block lifted")

	// Audit trail records actions but never report contents.
	var leaked int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE detail ILIKE '%scarf%' OR detail ILIKE '%comments%' OR detail ILIKE '%host%'`).Scan(&leaked)
	if leaked != 0 {
		t.Fatal("audit events contain report contents")
	}
	var actions int
	_ = e.db.QueryRow(`SELECT count(DISTINCT action) FROM audit_events WHERE action IN ('report.create','report.status','block.create','block.remove')`).Scan(&actions)
	if actions != 4 {
		t.Fatalf("report audit actions = %d", actions)
	}
}

func TestReportsRateLimited(t *testing.T) {
	e := newEnv(t, func(l *httpapi.Limits) { l.ReportsPerMember.Limit = 2 }, 4)
	adminC, _ := e.admin("owner@example.test")
	m := approvedMember(t, e, adminC, "m@example.test")
	for i := 0; i < 2; i++ {
		expect(t, e.do("POST", "/v1/reports", map[string]any{"reason": "something happened here"}, session(m)), 201, "report")
	}
	expect(t, e.do("POST", "/v1/reports", map[string]any{"reason": "something happened here"}, session(m)), 429, "report flood")
}

func TestMyGatheringsAndStatusVisibility(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	a := approvedMember(t, e, adminC, "a@example.test")
	b := approvedMember(t, e, adminC, "b@example.test")
	c := approvedMember(t, e, adminC, "c@example.test")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Small", "startsOn": "2031-06-01", "startsAt": "10:00", "place": "x", "capacity": 1}, session(adminC)), 201, "gathering")
	g := gatheringID(t, e, "Small")
	for _, s := range []string{a, b, c} {
		e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(s))
	}
	mineC := list(t, e.do("GET", "/v1/me/gatherings", nil, session(c)))
	if len(mineC) != 1 || mineC[0]["status"] != "waitlist" || mineC[0]["waitlist_position"].(float64) != 2 {
		t.Fatalf("c's gatherings: %v", mineC)
	}
	mineA := list(t, e.do("GET", "/v1/me/gatherings", nil, session(a)))
	if mineA[0]["status"] != "confirmed" || mineA[0]["waitlist_position"] != nil {
		t.Fatalf("a's gatherings: %v", mineA)
	}
	listing := list(t, e.do("GET", "/v1/gatherings", nil, session(b)))
	if listing[0]["my_status"] != "waitlist" || listing[0]["waitlist_position"].(float64) != 1 || listing[0]["places_left"].(float64) != 0 {
		t.Fatalf("b's listing: %v", listing)
	}
	// No member identities in any member-facing response.
	for _, raw := range []string{e.do("GET", "/v1/me/gatherings", nil, session(c)).raw, e.do("GET", "/v1/gatherings", nil, session(c)).raw} {
		if strings.Contains(raw, "a@example.test") || strings.Contains(raw, e.userID("a@example.test")) {
			t.Fatal("member-facing gathering data leaks another member")
		}
	}
	expect(t, e.do("POST", "/v1/gatherings/"+g+"/cancel", nil, session(a)), 200, "cancel")
	if list(t, e.do("GET", "/v1/me/gatherings", nil, session(b)))[0]["status"] != "confirmed" ||
		list(t, e.do("GET", "/v1/me/gatherings", nil, session(c)))[0]["waitlist_position"].(float64) != 1 {
		t.Fatal("queue positions not updated after promotion")
	}
	roster := list(t, e.do("GET", "/v1/admin/gatherings/"+g+"/attendees", nil, session(adminC)))
	if len(roster) != 2 || roster[0]["status"] != "confirmed" || roster[1]["waitlist_position"].(float64) != 1 {
		t.Fatalf("admin roster: %v", roster)
	}
}

func TestProfileValidation(t *testing.T) {
	e := newEnv(t, nil, 4)
	m := e.signup("m@example.test")
	bad := []map[string]any{
		{"firstName": ""},
		{"firstName": strings.Repeat("a", 61)},
		{"firstName": "Ok", "phone": "call me maybe"},
		{"firstName": "Ok", "phone": "123"},
		{"firstName": "Ok", "displayName": "line\nbreak"},
		{"firstName": "Ok", "interests": []string{strings.Repeat("x", 41)}},
		{"firstName": "Ok", "interests": []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11"}},
	}
	for _, b := range bad {
		if r := e.do("PATCH", "/v1/me", b, session(m)); r.code != 400 {
			t.Errorf("accepted invalid profile %v: %d", b, r.code)
		}
	}
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "  Noor ", "area": "Al Reem Island", "phone": "+971 50 123 4567", "interests": []string{"wellness", "Wellness", " majlis "}, "emailUpdates": true}, session(m)), 200, "valid profile")
	me := e.do("GET", "/v1/me", nil, session(m))
	if me.body["first_name"] != "Noor" || len(me.body["interests"].([]any)) != 2 || me.body["email_updates"] != true {
		t.Fatalf("normalised profile: %s", me.raw)
	}
}

func TestAccountDeletionRemovesPersonalData(t *testing.T) {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	m := approvedMember(t, e, adminC, "gone@example.test")
	id := e.userID("gone@example.test")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "G", "startsOn": "2031-06-01", "startsAt": "10:00", "place": "x", "capacity": 5}, session(adminC)), 201, "gathering")
	e.do("POST", "/v1/gatherings/"+gatheringID(t, e, "G")+"/keep", nil, session(m))
	e.do("POST", "/v1/reports", map[string]any{"reason": "a report I wrote myself"}, session(m))
	e.do("POST", "/v1/password/forgot", map[string]any{"email": "gone@example.test"})
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": goodPass}, session(m)), 200, "delete")
	for table, col := range map[string]string{"users": "id", "sessions": "user_id", "reservations": "user_id", "email_tokens": "user_id",
		"email_outbox": "user_id", "member_reports": "reporter_id", "announcement_deliveries": "user_id", "member_blocks": "blocker_id"} {
		var n int
		if err := e.db.QueryRow(fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s=$1`, table, col), id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s still holds %d rows for the deleted member", table, n)
		}
	}
	expect(t, e.do("GET", "/v1/me", nil, session(m)), 401, "session gone")
	expect(t, e.login("gone@example.test", goodPass), 401, "cannot log in")
}
