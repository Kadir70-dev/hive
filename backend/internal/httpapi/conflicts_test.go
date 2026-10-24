package httpapi_test

import (
	"strings"
	"sync"
	"testing"
)

type conflictSetup struct {
	e                         *env
	adminC, rep, sub, waiting string
	g, reportID               string
}

// setupConflict: reporter and subject both booked the same gathering (capacity 2,
// one more member waiting), then an admin links the report and applies a block.
func setupConflict(t *testing.T) conflictSetup {
	e := newEnv(t, nil, 4)
	adminC, _ := e.admin("owner@example.test")
	rep := approvedMember(t, e, adminC, "rep@example.test")
	sub := approvedMember(t, e, adminC, "sub@example.test")
	waiting := approvedMember(t, e, adminC, "wait@example.test")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Majlis", "startsOn": "2031-07-01", "startsAt": "19:00", "place": "Bateen", "capacity": 2}, session(adminC)), 201, "gathering")
	g := gatheringID(t, e, "Majlis")
	for _, s := range []string{rep, sub, waiting} {
		e.do("POST", "/v1/gatherings/"+g+"/keep", nil, session(s))
	}
	r := e.do("POST", "/v1/reports", map[string]any{"category": "harassment", "gatheringID": g, "reason": "unwanted comments again", "blockRequested": true}, session(rep))
	reportID := r.body["id"].(string)
	expect(t, e.do("PATCH", "/v1/admin/reports/"+reportID, map[string]any{"status": "in_review", "reportedMemberID": e.userID("sub@example.test")}, session(adminC)), 200, "link")
	b := e.do("POST", "/v1/admin/reports/"+reportID+"/block", nil, session(adminC))
	expect(t, b, 200, "block")
	if b.body["conflicts"].(float64) != 1 {
		t.Fatalf("applying the block should report the existing conflict: %s", b.raw)
	}
	return conflictSetup{e, adminC, rep, sub, waiting, g, reportID}
}

func TestSafetyConflictListAndAuthorization(t *testing.T) {
	c := setupConflict(t)
	l := list(t, c.e.do("GET", "/v1/admin/safety-conflicts", nil, session(c.adminC)))
	if len(l) != 1 || l[0]["gathering_id"] != c.g || l[0]["report_id"] != c.reportID {
		t.Fatalf("conflicts: %v", l)
	}
	for _, s := range []string{c.rep, c.sub} {
		expect(t, c.e.do("GET", "/v1/admin/safety-conflicts", nil, session(s)), 403, "member list")
		expect(t, c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "released_reported"}, session(s)), 403, "member resolve")
	}
	// Members' own views never mention the other person or the block.
	for _, s := range []string{c.rep, c.sub} {
		for _, path := range []string{"/v1/me/gatherings", "/v1/gatherings", "/v1/reports/mine"} {
			raw := c.e.do("GET", path, nil, session(s)).raw
			if strings.Contains(raw, "rep@example.test") || strings.Contains(raw, "sub@example.test") || strings.Contains(raw, "block") || strings.Contains(raw, "conflict") {
				t.Fatalf("%s leaks conflict details: %s", path, raw)
			}
		}
	}
	// Nothing was cancelled automatically.
	if list(t, c.e.do("GET", "/v1/me/gatherings", nil, session(c.sub)))[0]["status"] != "confirmed" {
		t.Fatal("existing booking must stay until an admin decides")
	}
	expect(t, c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "cancel_everyone"}, session(c.adminC)), 400, "invalid decision")
}

