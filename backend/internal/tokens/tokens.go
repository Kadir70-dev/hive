// Package tokens creates the single-use, hashed, expiring tokens used for
// email verification, password reset and member invitations.
package tokens

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"
)

const (
	PurposeVerify = "verify_email"
	PurposeReset  = "reset_password" // also used for import invitations
)

// Execer is satisfied by *sql.DB and *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Random returns 256 random bits, base64url encoded.
func Random() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Digest is the stored form of a token (raw tokens are never stored).
func Digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Issue invalidates earlier unused tokens of the same purpose and stores a new
// hashed one. The raw token is returned only so it can be emailed.
func Issue(ctx context.Context, q Execer, userID, purpose string, ttl time.Duration) (string, error) {
	if _, err := q.ExecContext(ctx, `UPDATE email_tokens SET used_at=now() WHERE user_id=$1 AND purpose=$2 AND used_at IS NULL`, userID, purpose); err != nil {
		return "", err
	}
	token, err := Random()
	if err != nil {
		return "", err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO email_tokens(token_hash,user_id,purpose,expires_at) VALUES($1,$2,$3,now()+$4::interval)`,
		Digest(token), userID, purpose, fmt.Sprintf("%d seconds", int(ttl.Seconds())))
	return token, err
}
