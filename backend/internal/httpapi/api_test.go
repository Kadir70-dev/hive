package httpapi_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"regexp"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"thehivesociety/api/internal/clientip"
	"thehivesociety/api/internal/httpapi"
	"thehivesociety/api/internal/mfa"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/storage"
	"thehivesociety/api/internal/testdb"
)

const (
	origin      = "https://thehivesociety.ae"
	testMFAKey  = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	testMetricsToken = "test-metrics-token-0123456789abcdef"
	proxySecret = "test-proxy-secret-0123456789abcdef"
	goodPass    = "a-long-enough-password"
)

// syncBuffer collects API logs so tests can assert what is (not) logged.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type env struct {
	logs *syncBuffer
	t   *testing.T
	db  *sql.DB
	mr  *miniredis.Miniredis
	h   http.Handler
	lim httpapi.Limits
}

func testLimits() httpapi.Limits {
	return httpapi.Limits{
		LoginPerIP:      ratelimit.Rule{Name: "login-ip", Limit: 1000, Window: 10 * time.Minute},
		SignupPerIP:     ratelimit.Rule{Name: "signup-ip", Limit: 1000, Window: time.Hour},
		LoginPerAccount: ratelimit.Rule{Name: "login-acct", Limit: 5, Window: 15 * time.Minute},
		APIPerIP:        ratelimit.Rule{Name: "api-ip", Limit: 10000, Window: time.Minute},
		HashWait:        5 * time.Second,
	}
}

func newEnv(t *testing.T, mutate func(*httpapi.Limits), hashSlots int) *env {
	t.Helper()
	return newEnvWith(t, mutate, hashSlots, true)
}

func newEnvWith(t *testing.T, mutate func(*httpapi.Limits), hashSlots int, withMFA bool) *env {
	t.Helper()
	return newEnvStore(t, mutate, hashSlots, withMFA, nil)
}

// newEnvStore also wires a photo store (nil disables photos).
func newEnvStore(t *testing.T, mutate func(*httpapi.Limits), hashSlots int, withMFA bool, store storage.Store) *env {
	t.Helper()
	db := testdb.Open(t)
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr(), MaxRetries: -1, DialTimeout: 100 * time.Millisecond})
	t.Cleanup(func() { rdb.Close() })
	lim := testLimits()
	if mutate != nil {
		mutate(&lim)
	}
	ips, err := clientip.New("10.0.0.2/32", proxySecret)
	if err != nil {
		t.Fatal(err)
	}
	logs := &syncBuffer{}
	log := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	var cipher *mfa.Cipher
	if withMFA {
		if cipher, err = mfa.NewCipher(testMFAKey); err != nil {
			t.Fatal(err)
		}
	}
	h := httpapi.New(httpapi.Config{
		AppURL: origin, MFACipher: cipher, Store: store, ImageSlots: 2, MetricsToken: testMetricsToken,
		DB: db, Redis: rdb, Hasher: password.NewHasher(hashSlots), Limiter: ratelimit.New(rdb, log), IPs: ips, Log: log,
		SecureCookies: true, AllowedOrigins: []string{origin}, RequestTimeout: 10 * time.Second, MaxBodyBytes: 4096, Limits: lim,
	})
	return &env{t: t, db: db, mr: mr, h: h, lim: lim, logs: logs}
}

type resp struct {
	code   int
	header http.Header
	body   map[string]any
	raw    string
	cookie string
}

type opt func(*http.Request)

func from(ip string) opt { return func(r *http.Request) { r.RemoteAddr = ip + ":40000" } }
func header(k, v string) opt {
	return func(r *http.Request) { r.Header.Set(k, v) }
}
func session(c string) opt {
	return func(r *http.Request) {
		if c != "" {
			r.AddCookie(&http.Cookie{Name: "hive_session", Value: c})
		}
	}
}

