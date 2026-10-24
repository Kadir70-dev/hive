// Package password hashes and verifies member passwords with Argon2id and
// caps how many of those memory-intensive operations run at the same time.
package password

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters (OWASP recommended range). Each hash uses Memory KiB.
const (
	Time    uint32 = 3
	Memory  uint32 = 64 * 1024
	Threads uint8  = 2
	KeyLen  uint32 = 32
	SaltLen        = 16
)

// ErrBusy is returned when no hashing slot became free before the context ended.
var ErrBusy = errors.New("password hashing capacity exhausted")

// Hasher bounds concurrent Argon2id work so request bursts cannot exhaust memory.
type Hasher struct {
	slots chan struct{}
	dummy string
}

// NewHasher allows at most concurrency simultaneous hash/verify operations.
func NewHasher(concurrency int) *Hasher {
	if concurrency < 1 {
		concurrency = 1
	}
	h := &Hasher{slots: make(chan struct{}, concurrency)}
	// A fixed hash used to equalise timing when an account does not exist.
	h.dummy, _ = hash("not-a-real-password-timing-equaliser")
	return h
}

func (h *Hasher) acquire(ctx context.Context) error {
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ErrBusy
	}
}

func (h *Hasher) release() { <-h.slots }

// Hash derives a new encoded hash. It blocks until a slot is free or ctx ends.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	return hash(password)
}

// Verify checks password against stored. An empty stored value runs a dummy
// verification so unknown accounts take the same time as known ones.
func (h *Hasher) Verify(ctx context.Context, stored, password string) (bool, error) {
	if err := h.acquire(ctx); err != nil {
		return false, err
	}
	defer h.release()
	if stored == "" {
		verify(h.dummy, password)
		return false, nil
	}
	return verify(stored, password), nil
}

func hash(password string) (string, error) {
	salt := make([]byte, SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, Time, Memory, Threads, KeyLen)
	return fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

// verify accepts the existing "salt.key" storage format used since launch.
func verify(stored, password string) bool {
	parts := strings.Split(stored, ".")
	if len(parts) != 2 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil || len(salt) != SaltLen {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil || len(expected) != int(KeyLen) {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, Time, Memory, Threads, KeyLen)
	return subtle.ConstantTimeCompare(got, expected) == 1
}
