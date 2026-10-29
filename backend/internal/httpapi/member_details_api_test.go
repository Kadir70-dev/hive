package httpapi_test

import "testing"

func TestMemberSignupRequiresAbuDhabiAndUAEPhone(t *testing.T) {
	e := newEnv(t, nil, 2)
	base := map[string]any{"email": "area-check@example.test", "password": goodPass, "firstName": "Noor"}
	r := e.do("POST", "/v1/signup", base)
	expect(t, r, 400, "missing Abu Dhabi area")
	if r.body["code"] != "invalid_area" {
		t.Fatalf("missing area: %s", r.raw)
	}

	base["area"] = "Dubai"
	r = e.do("POST", "/v1/signup", base)
	expect(t, r, 400, "outside Abu Dhabi")
	if r.body["code"] != "invalid_area" {
		t.Fatalf("other emirate: %s", r.raw)
	}

	base["area"] = "Al Reem Island"
	base["phone"] = "+91 98765 43210"
	r = e.do("POST", "/v1/signup", base)
	expect(t, r, 400, "international phone")
	if r.body["code"] != "invalid_phone" {
		t.Fatalf("other country: %s", r.raw)
	}

	base["phone"] = ""
	expect(t, e.do("POST", "/v1/signup", base), 202, "phone optional")
	var savedArea string
	var savedPhone *string
	if err := e.db.QueryRow("SELECT area,phone FROM users WHERE email=$1", base["email"]).Scan(&savedArea, &savedPhone); err != nil {
		t.Fatal(err)
	}
	if savedArea != "Al Reem Island" || savedPhone != nil {
		t.Fatalf("stored area=%q phone=%v", savedArea, savedPhone)
	}
}

func TestMemberProfileAcceptsOnlyUAEPhoneAndAbuDhabiArea(t *testing.T) {
	e := newEnv(t, nil, 2)
	sess := e.signup("valid-member@example.test")
	payload := map[string]any{"firstName": "Noor", "area": "Al Bateen", "phone": "+971 50 123 4567"}
	expect(t, e.do("PATCH", "/v1/me", payload, session(sess)), 200, "valid UAE mobile")
	me := e.do("GET", "/v1/me", nil, session(sess))
	if me.body["phone"] != "+971501234567" || me.body["area"] != "Al Bateen" {
		t.Fatalf("not normalized: %s", me.raw)
	}

	payload["phone"] = "+1 202 555 0100"
	expect(t, e.do("PATCH", "/v1/me", payload, session(sess)), 400, "foreign mobile")
	payload["phone"] = ""
	payload["area"] = "Dubai"
	expect(t, e.do("PATCH", "/v1/me", payload, session(sess)), 400, "outside Abu Dhabi")
	payload["area"] = "Khalifa City"
	expect(t, e.do("PATCH", "/v1/me", payload, session(sess)), 200, "optional mobile removed")
}
