// Package mfa implements TOTP (RFC 6238, HMAC-SHA1, 6 digits, 30 s steps —
// the format every authenticator app supports), encryption of TOTP secrets at
// rest, and single-use recovery codes.
package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	Period = 30
	Digits = 6
	Skew   = 1 // accept one step either side for clock drift
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateSecret returns a new 160-bit base32 secret.
func GenerateSecret() (string, error) {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return b32.EncodeToString(b), nil
}

// URI builds the otpauth:// URI that authenticator apps import.
func URI(secret, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {secret}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
	return "otpauth://totp/" + label + "?" + q.Encode()
}

func step(t time.Time) int64 { return t.Unix() / Period }

// codeAt returns the code for a time step.
func codeAt(secret string, counter int64) (string, error) {
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", errors.New("invalid TOTP secret")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(counter))
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1000000), nil
}

// Code returns the current code (used by tests and tooling).
func Code(secret string, t time.Time) (string, error) { return codeAt(secret, step(t)) }

// Validate checks code at time now. lastStep is the last accepted step for
// this account; codes at or before it are rejected to stop replay. It returns
// the matched step so the caller can persist it.
func Validate(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	code = strings.TrimSpace(code)
	if len(code) != Digits {
		return 0, false
	}
	cur := step(now)
	for s := cur - Skew; s <= cur+Skew; s++ {
		if s <= lastStep {
			continue
		}
		want, err := codeAt(secret, s)
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return s, true
		}
	}
	return 0, false
}

// Cipher encrypts TOTP secrets with AES-256-GCM.
type Cipher struct{ aead cipher.AEAD }

// NewCipher accepts a base64 (std or URL) encoded 32-byte key.
func NewCipher(encodedKey string) (*Cipher, error) {
	encodedKey = strings.TrimSpace(encodedKey)
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil {
		key, err = base64.RawURLEncoding.DecodeString(encodedKey)
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("MFA_ENCRYPTION_KEY must be 32 random bytes, base64 encoded")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Cipher{aead: aead}, nil
}

// Seal encrypts plaintext; the user id is bound as associated data so a
// ciphertext cannot be moved to another account.
func (c *Cipher) Seal(plaintext, userID string) ([]byte, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return c.aead.Seal(nonce, nonce, []byte(plaintext), []byte(userID)), nil
}

// Open decrypts a value produced by Seal.
func (c *Cipher) Open(ciphertext []byte, userID string) (string, error) {
	n := c.aead.NonceSize()
	if len(ciphertext) < n {
		return "", errors.New("ciphertext too short")
	}
	pt, err := c.aead.Open(nil, ciphertext[:n], ciphertext[n:], []byte(userID))
	if err != nil {
		return "", errors.New("cannot decrypt TOTP secret")
	}
	return string(pt), nil
}

// RecoveryCodes returns n codes like "k3f9-2m7x-q8w4" (60 bits of entropy each).
func RecoveryCodes(n int) ([]string, error) {
	const alphabet = "abcdefghjkmnpqrstuvwxyz23456789"
	out := make([]string, n)
	for i := range out {
		b := make([]byte, 12)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		var s strings.Builder
		for j, v := range b {
			if j > 0 && j%4 == 0 {
				s.WriteByte('-')
			}
			s.WriteByte(alphabet[int(v)%len(alphabet)])
		}
		out[i] = s.String()
	}
	return out, nil
}

// HashRecoveryCode normalises and hashes a recovery code for storage/lookup.
func HashRecoveryCode(code string) string {
	norm := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(code), "-", ""))
	sum := sha256.Sum256([]byte("hive-recovery:" + norm))
	return hex.EncodeToString(sum[:])
}