func (e *env) do(method, path string, body any, opts ...opt) resp {
	e.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.RemoteAddr = "198.51.100.1:40000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	for _, o := range opts {
		o(req)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := resp{code: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	for _, c := range rec.Result().Cookies() {
		if c.Name == "hive_session" {
			out.cookie = c.Value
		}
	}
	return out
}

// rawSignup submits the signup form and expects the uniform 202 response.
func (e *env) rawSignup(email string, opts ...opt) resp {
	e.t.Helper()
	r := e.do("POST", "/v1/signup", map[string]any{"email": email, "password": goodPass, "firstName": "Noor"}, opts...)
	if r.code != 202 || r.cookie != "" {
		e.t.Fatalf("signup %s: %d %s (cookie=%q)", email, r.code, r.raw, r.cookie)
	}
	return r
}

var tokenRe = regexp.MustCompile(`#token=([A-Za-z0-9_-]+)`)

// lastToken extracts the one-time token from the newest queued email of a template.
func (e *env) lastToken(email, template string) string {
	e.t.Helper()
	var body string
	err := e.db.QueryRow(`SELECT text_body FROM email_outbox WHERE to_addr=$1 AND template=$2 ORDER BY id DESC LIMIT 1`, email, template).Scan(&body)
	if err != nil {
		e.t.Fatalf("no %s email for %s: %v", template, email, err)
	}
	m := tokenRe.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("no token in %s email", template)
	}
	return m[1]
}

func (e *env) login(email, pass string, opts ...opt) resp {
	e.t.Helper()
	return e.do("POST", "/v1/login", map[string]any{"email": email, "password": pass}, opts...)
}

// signup runs the full member flow (sign up, confirm email, log in) and returns the session.
func (e *env) signup(email string, opts ...opt) string {
	e.t.Helper()
	e.rawSignup(email, opts...)
	expect(e.t, e.do("POST", "/v1/verify-email", map[string]any{"token": e.lastToken(email, "verify_email")}, opts...), 200, "verify "+email)
	r := e.login(email, goodPass, opts...)
	if r.code != 200 || r.cookie == "" {
		e.t.Fatalf("login %s: %d %s", email, r.code, r.raw)
	}
	return r.cookie
}

// admin creates a verified admin with TOTP enrolled; returns session and secret.
func (e *env) admin(email string) (string, string) {
	e.t.Helper()
	c := e.signup(email)
	if _, err := e.db.Exec(`UPDATE users SET is_admin=true, status='approved' WHERE email=$1`, email); err != nil {
		e.t.Fatal(err)
	}
	setup := e.do("POST", "/v1/mfa/totp/setup", nil, session(c))
	expect(e.t, setup, 200, "mfa setup")
	secret := setup.body["secret"].(string)
	code, _ := mfa.Code(secret, time.Now())
	expect(e.t, e.do("POST", "/v1/mfa/totp/enable", map[string]any{"code": code}, session(c)), 200, "mfa enable")
	return c, secret
}

