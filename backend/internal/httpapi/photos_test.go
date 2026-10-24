package httpapi_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"thehivesociety/api/internal/storage"
)

// photoJPEG makes a JPEG carrying an EXIF segment with a fake GPS marker.
func photoJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{63, 110, 94, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	payload := append([]byte("Exif\x00\x00II*\x00\x08\x00\x00\x00\x00\x00"), []byte("GPSLatitude=24.45")...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), append(seg, payload...)...), b[2:]...)
}

func (e *env) upload(cookie, field string, data []byte) resp {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile(field, "me.jpg")
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/v1/me/photo", &body)
	req.RemoteAddr = "198.51.100.1:40000"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", origin)
	req.AddCookie(&http.Cookie{Name: "hive_session", Value: cookie})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return resp{code: rec.Code, header: rec.Header(), raw: rec.Body.String()}
}

func (e *env) fetch(path, cookie string) (int, http.Header, []byte) {
	req := httptest.NewRequest("GET", path, nil)
	req.RemoteAddr = "198.51.100.1:40000"
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "hive_session", Value: cookie})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec.Code, rec.Header(), rec.Body.Bytes()
}

func avatarKey(t *testing.T, e *env, email string) string {
	t.Helper()
	var key *string
	_ = e.db.QueryRow(`SELECT avatar_path FROM users WHERE email=$1`, email).Scan(&key)
	if key == nil {
		return ""
	}
	return *key
}

func queued(t *testing.T, e *env) []string {
	t.Helper()
	rows, _ := e.db.Query(`SELECT object_key FROM storage_deletions ORDER BY object_key`)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		out = append(out, k)
	}
	return out
}

func photoLifecycle(t *testing.T, store storage.Store) {
	e := newEnvStore(t, nil, 4, true, store)
	adminC, _ := e.admin("owner@example.test")
	m := e.signup("photo@example.test")
	other := e.signup("other@example.test")

	r := e.upload(m, "photo", photoJPEG(t, 1600, 1200))
	expect(t, r, 200, "upload")
	key := avatarKey(t, e, "photo@example.test")
	if !strings.HasPrefix(key, "avatars/") || len(key) != len("avatars/")+32+len(".jpg") {
		t.Fatalf("object key should be random: %q", key)
	}
	me := e.do("GET", "/v1/me", nil, session(m))
	if me.body["has_photo"] != true || strings.Contains(me.raw, "avatars/") {
		t.Fatalf("/me must report has_photo without exposing the key: %s", me.raw)
	}
	code, hdr, img := e.fetch("/v1/me/photo", m)
	if code != 200 || hdr.Get("Content-Type") != "image/jpeg" || hdr.Get("Cache-Control") != "private, no-store" {
		t.Fatalf("own photo: %d %v", code, hdr)
	}
	if bytes.Contains(img, []byte("Exif")) || bytes.Contains(img, []byte("GPSLatitude")) {
		t.Fatal("stored photo still carries EXIF/GPS metadata")
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(img))
	if err != nil || cfg.Width != 512 || cfg.Height != 384 {
		t.Fatalf("stored photo dimensions: %v %dx%d", err, cfg.Width, cfg.Height)
	}

	// Access control: anonymous 401, other members have no route to it, admins can view.
	if code, _, _ := e.fetch("/v1/me/photo", ""); code != 401 {
		t.Fatalf("anonymous photo: %d", code)
	}
	if code, _, _ := e.fetch("/v1/admin/members/"+e.userID("photo@example.test")+"/photo", other); code != 403 {
		t.Fatalf("member reading another member's photo: %d", code)
	}
	if code, _, _ := e.fetch("/v1/me/photo", other); code != 404 {
		t.Fatalf("member without photo: %d", code)
	}
	if code, _, _ := e.fetch("/v1/admin/members/"+e.userID("photo@example.test")+"/photo", adminC); code != 200 {
		t.Fatalf("admin photo view: %d", code)
	}

	// Replace queues the old object; delete queues the new one.
	expect(t, e.upload(m, "photo", photoJPEG(t, 300, 300)), 200, "replace")
	if q := queued(t, e); len(q) != 1 || q[0] != key {
		t.Fatalf("old photo not queued for deletion: %v", q)
	}
	newKey := avatarKey(t, e, "photo@example.test")
	expect(t, e.do("DELETE", "/v1/me/photo", nil, session(m)), 200, "delete")
	expect(t, e.do("DELETE", "/v1/me/photo", nil, session(m)), 404, "delete twice")
	if q := queued(t, e); len(q) != 2 {
		t.Fatalf("deleted photo not queued: %v", q)
	}
	if e.do("GET", "/v1/me", nil, session(m)).body["has_photo"] != false {
		t.Fatal("has_photo should be false after removal")
	}

	// Account deletion queues the photo too.
	expect(t, e.upload(other, "photo", photoJPEG(t, 200, 200)), 200, "other upload")
	otherKey := avatarKey(t, e, "other@example.test")
	expect(t, e.do("DELETE", "/v1/me", map[string]any{"password": goodPass}, session(other)), 200, "delete account")
	found := false
	for _, k := range queued(t, e) {
		found = found || k == otherKey
	}
	if !found {
		t.Fatal("account deletion did not queue the photo for removal")
	}
	_ = newKey
}

