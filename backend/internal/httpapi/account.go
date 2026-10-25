package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"time"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/tokens"
)

const (
	purposeVerify = "verify_email"
	purposeReset  = "reset_password"
)

type execQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func issueToken(ctx context.Context, q execQuerier, userID, purpose string, ttl time.Duration) (string, error) {
	return tokens.Issue(ctx, q, userID, purpose, ttl)
}

// consumeToken atomically marks a valid token used and returns its owner.
func consumeToken(ctx context.Context, q execQuerier, token, purpose string) (userID, email string, err error) {
	if len(token) < 20 || len(token) > 100 {
		return "", "", sql.ErrNoRows
	}
	err = q.QueryRowContext(ctx, `
UPDATE email_tokens t SET used_at=now() FROM users u
WHERE t.token_hash=$1 AND t.purpose=$2 AND t.used_at IS NULL AND t.expires_at>now() AND u.id=t.user_id
RETURNING u.id::text, u.email`, digest(token), purpose).Scan(&userID, &email)
	return userID, email, err
}

func (s *Server) sendVerification(ctx context.Context, q execQuerier, userID, email, name string) error {
	token, err := issueToken(ctx, q, userID, purposeVerify, s.verifyTTL)
	if err != nil {
		return err
	}
	return mail.Enqueue(ctx, q, userID, mail.TemplateVerifyEmail, mail.VerifyEmail(email, name, s.appURL, token, int(s.verifyTTL.Hours())))
}

func (s *Server) audit(ctx context.Context, q execQuerier, actor, action, target, detail string) {
	var a, t any
	if actor != "" {
		a = actor
	}
	if target != "" {
		t = target
	}
	if _, err := q.ExecContext(ctx, `INSERT INTO audit_events(actor_id,action,target_id,detail) VALUES($1,$2,$3,NULLIF($4,''))`, a, action, t, detail); err != nil {
		s.log.Error("audit write failed", "action", action, "err", err)
	}
}

func (s *Server) tokenLimited(w http.ResponseWriter, r *http.Request) bool {
	if res := s.limiter.Hit(r.Context(), s.limits.TokenPerIP, ratelimit.Key(s.ips.IP(r))); !res.Allowed {
		s.tooMany(w, r, res)
		return true
	}
	return false
}

// verifyEmail confirms an address from the emailed link.
func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	if s.tokenLimited(w, r) {
		return
	}
	var v struct{ Token string }
	if !s.parse(w, r, &v) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "verification failed", err)
		return
	}
	defer tx.Rollback()
	uid, _, err := consumeToken(r.Context(), tx, v.Token, purposeVerify)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 400, "invalid_token", "this link is invalid or has expired; request a new one")
		return
	}
	if err == nil {
		var res sql.Result
		res, err = tx.ExecContext(r.Context(), `UPDATE users SET email_verified_at=now() WHERE id=$1 AND email_verified_at IS NULL`, uid)
		if err == nil {
			if n, _ := res.RowsAffected(); n == 1 {
				// Count a conversion only after the email is actually verified.
				_, err = tx.ExecContext(r.Context(), "UPDATE signup_attribution SET verified_at=now() WHERE user_id=$1 AND verified_at IS NULL", uid)
				if err == nil {
					// First confirmation: the member is now ready for review.
					err = s.notifyAdminsNewMember(r.Context(), tx, uid)
				}
			}
		}
	}
	if err == nil {
		s.audit(r.Context(), tx, uid, "email.verified", uid, "")
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "verification failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

// resendVerification always answers 202 so it cannot reveal accounts.
func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	if s.tokenLimited(w, r) {
		return
	}
	var v struct{ Email string }
	if !s.parse(w, r, &v) {
		return
	}
	email := normalizeEmail(v.Email)
	accepted := map[string]any{"ok": true, "status": "if_account_exists_email_sent"}
	if !validEmail(email) {
		reply(w, 202, accepted)
		return
	}
	if res := s.limiter.Hit(r.Context(), s.limits.EmailPerAccount, ratelimit.Key("verify", email)); !res.Allowed {
		reply(w, 202, accepted) // silently drop to protect the inbox owner
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "request failed", err)
		return
	}
	defer tx.Rollback()
	var id, name string
	err = tx.QueryRowContext(r.Context(), `SELECT id::text, first_name FROM users WHERE email=$1 AND email_verified_at IS NULL`, email).Scan(&id, &name)
	if err == nil {
		err = s.sendVerification(r.Context(), tx, id, email, name)
		if err == nil {
			err = tx.Commit()
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		s.internal(w, r, "request failed", err)
		return
	}
	reply(w, 202, accepted)
}

