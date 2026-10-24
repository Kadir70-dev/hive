package mfa

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// RFC 6238 Appendix B vectors for SHA-1 (8-digit values; we use the last 6).
func TestRFC6238Vectors(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	cases := map[int64]string{59: "94287082", 1111111109: "07081804", 1111111111: "14050471", 1234567890: "89005924", 2000000000: "69279037", 20000000000: "65353130"}
	for ts, want := range cases {
		got, err := Code(secret, time.Unix(ts, 0))
		if err != nil {
			t.Fatal(err)
		}
		if got != want[2:] {
			t.Errorf("t=%d: got %s want %s", ts, got, want[2:])
		}
	}
}

func TestValidateSkewAndReplay(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_800_000_000, 0)
	code, _ := Code(secret, now)
	s, ok := Validate(secret, code, now, 0)
	if !ok {
		t.Fatal("current code rejected")
	}
	if _, ok := Validate(secret, code, now, s); ok {
		t.Fatal("replayed code accepted")
	}
	prev, _ := Code(secret, now.Add(-30*time.Second))
	if _, ok := Validate(secret, prev, now, 0); !ok {
		t.Fatal("previous step should be accepted for clock skew")
	}
	old, _ := Code(secret, now.Add(-90*time.Second))
	if _, ok := Validate(secret, old, now, 0); ok {
		t.Fatal("code three steps old accepted")
	}
	if _, ok := Validate(secret, "12345", now, 0); ok {
		t.Fatal("short code accepted")
	}
}

func TestCipherBindsUser(t *testing.T) {
	c, err := NewCipher("MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=")
	if err != nil {
		t.Fatal(err)
	}
	ct, _ := c.Seal("SECRET", "user-a")
	if pt, err := c.Open(ct, "user-a"); err != nil || pt != "SECRET" {
		t.Fatalf("roundtrip: %q %v", pt, err)
	}
	if _, err := c.Open(ct, "user-b"); err == nil {
		t.Fatal("ciphertext must not open for another user")
	}
	if _, err := NewCipher("too-short"); err == nil {
		t.Fatal("short key accepted")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, _ := RecoveryCodes(8)
	seen := map[string]bool{}
	for _, c := range codes {
		if len(c) != 14 || seen[c] {
			t.Fatalf("bad code %q", c)
		}
		seen[c] = true
		if HashRecoveryCode(strings.ToUpper(c)) != HashRecoveryCode(strings.ReplaceAll(c, "-", "")) {
			t.Fatal("hash must ignore case and dashes")
		}
	}
	if !strings.HasPrefix(URI("ABC", "a@b.c", "The Hive Society"), "otpauth://totp/") {
		t.Fatal("bad URI")
	}
}
