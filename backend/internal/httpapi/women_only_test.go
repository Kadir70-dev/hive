package httpapi_test

import (
	"testing"
)

func TestWomenOnlyReservationRequiresExplicitAttestation(t *testing.T) {
	e := newEnv(t, nil, 2)
	adminC, _ := e.admin("women-gatherings-admin@example.test")
	memberC := e.signup("women-gatherings-member@example.test")
	approve(t, e, adminC, "women-gatherings-member@example.test")

	create := e.do("POST", "/v1/admin/gatherings", map[string]any{
		"title": "Women-only gathering", "type": "women-only",
		"startsOn": "2031-09-01", "startsAt": "18:00", "place": "Abu Dhabi", "capacity": 1,
	}, session(adminC))
	expect(t, create, 201, "create women-only gathering")
	gid := gatheringID(t, e, "Women-only gathering")
	path := "/v1/gatherings/" + gid + "/keep"

	missing := e.do("POST", path, nil, session(memberC))
	if missing.code != 403 || missing.body["code"] != "women_only_confirmation_required" {
		t.Fatalf("missing attestation: %d %s", missing.code, missing.raw)
	}
	explicitFalse := e.do("POST", path, map[string]any{"womenOnlyAcknowledged": false}, session(memberC))
	expect(t, explicitFalse, 403, "declined attestation")
	var count int
	if err := e.db.QueryRow("SELECT count(*) FROM reservations WHERE gathering_id=$1", gid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("reservation created without acknowledgement: count=%d err=%v", count, err)
	}
	confirmed := e.do("POST", path, map[string]any{"womenOnlyAcknowledged": true}, session(memberC))
	expect(t, confirmed, 200, "acknowledged women-only booking")
	if confirmed.body["status"] != "confirmed" {
		t.Fatalf("booking status: %s", confirmed.raw)
	}
	// Existing reservations remain idempotent for older clients.
	expect(t, e.do("POST", path, nil, session(memberC)), 200, "idempotent existing place")
	changed := e.do("PUT", "/v1/admin/gatherings/"+gid, map[string]any{
		"title": "Women-only gathering", "type": "mixed",
		"startsOn": "2031-09-01", "startsAt": "18:00", "place": "Abu Dhabi", "capacity": 1,
	}, session(adminC))
	if changed.code != 409 || changed.body["code"] != "category_change_with_bookings" {
		t.Fatalf("category change must protect existing reservations: %d %s", changed.code, changed.raw)
	}

	createMixed := e.do("POST", "/v1/admin/gatherings", map[string]any{
		"title": "Mixed gathering", "type": "mixed",
		"startsOn": "2031-09-02", "startsAt": "19:00", "place": "Abu Dhabi", "capacity": 2,
	}, session(adminC))
	expect(t, createMixed, 201, "create mixed gathering")
	mixedID := gatheringID(t, e, "Mixed gathering")
	expect(t, e.do("POST", "/v1/gatherings/"+mixedID+"/keep", nil, session(memberC)), 200, "mixed booking needs no attestation")
}
