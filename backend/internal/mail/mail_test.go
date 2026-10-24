package mail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResendSendsWithAuthAndIdempotency(t *testing.T) {
	var got map[string]any
	var auth, idem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, idem = r.Header.Get("Authorization"), r.Header.Get("Idempotency-Key")
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		if r.URL.Path != "/emails" || r.Method != http.MethodPost {
			w.WriteHeader(404)
			return
		}
		_, _ = w.Write([]byte(`{"id":"email_123"}`))
	}))
	defer srv.Close()
	r, err := NewResend("re_test_key", "The Hive Society <hello@mailer.example.test>", "hello@example.test")
	if err != nil {
		t.Fatal(err)
	}
	r.BaseURL = srv.URL
	id, err := r.Send(context.Background(), Message{To: "a@example.test", Subject: "S", Text: "T", HTML: "<p>H</p>"}, "hive-outbox-7")
	if err != nil || id != "email_123" {
		t.Fatalf("send: %q %v", id, err)
	}
	if auth != "Bearer re_test_key" || idem != "hive-outbox-7" {
		t.Fatalf("headers: %q %q", auth, idem)
	}
	if got["reply_to"] != "hello@example.test" || got["to"].([]any)[0] != "a@example.test" {
		t.Fatalf("payload: %v", got)
	}
}

func TestResendErrorClassification(t *testing.T) {
	for status, permanent := range map[int]bool{400: true, 401: true, 403: true, 422: true, 429: false, 500: false, 503: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"message":"nope"}`))
		}))
		r, _ := NewResend("re_x", "a@b.test", "")
		r.BaseURL = srv.URL
		_, err := r.Send(context.Background(), Message{To: "x@y.test"}, "k")
		srv.Close()
		if err == nil || IsPermanent(err) != permanent {
			t.Errorf("status %d: err=%v permanent=%v", status, err, IsPermanent(err))
		}
	}
	if _, err := NewResend("", "a@b.test", ""); err == nil {
		t.Fatal("missing key accepted")
	}
}

func TestTemplatesEscapeAndUseFragmentLinks(t *testing.T) {
	m := VerifyEmail("a@example.test", "<script>x</script>", "https://thehivesociety.ae/", "TOKEN123", 24)
	if strings.Contains(m.HTML, "<script>x") {
		t.Fatal("name must be HTML-escaped")
	}
	if !strings.Contains(m.Text, "https://thehivesociety.ae/verify-email#token=TOKEN123") {
		t.Fatalf("link must carry the token in the fragment: %s", m.Text)
	}
	if !strings.Contains(ResetPassword("a@example.test", "https://x.test", "T", 30).Text, "/reset-password#token=T") {
		t.Fatal("reset link")
	}
}

func TestBackoff(t *testing.T) {
	if Backoff(1) != 30*time.Second || Backoff(2) != time.Minute || Backoff(20) != time.Hour {
		t.Fatalf("backoff: %v %v %v", Backoff(1), Backoff(2), Backoff(20))
	}
	if !IsPermanent(&PermanentError{Err: errors.New("x")}) || IsPermanent(errors.New("x")) {
		t.Fatal("IsPermanent")
	}
}
