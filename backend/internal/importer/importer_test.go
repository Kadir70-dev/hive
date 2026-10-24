package importer_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"thehivesociety/api/internal/importer"
	"thehivesociety/api/internal/testdb"
)

func load(t *testing.T) *importer.Input {
	t.Helper()
	in, err := importer.Load("testdata/export")
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func codes(issues []importer.Issue) map[string]int {
	out := map[string]int{}
	for _, i := range issues {
		out[i.File+":"+i.Code]++
	}
	return out
}

func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestLoadValidatesRows(t *testing.T) {
	in := load(t)
	c := codes(in.Issues)
	want := map[string]int{
		"members.csv:invalid_email": 1, "members.csv:duplicate_email_in_file": 1, "members.csv:missing_first_name": 1,
		"members.csv:unknown_status": 1, "members.csv:duplicate_legacy_id": 1,
		"gatherings.csv:invalid_starts_on": 1, "bookings.csv:duplicate_booking": 1, "bookings.csv:invalid_booking_status": 1,
	}
	for k, v := range want {
		if c[k] != v {
			t.Errorf("%s: got %d want %d (all: %v)", k, c[k], v, c)
		}
	}
	// m1..m4, m9, m10 are valid; m5–m8 and the repeated m1 are rejected above.
	if len(in.Members) != 6 || in.Members[0].Email != "aisha.fixture@example.test" || in.Members[1].Status != "approved" || in.Members[3].Status != "declined" {
		t.Fatalf("members parsed: %+v", in.Members)
	}
	if codes(in.Warnings)["members.csv:invalid_phone_dropped"] != 1 || in.Members[1].Phone != "" {
		t.Fatal("invalid phone should be dropped with a warning")
	}
}

func TestPlanWritesNothingAndApplyIsIdempotent(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO users(email,password_hash,first_name,status) VALUES('existing.fixture@example.test','x','Already','approved')`); err != nil {
		t.Fatal(err)
	}
	plan, err := importer.Plan(ctx, db, load(t))
	if err != nil {
		t.Fatal(err)
	}
	if count(t, db, `SELECT count(*) FROM users`) != 1 || count(t, db, `SELECT count(*) FROM legacy_member_map`) != 0 {
		t.Fatal("dry run wrote data")
	}
	applied, err := importer.Apply(ctx, db, load(t))
	if err != nil {
		t.Fatal(err)
	}
	pj, _ := json.Marshal(plan.Members)
	aj, _ := json.Marshal(applied.Members)
	if string(pj) != string(aj) || plan.Mode != "dry-run" || applied.Mode != "applied" {
		t.Fatalf("plan must predict apply exactly: %s vs %s", pj, aj)
	}
	if applied.Members["created"] != 5 || applied.Members["email_exists"] != 1 {
		t.Fatalf("members: %v", applied.Members)
	}
	// Capacity 1: first confirmed booking keeps its place, the second becomes waitlist;
	// pending member's booking is skipped; queue order follows created_at.
	if applied.Bookings["created_confirmed"] != 1 || applied.Bookings["created_waitlist"] != 2 || codes(applied.Warnings)["bookings.csv:over_capacity_moved_to_waitlist"] != 1 {
		t.Fatalf("bookings: %v warnings %v", applied.Bookings, applied.Warnings)
	}
	ce := codes(applied.Errors)
	if ce["bookings.csv:member_not_approved"] != 1 || ce["bookings.csv:member_not_imported"] != 1 || ce["bookings.csv:gathering_not_imported"] != 1 || ce["members.csv:email_already_registered"] != 1 {
		t.Fatalf("booking/member errors: %v", ce)
	}
	if count(t, db, `SELECT count(*) FROM users WHERE email='existing.fixture@example.test' AND first_name='Already'`) != 1 {
		t.Fatal("existing account must not be modified")
	}
	var status string
	_ = db.QueryRow(`SELECT r.status FROM reservations r JOIN users u ON u.id=r.user_id WHERE u.email='bina.fixture@example.test'`).Scan(&status)
	if status != "waitlist" {
		t.Fatalf("second confirmed booking over capacity: %s", status)
	}
	if count(t, db, `SELECT count(*) FROM users WHERE email='aisha.fixture@example.test' AND email_verified_at IS NOT NULL AND email_updates AND imported_at IS NOT NULL`) != 1 {
		t.Fatal("member fields not mapped")
	}

	// Re-running imports nothing new.
	again, err := importer.Apply(ctx, db, load(t))
	if err != nil {
		t.Fatal(err)
	}
	if again.Members["created"] != 0 || again.Members["already_imported"] != 5 || again.Gatherings["created"] != 0 || again.Bookings["already_present"] != 3 {
		t.Fatalf("re-run must be a no-op: %+v", again)
	}
	if count(t, db, `SELECT count(*) FROM users`) != 6 || count(t, db, `SELECT count(*) FROM reservations`) != 3 {
		t.Fatal("re-run created duplicates")
	}
}

func TestImportedPasswordsAreUnusableAndReportsArePrivate(t *testing.T) {
	db := testdb.Open(t)
	rep, err := importer.Apply(context.Background(), db, load(t))
	if err != nil {
		t.Fatal(err)
	}
	if count(t, db, `SELECT count(*) FROM users WHERE imported_at IS NOT NULL AND password_hash NOT LIKE '!imported:%'`) != 0 {
		t.Fatal("imported accounts must have unusable passwords")
	}
	raw, _ := json.Marshal(rep)
	for _, leak := range []string{"@", "Aisha", "Bina", "+971", "Al Reem", "Qahwa"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("report contains personal data %q: %s", leak, raw)
		}
	}
}

func TestInvitations(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if _, err := importer.Apply(ctx, db, load(t)); err != nil {
		t.Fatal(err)
	}
	dry, err := importer.Invite(ctx, db, "https://thehivesociety.ae", 2, false)
	if err != nil || dry.Invited != 2 || count(t, db, `SELECT count(*) FROM email_outbox`) != 0 {
		t.Fatalf("dry-run invite: %+v %v", dry, err)
	}
	first, err := importer.Invite(ctx, db, "https://thehivesociety.ae", 2, true)
	if err != nil || first.Invited != 2 || first.Pending != 3 {
		t.Fatalf("first batch: %+v %v", first, err)
	}
	rest, _ := importer.Invite(ctx, db, "https://thehivesociety.ae", 50, true)
	if rest.Invited != 3 || rest.Pending != 0 {
		t.Fatalf("second batch: %+v", rest)
	}
	none, _ := importer.Invite(ctx, db, "https://thehivesociety.ae", 50, true)
	if none.Invited != 0 {
		t.Fatal("members must not be invited twice")
	}
	// Declined members are never invited; each invite carries a 7-day reset token.
	if count(t, db, `SELECT count(*) FROM email_outbox o JOIN users u ON u.id=o.user_id WHERE u.status='declined'`) != 0 {
		t.Fatal("declined member invited")
	}
	if count(t, db, `SELECT count(*) FROM email_tokens WHERE purpose='reset_password' AND expires_at > now() + interval '6 days'`) != 5 {
		t.Fatal("invitation tokens missing")
	}
	var body string
	_ = db.QueryRow(`SELECT text_body FROM email_outbox WHERE template='member_invitation' LIMIT 1`).Scan(&body)
	if !strings.Contains(body, "/reset-password#token=") {
		t.Fatalf("invitation link: %s", body)
	}
}

func TestApplyRequiresDatabaseConfirmation(t *testing.T) {
	db := testdb.Open(t)
	if err := importer.CheckDatabase(context.Background(), db, ""); err == nil {
		t.Fatal("empty confirmation accepted")
	}
	if err := importer.CheckDatabase(context.Background(), db, "production"); err == nil {
		t.Fatal("wrong database accepted")
	}
	var name string
	_ = db.QueryRow(`SELECT current_database()`).Scan(&name)
	if err := importer.CheckDatabase(context.Background(), db, name); err != nil {
		t.Fatal(err)
	}
}
