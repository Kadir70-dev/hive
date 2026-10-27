package httpapi_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func cmsDoc(title string) map[string]any {
	return map[string]any{
		"en":  map[string]any{"heroTitle": title, "faq": [][]string{{"QA-TEST question?", "QA-TEST answer."}}},
		"ar":  map[string]any{"heroLine": "QA-TEST سطر"},
		"seo": map[string]any{"title": "QA-TEST title", "robots": "index", "canonical": "/"},
	}
}

func publicTitle(t *testing.T, e *env) any {
	t.Helper()
	r := e.do("GET", "/v1/cms/pages/home", nil)
	expect(t, r, 200, "public page")
	return r.body["document"].(map[string]any)["en"].(map[string]any)["heroTitle"]
}

// Draft never changes the public page; publish does; restore brings back an old version.
func TestCmsDraftPublishRestore(t *testing.T) {
	e := newEnvWith(t, nil, 4, true)
	adminC, _ := e.admin("studio-admin@example.test")
	member := e.signup("studio-member@example.test")

	empty := e.do("GET", "/v1/cms/pages/home", nil)
	expect(t, empty, 200, "public before any content")
	if publicTitle(t, e) != nil {
		t.Fatalf("expected no overrides: %s", empty.raw)
	}
	expect(t, e.do("GET", "/v1/cms/pages/nope", nil), 404, "unknown page")
	expect(t, e.do("GET", "/v1/admin/cms/pages/home", nil, session(member)), 403, "member blocked from studio")
	expect(t, e.do("GET", "/v1/admin/cms/pages/home", nil), 401, "anonymous blocked from studio")
	expect(t, e.do("PUT", "/v1/admin/cms/pages/home/draft", map[string]any{"document": cmsDoc("x"), "expected_revision": 0}, session(member)), 403, "member cannot save")

	save := func(title string, rev float64, want int) resp {
		r := e.do("PUT", "/v1/admin/cms/pages/home/draft", map[string]any{"document": cmsDoc(title), "expected_revision": rev}, session(adminC))
		expect(t, r, want, "save draft "+title)
		return r
	}
	r := save("QA-TEST one <em>first</em>", 0, 200)
	if r.body["draft_revision"].(float64) != 1 {
		t.Fatalf("revision: %s", r.raw)
	}
	if publicTitle(t, e) != nil {
		t.Fatal("saving a draft changed the public page")
	}
	save("stale", 0, 409)
	expect(t, e.do("POST", "/v1/admin/cms/pages/home/publish", map[string]any{"expected_revision": 0}, session(adminC)), 409, "publish of an unseen revision")
	expect(t, e.do("POST", "/v1/admin/cms/pages/home/publish", map[string]any{"expected_revision": 1}, session(adminC)), 200, "publish v1")
	if publicTitle(t, e) != "QA-TEST one <em>first</em>" {
		t.Fatal("publish did not reach the public page")
	}

	save("QA-TEST two", 1, 200)
	expect(t, e.do("POST", "/v1/admin/cms/pages/home/publish", map[string]any{"expected_revision": 2}, session(adminC)), 200, "publish v2")
	if publicTitle(t, e) != "QA-TEST two" {
		t.Fatal("second publish not live")
	}
	h := e.do("GET", "/v1/admin/cms/pages/home/versions", nil, session(adminC))
	expect(t, h, 200, "history")
	if n := len(h.body["data"].([]any)); n != 2 {
		t.Fatalf("history: %s", h.raw)
	}
	expect(t, e.do("POST", "/v1/admin/cms/pages/home/restore", map[string]any{"version": 1}, session(adminC)), 200, "restore v1")
	if publicTitle(t, e) != "QA-TEST one <em>first</em>" {
		t.Fatal("restore did not republish version 1")
	}
	expect(t, e.do("POST", "/v1/admin/cms/pages/home/restore", map[string]any{"version": 99}, session(adminC)), 404, "restore missing version")
	page := e.do("GET", "/v1/admin/cms/pages/home", nil, session(adminC))
	expect(t, page, 200, "admin page")
	if page.body["draft_revision"].(float64) != 3 || page.body["published_revision"].(float64) != 3 {
		t.Fatalf("revisions after restore: %s", page.raw)
	}
	var audits int
	_ = e.db.QueryRow(`SELECT count(*) FROM audit_events WHERE action LIKE 'cms.%'`).Scan(&audits)
	if audits != 5 {
		t.Fatalf("expected 5 cms audit events, got %d", audits)
	}
}