// forgotPassword emails a reset link; the response never reveals whether the
// address has an account.
func (s *Server) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if s.tokenLimited(w, r) {
		return
	}
	var v struct{ Email string }
	if !s.parse(w, r, &v) {
		return
	}
	email := normalizeEmail(v.Email)
	accepted := map[string]any{"ok": true, "status": "if_account_exists_email_sent"}
	if !validEmail(email) {
		reply(w, 202, accepted)
		return
	}
	if res := s.limiter.Hit(r.Context(), s.limits.EmailPerAccount, ratelimit.Key("reset", email)); !res.Allowed {
		reply(w, 202, accepted)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "request failed", err)
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `SELECT id::text FROM users WHERE email=$1`, email).Scan(&id)
	if err == nil {
		var token string
		token, err = issueToken(r.Context(), tx, id, purposeReset, s.resetTTL)
		if err == nil {
			err = mail.Enqueue(r.Context(), tx, id, mail.TemplateResetPassword, mail.ResetPassword(email, s.appURL, token, int(s.resetTTL.Minutes())))
		}
		if err == nil {
			s.audit(r.Context(), tx, "", "password.reset_requested", id, "")
			err = tx.Commit()
		}
	} else if errors.Is(err, sql.ErrNoRows) {
		err = nil
	}
	if err != nil {
		s.internal(w, r, "request failed", err)
		return
	}
	reply(w, 202, accepted)
}

// resetPassword sets a new password from a valid reset link and signs the
// account out everywhere by deleting all of its sessions.
func (s *Server) resetPassword(w http.ResponseWriter, r *http.Request) {
	if s.tokenLimited(w, r) {
		return
	}
	var v struct{ Token, Password string }
	if !s.parse(w, r, &v) {
		return
	}
	if len(v.Password) < 12 || len(v.Password) > 128 {
		s.fail(w, r, 400, "invalid_password", "password needs 12 to 128 characters")
		return
	}
	ctx, cancel := s.hashSlot(r)
	hash, err := s.hasher.Hash(ctx, v.Password)
	cancel()
	if errors.Is(err, password.ErrBusy) {
		s.busy(w, r)
		return
	}
	if err != nil {
		s.internal(w, r, "reset failed", err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "reset failed", err)
		return
	}
	defer tx.Rollback()
	uid, email, err := consumeToken(r.Context(), tx, v.Token, purposeReset)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 400, "invalid_token", "this link is invalid or has expired; request a new one")
		return
	}
	steps := []struct {
		q    string
		args []any
	}{
		// Possession of the inbox is proven, so the address counts as verified.
		{`UPDATE users SET password_hash=$2, password_changed_at=now(), email_verified_at=COALESCE(email_verified_at,now()) WHERE id=$1`, []any{uid, hash}},
		{`DELETE FROM sessions WHERE user_id=$1`, []any{uid}},
		{`UPDATE email_tokens SET used_at=now() WHERE user_id=$1 AND purpose='reset_password' AND used_at IS NULL`, []any{uid}},
	}
	for _, st := range steps {
		if err != nil {
			break
		}
		_, err = tx.ExecContext(r.Context(), st.q, st.args...)
	}
	if err == nil {
		err = mail.Enqueue(r.Context(), tx, uid, mail.TemplatePasswordChanged, mail.PasswordChanged(email))
	}
	if err == nil {
		s.audit(r.Context(), tx, uid, "password.reset", uid, "all sessions revoked")
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "reset failed", err)
		return
	}
	s.limiter.Reset(r.Context(), s.limits.LoginPerAccount, ratelimit.Key(email))
	s.setSessionCookie(w, "", -1)
	reply(w, 200, map[string]any{"ok": true})
}