func TestSafetyConflictReleaseIsNeutralAndPromotes(t *testing.T) {
	c := setupConflict(t)
	expect(t, c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "released_reported", "note": "Spoke to both"}, session(c.adminC)), 200, "release")
	if len(list(t, c.e.do("GET", "/v1/me/gatherings", nil, session(c.sub)))) != 0 {
		t.Fatal("released member still booked")
	}
	if list(t, c.e.do("GET", "/v1/me/gatherings", nil, session(c.waiting)))[0]["status"] != "confirmed" {
		t.Fatal("freed place should go to the next in line")
	}
	var body string
	if err := c.e.db.QueryRow(`SELECT text_body FROM email_outbox WHERE to_addr='sub@example.test' AND template='place_released'`).Scan(&body); err != nil {
		t.Fatal("released member was not told")
	}
	// No reason, no other person, no internal note. (Test members all share the
	// first name used in the greeting, so names are checked via emails.)
	for _, leak := range []string{"rep@", "report", "block", "safety", "Spoke to both", "harassment"} {
		if strings.Contains(strings.ToLower(body), strings.ToLower(leak)) {
			t.Fatalf("release email leaks %q: %s", leak, body)
		}
	}
	if len(list(t, c.e.do("GET", "/v1/admin/safety-conflicts", nil, session(c.adminC)))) != 0 {
		t.Fatal("resolved conflict still listed")
	}
	expect(t, c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "acknowledged"}, session(c.adminC)), 409, "decide twice")
	var leaked int
	_ = c.e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE detail ILIKE '%spoke%'`).Scan(&leaked)
	if leaked != 0 {
		t.Fatal("audit contains the admin note")
	}
}

func TestSafetyConflictAcknowledgeKeepsBookings(t *testing.T) {
	c := setupConflict(t)
	expect(t, c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "acknowledged", "note": "Different tables"}, session(c.adminC)), 200, "acknowledge")
	for _, s := range []string{c.rep, c.sub} {
		if list(t, c.e.do("GET", "/v1/me/gatherings", nil, session(s)))[0]["status"] != "confirmed" {
			t.Fatal("acknowledging must keep both bookings")
		}
	}
	if len(list(t, c.e.do("GET", "/v1/admin/safety-conflicts", nil, session(c.adminC)))) != 0 {
		t.Fatal("acknowledged conflict still listed")
	}
}

// Two admins resolving at once, racing a member's own cancellation: exactly one
// decision, exactly one release, no lost or double promotion.
func TestSafetyConflictConcurrentResolution(t *testing.T) {
	c := setupConflict(t)
	second, _ := c.e.admin("second@example.test")
	var wg sync.WaitGroup
	codes := make([]int, 3)
	run := func(i int, f func() resp) { defer wg.Done(); codes[i] = f().code }
	wg.Add(3)
	go run(0, func() resp {
		return c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "released_reported"}, session(c.adminC))
	})
	go run(1, func() resp {
		return c.e.do("POST", "/v1/admin/safety-conflicts/resolve", map[string]any{"gatheringID": c.g, "reportID": c.reportID, "decision": "released_reporter"}, session(second))
	})
	go run(2, func() resp { return c.e.do("POST", "/v1/gatherings/"+c.g+"/cancel", nil, session(c.sub)) })
	wg.Wait()
	var decisions, confirmed, reservations int
	_ = c.e.db.QueryRow(`SELECT count(*) FROM safety_conflict_decisions`).Scan(&decisions)
	_ = c.e.db.QueryRow(`SELECT count(*) FILTER (WHERE status='confirmed'), count(*) FROM reservations WHERE gathering_id=$1`, c.g).Scan(&confirmed, &reservations)
	if decisions > 1 {
		t.Fatalf("%d decisions recorded", decisions)
	}
	// Both originally confirmed members may independently leave during this race.
	// If both leave there is only one waiting member to promote, so one remaining
	// confirmed place is correct; require no overbooking or stranded waitlist.
	if confirmed < 1 || confirmed > 2 || reservations != confirmed {
		t.Fatalf("after races: %d confirmed of %d reservations (codes %v)", confirmed, reservations, codes)
	}
	var waitingStatus string
	if err := c.e.db.QueryRow(`SELECT status FROM reservations WHERE gathering_id=$1 AND user_id=$2`,
		c.g, c.e.userID("wait@example.test")).Scan(&waitingStatus); err != nil || waitingStatus != "confirmed" {
		t.Fatalf("earliest waiter was not promoted: status=%q err=%v (codes %v)", waitingStatus, err, codes)
	}
	for _, code := range codes {
		if code != 200 && code != 409 && code != 404 {
			t.Fatalf("unexpected status in race: %v", codes)
		}
	}
}