func TestCmsRejectsUnsafeContent(t *testing.T) {
	e := newEnvWith(t, nil, 4, true)
	adminC, _ := e.admin("studio-admin@example.test")
	bad := []map[string]any{
		{"en": map[string]any{"heroTitle": "<img src=x onerror=alert(1)>"}},
		{"en": map[string]any{"heroTitle": "<em onclick=x>hi</em>"}},
		{"en": map[string]any{"visionImg1": "javascript:alert(1)"}},
		{"en": map[string]any{"visionImg1": "https://example.test/a.jpg\") ;background:red"}},
		{"en": map[string]any{"visionImg1": "//evil.test/a.jpg"}},
		{"en": map[string]any{"langBtn": "x"}},
		{"en": map[string]any{"bad-key": "x"}},
		{"en": map[string]any{"faq": [][]string{{"only one"}}}},
		{"en": map[string]any{"heroTitle": 42}},
		{"seo": map[string]any{"robots": "nofollow"}},
		{"seo": map[string]any{"canonical": "https://evil.test/"}},
		{"seo": map[string]any{"title": strings.Repeat("a", 141)}},
	}
	for i, doc := range bad {
		r := e.do("PUT", "/v1/admin/cms/pages/home/draft", map[string]any{"document": doc, "expected_revision": 0}, session(adminC))
		if r.code != 422 && r.code != 400 {
			t.Fatalf("bad doc %d accepted: %d %s", i, r.code, r.raw)
		}
	}
	ok := map[string]any{"en": map[string]any{"visionImg1": "/api/backend/cms/media/00000000-0000-0000-0000-000000000000", "visionImg2": "https://images.example.test/a.jpg"}}
	expect(t, e.do("PUT", "/v1/admin/cms/pages/home/draft", map[string]any{"document": ok, "expected_revision": 0}, session(adminC)), 200, "safe image URLs")
}

func (e *env) cmsUpload(cookie string, data []byte) resp {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "QA TEST majlis.photo.jpg")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/admin/cms/media", &body)
	req.RemoteAddr = "198.51.100.1:40000"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "hive_session", Value: cookie})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	out := resp{code: rec.Code, header: rec.Header(), raw: rec.Body.String()}
	return out
}

func TestCmsMediaLibrary(t *testing.T) {
	e := newEnvWith(t, nil, 4, true)
	adminC, _ := e.admin("studio-admin@example.test")
	member := e.signup("studio-member@example.test")
	photo := photoJPEG(t, 3000, 1500)

	expect(t, e.cmsUpload(member, photo), 403, "member upload")
	expect(t, e.cmsUpload(adminC, []byte("not an image")), 400, "non-image upload")
	up := e.cmsUpload(adminC, photo)
	expect(t, up, 201, "admin upload")
	var id string
	_ = e.db.QueryRow(`SELECT id::text FROM cms_media`).Scan(&id)
	if !strings.Contains(up.raw, `"/api/backend/cms/media/`+id+`"`) || !strings.Contains(up.raw, `"width":2400`) {
		t.Fatalf("upload reply: %s", up.raw)
	}

	code, h, data := e.fetch("/v1/cms/media/"+id, "")
	if code != 200 || h.Get("Content-Type") != "image/jpeg" || !strings.Contains(h.Get("Cache-Control"), "immutable") {
		t.Fatalf("serve: %d %v", code, h)
	}
	if bytes.Contains(data, []byte("Exif")) || bytes.Contains(data, []byte("GPS")) {
		t.Fatal("stored website image still carries EXIF metadata")
	}
	if c, _, _ := e.fetch("/v1/cms/media/not-an-id", ""); c != 404 {
		t.Fatalf("bad id: %d", c)
	}

	expect(t, e.do("PATCH", "/v1/admin/cms/media/"+id, map[string]any{"alt_text": "QA-TEST majlis at dusk"}, session(adminC)), 200, "alt text")
	expect(t, e.do("PATCH", "/v1/admin/cms/media/"+id, map[string]any{"alt_text": "two\nlines"}, session(adminC)), 422, "multi-line alt")
	expect(t, e.do("PATCH", "/v1/admin/cms/media/"+id, map[string]any{"alt_text": "x"}, session(member)), 403, "member alt edit")
	list := e.do("GET", "/v1/admin/cms/media", nil, session(adminC))
	expect(t, list, 200, "list media")
	if !strings.Contains(list.raw, "QA-TEST majlis at dusk") || !strings.Contains(list.raw, "QA-TEST-majlis-photo.jpg") || strings.Contains(list.raw, "\"data\":\"") {
		t.Fatalf("list: %s", list.raw)
	}
}