func (e *env) userID(email string) string {
	var id string
	if err := e.db.QueryRow(`SELECT id::text FROM users WHERE email=$1`, email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func expect(t *testing.T, r resp, code int, what string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("%s: got %d, want %d (%s)", what, r.code, code, r.raw)
	}
}

func myStatus(t *testing.T, e *env, cookie string) string {
	t.Helper()
	r := e.do("GET", "/v1/gatherings", nil, session(cookie))
	expect(t, r, 200, "list gatherings")
	var list []map[string]any
	_ = json.Unmarshal([]byte(r.raw), &list)
	if len(list) != 1 {
		t.Fatalf("expected one gathering, got %s", r.raw)
	}
	return list[0]["my_status"].(string)
}

// End-to-end coverage of the essential member and admin routes.
func TestEssentialRoutes(t *testing.T) {
	e := newEnv(t, nil, 4)
	expect(t, e.do("GET", "/health", nil), 200, "health")
	expect(t, e.do("GET", "/ready", nil), 200, "ready")

	adminC, _ := e.admin("admin@example.test")
	a := e.signup("a@example.test")
	b := e.signup("b@example.test")
	c := e.signup("c@example.test")

	me := e.do("GET", "/v1/me", nil, session(a))
	expect(t, me, 200, "me")
	if me.body["status"] != "pending" || me.body["email"] != "a@example.test" {
		t.Fatalf("me: %s", me.raw)
	}
	expect(t, e.do("GET", "/v1/gatherings", nil, session(a)), 403, "pending member blocked from gatherings")
	expect(t, e.do("GET", "/v1/admin/members", nil, session(a)), 403, "member blocked from admin")
	expect(t, e.do("GET", "/v1/me", nil), 401, "anonymous me")

	for _, em := range []string{"a@example.test", "b@example.test", "c@example.test"} {
		expect(t, e.do("PATCH", "/v1/admin/members/"+e.userID(em), map[string]any{"status": "approved"}, session(adminC)), 200, "approve")
	}
	expect(t, e.do("PATCH", "/v1/admin/members/00000000-0000-0000-0000-000000000000", map[string]any{"status": "approved"}, session(adminC)), 404, "approve missing member")

	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Qahwa morning", "type": "majlis", "startsOn": "2030-02-01", "startsAt": "10:00", "place": "Saadiyat", "capacity": 1}, session(adminC)), 201, "create gathering")
	expect(t, e.do("POST", "/v1/admin/gatherings", map[string]any{"title": "Bad", "startsOn": "nope", "startsAt": "10:00", "place": "x", "capacity": 1}, session(adminC)), 400, "invalid gathering")
	var gid string
	_ = e.db.QueryRow(`SELECT id::text FROM gatherings`).Scan(&gid)

	k := e.do("POST", "/v1/gatherings/"+gid+"/keep", nil, session(a))
	expect(t, k, 200, "a keeps")
	if k.body["status"] != "confirmed" {
		t.Fatalf("a should be confirmed: %s", k.raw)
	}
	if e.do("POST", "/v1/gatherings/"+gid+"/keep", nil, session(b)).body["status"] != "waitlist" {
		t.Fatal("b should be waitlisted")
	}
	if e.do("POST", "/v1/gatherings/"+gid+"/keep", nil, session(c)).body["status"] != "waitlist" {
		t.Fatal("c should be waitlisted")
	}
	expect(t, e.do("POST", "/v1/gatherings/not-a-uuid/keep", nil, session(a)), 404, "bad gathering id")

	expect(t, e.do("POST", "/v1/admin/checkin", map[string]any{"gatheringID": gid, "userID": e.userID("b@example.test"), "attended": true}, session(adminC)), 409, "check-in of waitlisted member")
	expect(t, e.do("POST", "/v1/admin/checkin", map[string]any{"gatheringID": gid, "userID": e.userID("a@example.test"), "attended": true}, session(adminC)), 200, "check-in confirmed member")

	// Cancel promotes b.
	expect(t, e.do("POST", "/v1/gatherings/"+gid+"/cancel", nil, session(a)), 200, "a cancels")
	expect(t, e.do("POST", "/v1/gatherings/"+gid+"/cancel", nil, session(a)), 404, "cancel twice")
	if s := myStatus(t, e, b); s != "confirmed" {
		t.Fatalf("b should be promoted, got %q", s)
	}
	// Account deletion promotes c.
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": "wrong-password-000"}, session(b)), 401, "deletion needs the password")
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": goodPass}, session(b)), 200, "b deletes account")
	if s := myStatus(t, e, c); s != "confirmed" {
		t.Fatalf("c should be promoted after deletion, got %q", s)
	}
	expect(t, e.do("GET", "/v1/me", nil, session(b)), 401, "deleted session")
	// Declining c frees the seat; a rejoins and is confirmed.
	expect(t, e.do("PATCH", "/v1/admin/members/"+e.userID("c@example.test"), map[string]any{"status": "declined"}, session(adminC)), 200, "decline c")
	if e.do("POST", "/v1/gatherings/"+gid+"/keep", nil, session(a)).body["status"] != "confirmed" {
		t.Fatal("a should get the seat freed by the decline")
	}

	expect(t, e.do("POST", "/v1/admin/announcements", map[string]any{"title": "Welcome", "body": "First season"}, session(adminC)), 201, "announcement")
	expect(t, e.do("GET", "/v1/announcements", nil, session(a)), 200, "list announcements")
	expect(t, e.do("POST", "/v1/admin/partners", map[string]any{"name": "Cafe"}, session(adminC)), 201, "partner")
	expect(t, e.do("GET", "/v1/partners", nil, session(a)), 200, "list partners")
	expect(t, e.do("POST", "/v1/reports", map[string]any{"id": e.userID("admin@example.test"), "reason": "a test report reason"}, session(a)), 201, "report")
	expect(t, e.do("POST", "/v1/blocks", map[string]any{"id": e.userID("a@example.test")}, session(a)), 404, "members cannot create blocks directly (admin-mediated)")
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": "Noor", "displayName": "N", "interests": []string{"wellness"}}, session(a)), 200, "update profile")
	expect(t, e.do("DELETE", "/v1/admin/gatherings/"+gid, nil, session(adminC)), 200, "delete gathering")
	expect(t, e.do("DELETE", "/v1/admin/gatherings/"+gid, nil, session(adminC)), 404, "delete gathering twice")

	expect(t, e.do("POST", "/v1/logout", nil, session(a)), 200, "logout")
	expect(t, e.do("GET", "/v1/me", nil, session(a)), 401, "after logout")
}

func TestLoginErrorsAreGeneric(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.signup("known@example.test")
	wrong := e.do("POST", "/v1/login", map[string]any{"email": "known@example.test", "password": "wrong-password-123"})
	missing := e.do("POST", "/v1/login", map[string]any{"email": "nobody@example.test", "password": "wrong-password-123"})
	expect(t, wrong, 401, "wrong password")
	expect(t, missing, 401, "unknown account")
	if wrong.body["error"] != missing.body["error"] || wrong.body["code"] != missing.body["code"] {
		t.Fatalf("errors differ: %s vs %s", wrong.raw, missing.raw)
	}
	ok := e.do("POST", "/v1/login", map[string]any{"email": " KNOWN@example.test ", "password": goodPass})
	expect(t, ok, 200, "login")
	if ok.cookie == "" {
		t.Fatal("no session cookie")
	}
	dup := e.do("POST", "/v1/signup", map[string]any{"email": "known@example.test", "password": goodPass, "firstName": "X"})
	expect(t, dup, 202, "duplicate signup looks like a new one")
	expect(t, e.do("POST", "/v1/signup", map[string]any{"email": "short@example.test", "password": "short", "firstName": "X"}), 400, "short password")
}

func TestRepeatedFailedLoginsLockAccount(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.signup("target@example.test")
	for i := 0; i < 5; i++ {
		expect(t, e.do("POST", "/v1/login", map[string]any{"email": "target@example.test", "password": "guess-number-xyz"}, from("203.0.113."+string(rune('1'+i)))), 401, "failed attempt")
	}
	// Even the correct password, from a new IP, is refused while locked.
	locked := e.do("POST", "/v1/login", map[string]any{"email": "target@example.test", "password": goodPass}, from("203.0.113.99"))
	expect(t, locked, 429, "locked account")
	if locked.header.Get("Retry-After") == "" || locked.body["code"] != "rate_limited" {
		t.Fatalf("missing retry info: %v %s", locked.header, locked.raw)
	}
	// Unknown accounts lock identically, so the lock reveals nothing.
	for i := 0; i < 5; i++ {
		e.do("POST", "/v1/login", map[string]any{"email": "ghost@example.test", "password": "guess-number-xyz"})
	}
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "ghost@example.test", "password": "guess-number-xyz"}), 429, "unknown account lock")

	e.mr.FastForward(16 * time.Minute)
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "target@example.test", "password": goodPass}), 200, "lock expires")
}