func TestPhotoLifecycleInMemory(t *testing.T) { photoLifecycle(t, storage.NewMemory()) }

// The same lifecycle against a real S3-compatible server (SeaweedFS in CI).
func TestPhotoLifecycleS3(t *testing.T) {
	ep := os.Getenv("TEST_S3_ENDPOINT")
	if ep == "" {
		t.Skip("TEST_S3_ENDPOINT not set")
	}
	s3, err := storage.NewS3(storage.Config{Endpoint: ep, Bucket: "hive-test-photos", AccessKeyID: os.Getenv("TEST_S3_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("TEST_S3_SECRET_KEY"), Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s3.EnsureBucket(context.Background()); err != nil {
		t.Fatal(err)
	}
	photoLifecycle(t, s3)
}

func TestPhotoValidationAndLimits(t *testing.T) {
	store := storage.NewMemory()
	e := newEnvStore(t, nil, 4, true, store)
	m := e.signup("v@example.test")
	cases := []struct {
		name  string
		field string
		data  []byte
		code  int
	}{
		{"svg disguised", "photo", []byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), 400},
		{"html", "photo", []byte("<html><script>alert(1)</script></html>"), 400},
		{"truncated jpeg", "photo", photoJPEG(t, 50, 50)[:60], 400},
		{"wrong field", "file", photoJPEG(t, 50, 50), 400},
		{"over 5 MB", "photo", append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, 5<<20)...), 413},
	}
	for _, c := range cases {
		if r := e.upload(m, c.field, c.data); r.code != c.code {
			t.Errorf("%s: got %d want %d (%s)", c.name, r.code, c.code, r.raw)
		}
	}
	if len(store.Keys()) != 0 {
		t.Fatal("rejected uploads must not reach storage")
	}
	// Body far beyond the route limit is refused before parsing.
	req := httptest.NewRequest("POST", "/v1/me/photo", io.LimitReader(zeroReader{}, 7<<20))
	req.ContentLength = 7 << 20
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.AddCookie(&http.Cookie{Name: "hive_session", Value: m})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	if rec.Code != 413 {
		t.Fatalf("7 MB body: %d", rec.Code)
	}
	// Other routes keep the small JSON limit.
	expect(t, e.do("PATCH", "/v1/me", map[string]any{"firstName": strings.Repeat("a", 70000)}, session(m)), 413, "json limit unchanged")
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestPhotosDisabledWithoutStorage(t *testing.T) {
	e := newEnv(t, nil, 4)
	m := e.signup("d@example.test")
	r := e.upload(m, "photo", photoJPEG(t, 50, 50))
	if r.code != 503 || !strings.Contains(r.raw, "photos_unavailable") {
		t.Fatalf("no storage: %d %s", r.code, r.raw)
	}
}

func TestPhotoUploadRateLimitAndStorageFailure(t *testing.T) {
	store := storage.NewMemory()
	e := newEnvStore(t, nil, 4, true, store)
	m := e.signup("rl@example.test")
	store.FailPut = io.ErrUnexpectedEOF
	r := e.upload(m, "photo", photoJPEG(t, 60, 60))
	if r.code != 503 || avatarKey(t, e, "rl@example.test") != "" {
		t.Fatalf("storage failure must not change the profile: %d %s", r.code, r.raw)
	}
	store.FailPut = nil
	for i := 0; i < 9; i++ {
		expect(t, e.upload(m, "photo", photoJPEG(t, 60, 60)), 200, "upload")
	}
	expect(t, e.upload(m, "photo", photoJPEG(t, 60, 60)), 429, "11th upload in an hour")
}

func TestPhotoFlagsAndAdminListDoNotExposeStorageKeys(t *testing.T) {
	disabled := newEnv(t, nil, 4)
	memberSession := disabled.signup("disabled@example.test")
	me := disabled.do("GET", "/v1/me", nil, session(memberSession))
	expect(t, me, 200, "photo flags disabled")
	if me.body["photos_enabled"] != false || me.body["has_photo"] != false {
		t.Fatalf("disabled photo flags incorrect: %s", me.raw)
	}

	store := storage.NewMemory()
	e := newEnvStore(t, nil, 4, true, store)
	adminSession, _ := e.admin("owner@example.test")
	m := e.signup("photo-flags@example.test")
	expect(t, e.upload(m, "photo", photoJPEG(t, 100, 100)), 200, "photo flag upload")
	key := avatarKey(t, e, "photo-flags@example.test")
	me = e.do("GET", "/v1/me", nil, session(m))
	expect(t, me, 200, "photo flags enabled")
	if me.body["photos_enabled"] != true || me.body["has_photo"] != true || strings.Contains(me.raw, key) || strings.Contains(me.raw, "avatar_path") {
		t.Fatalf("photo status must reveal flags only, never storage keys: %s", me.raw)
	}
	list := e.do("GET", "/v1/admin/members", nil, session(adminSession))
	expect(t, list, 200, "admin members photo metadata")
	if !strings.Contains(list.raw, "\"has_photo\":true") || strings.Contains(list.raw, key) || strings.Contains(list.raw, "avatar_path") {
		t.Fatalf("admin list must only reveal has_photo: %s", list.raw)
	}
}
