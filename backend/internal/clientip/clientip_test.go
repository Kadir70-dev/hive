package clientip

import (
	"net/http/httptest"
	"testing"
)

const secret = "0123456789abcdef0123456789abcdef"

func TestResolver(t *testing.T) {
	res, err := New("172.28.10.10/32, 10.0.0.0/8", secret)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{"direct client", "203.0.113.7:5555", nil, "203.0.113.7"},
		{"spoofed XFF from untrusted peer is ignored", "203.0.113.7:5555",
			map[string]string{"X-Forwarded-For": "1.2.3.4"}, "203.0.113.7"},
		{"trusted proxy: right-most untrusted hop", "172.28.10.10:443",
			map[string]string{"X-Forwarded-For": "1.2.3.4, 198.51.100.9"}, "198.51.100.9"},
		{"trusted proxy chain skipped", "172.28.10.10:443",
			map[string]string{"X-Forwarded-For": "198.51.100.9, 10.1.2.3"}, "198.51.100.9"},
		{"garbage XFF falls back to peer", "172.28.10.10:443",
			map[string]string{"X-Forwarded-For": "not-an-ip"}, "172.28.10.10"},
		{"client IP header without secret is ignored", "203.0.113.7:5555",
			map[string]string{HeaderClientIP: "9.9.9.9"}, "203.0.113.7"},
		{"client IP header with wrong secret is ignored", "203.0.113.7:5555",
			map[string]string{HeaderClientIP: "9.9.9.9", HeaderProxySecret: "wrong"}, "203.0.113.7"},
		{"client IP header with secret is used", "172.28.10.10:443",
			map[string]string{HeaderClientIP: "9.9.9.9", HeaderProxySecret: secret, "X-Forwarded-For": "52.1.1.1"}, "9.9.9.9"},
		{"ipv6 peer", "[2001:db8::1]:443", nil, "2001:db8::1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("POST", "/v1/login", nil)
		r.RemoteAddr = c.remote
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := res.IP(r); got != c.want {
			t.Errorf("%s: got %s, want %s", c.name, got, c.want)
		}
	}
}

func TestNoSecretNeverTrustsClientHeader(t *testing.T) {
	res, _ := New("", "")
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "203.0.113.7:1"
	r.Header.Set(HeaderClientIP, "9.9.9.9")
	r.Header.Set(HeaderProxySecret, "")
	if got := res.IP(r); got != "203.0.113.7" {
		t.Fatalf("got %s", got)
	}
}

func TestInvalidCIDR(t *testing.T) {
	if _, err := New("not-a-cidr", ""); err == nil {
		t.Fatal("expected error")
	}
}