func TestSuccessfulLoginResetsFailures(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.signup("reset@example.test")
	for i := 0; i < 4; i++ {
		e.do("POST", "/v1/login", map[string]any{"email": "reset@example.test", "password": "bad-password-000"})
	}
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "reset@example.test", "password": goodPass}), 200, "login")
	for i := 0; i < 5; i++ {
		expect(t, e.do("POST", "/v1/login", map[string]any{"email": "reset@example.test", "password": "bad-password-000"}), 401, "counter was reset")
	}
}

func TestPerIPLimitsAndSpoofing(t *testing.T) {
	e := newEnv(t, func(l *httpapi.Limits) {
		l.LoginPerIP.Limit = 3
		l.SignupPerIP.Limit = 2
	}, 4)
	for i := 0; i < 3; i++ {
		expect(t, e.do("POST", "/v1/login", map[string]any{"email": "x" + string(rune('a'+i)) + "@example.test", "password": "pw-not-valid-123"}, from("203.0.113.5")), 401, "within IP limit")
	}
	// Spoofed forwarding headers from an untrusted peer do not create a new bucket.
	spoof := e.do("POST", "/v1/login", map[string]any{"email": "y@example.test", "password": "pw-not-valid-123"},
		from("203.0.113.5"), header("X-Forwarded-For", "1.1.1.1"), header("X-Hive-Client-IP", "2.2.2.2"), header("X-Real-IP", "3.3.3.3"))
	expect(t, spoof, 429, "spoofed headers ignored")

	// The trusted Netlify proxy (shared secret) passes distinct client IPs.
	for _, client := range []string{"192.0.2.10", "192.0.2.11"} {
		r := e.do("POST", "/v1/login", map[string]any{"email": "z@example.test", "password": "pw-not-valid-123"},
			from("10.0.0.2"), header("X-Hive-Proxy-Secret", proxySecret), header("X-Hive-Client-IP", client))
		expect(t, r, 401, "proxied client "+client)
	}
	// A wrong secret is treated as the proxy's own address, not the claimed client.
	for i := 0; i < 3; i++ {
		e.do("POST", "/v1/login", map[string]any{"email": "w@example.test", "password": "pw-not-valid-123"},
			from("10.0.0.2"), header("X-Hive-Proxy-Secret", "wrong"), header("X-Hive-Client-IP", "192.0.2."+string(rune('1'+i))))
	}
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "w@example.test", "password": "pw-not-valid-123"},
		from("10.0.0.2"), header("X-Hive-Proxy-Secret", "wrong"), header("X-Hive-Client-IP", "192.0.2.200")), 429, "wrong secret")

	e.rawSignup("s1@example.test", from("203.0.113.8"))
	e.rawSignup("s2@example.test", from("203.0.113.8"))
	expect(t, e.do("POST", "/v1/signup", map[string]any{"email": "s3@example.test", "password": goodPass, "firstName": "S"}, from("203.0.113.8")), 429, "signup IP limit")
	expect(t, e.do("POST", "/v1/verify-email", map[string]any{"token": e.lastToken("s1@example.test", "verify_email")}), 200, "verify s1")

	e.mr.FastForward(11 * time.Minute)
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "s1@example.test", "password": goodPass}, from("203.0.113.5")), 200, "IP login limit expired")
}

