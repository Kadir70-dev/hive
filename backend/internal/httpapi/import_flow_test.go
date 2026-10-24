package httpapi_test

import (
	"context"
	"testing"

	"thehivesociety/api/internal/importer"
)

func TestImportedMemberActivatesThroughInvitation(t *testing.T) {
	e := newEnv(t, nil, 4)
	in, err := importer.Load("../importer/testdata/export")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := importer.Apply(context.Background(), e.db, in); err != nil {
		t.Fatal(err)
	}
	// No password works before the invitation is used — not even the empty one.
	for _, pw := range []string{"", "anything-at-all-123", "!imported:"} {
		if r := e.login("aisha.fixture@example.test", pw); r.code != 401 {
			t.Fatalf("imported account accepted password %q: %d", pw, r.code)
		}
	}
	if _, err := importer.Invite(context.Background(), e.db, origin, 50, true); err != nil {
		t.Fatal(err)
	}
	token := e.lastToken("aisha.fixture@example.test", "member_invitation")
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": token, "password": "a-brand-new-password"}), 200, "accept invitation")
	r := e.login("aisha.fixture@example.test", "a-brand-new-password")
	expect(t, r, 200, "login after invitation")
	me := e.do("GET", "/v1/me", nil, session(r.cookie))
	if me.body["status"] != "approved" || me.body["first_name"] != "Aisha" {
		t.Fatalf("imported profile: %s", me.raw)
	}
	gs := list(t, e.do("GET", "/v1/me/gatherings", nil, session(r.cookie)))
	if len(gs) != 1 || gs[0]["status"] != "confirmed" {
		t.Fatalf("imported booking: %v", gs)
	}
	expect(t, e.do("POST", "/v1/password/reset", map[string]any{"token": token, "password": "another-password-1"}), 400, "invitation is single use")
}