func TestRedisOutageKeepsLimits(t *testing.T) {
	e := newEnv(t, nil, 4)
	e.signup("outage@example.test")
	e.mr.Close()
	for i := 0; i < 5; i++ {
		expect(t, e.do("POST", "/v1/login", map[string]any{"email": "outage@example.test", "password": "bad-password-000"}), 401, "failure during outage")
	}
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "outage@example.test", "password": goodPass}), 429, "fallback lock during outage")
	r := e.do("GET", "/ready", nil)
	expect(t, r, 200, "ready while Redis is down")
	if r.body["redis"] != "degraded" {
		t.Fatalf("readiness should report degraded Redis: %s", r.raw)
	}
}

// A burst larger than the hashing capacity is shed with 503, never queued
// without bound, and never produces a 500.
func TestConcurrentLoginBurst(t *testing.T) {
	e := newEnv(t, func(l *httpapi.Limits) { l.HashWait = 30 * time.Millisecond }, 1)
	e.signup("burst@example.test")
	const n = 16
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes <- e.do("POST", "/v1/login", map[string]any{"email": "burst" + string(rune('a'+i)) + "@example.test", "password": "pw-not-valid-123"}).code
		}(i)
	}
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for c := range codes {
		counts[c]++
	}
	if counts[500] != 0 || counts[503] == 0 || counts[401] == 0 || counts[401]+counts[503] != n {
		t.Fatalf("unexpected status mix: %v", counts)
	}
}

func TestRequestHardening(t *testing.T) {
	e := newEnv(t, nil, 4)
	big := strings.Repeat("a", 8000)
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": big, "password": big}), 413, "oversized body")
	expect(t, e.do("POST", "/v1/login", map[string]any{"email": "a@b.c", "password": "x"}, header("Origin", "https://evil.example")), 403, "foreign origin")
	r := e.do("GET", "/v1/nothing-here", nil)
	expect(t, r, 404, "unknown route")
	if r.body["request_id"] == nil {
		t.Fatal("errors should carry a request id")
	}
}
