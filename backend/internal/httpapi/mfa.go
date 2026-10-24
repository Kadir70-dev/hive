package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"thehivesociety/api/internal/mfa"
	"thehivesociety/api/internal/ratelimit"
)

// requireAdminAccount admits verified admins regardless of MFA state; used
// only by the MFA enrolment/verification endpoints themselves.
func (s *Server) requireAdminAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := own(r)
		if !u.IsAdmin || !u.EmailVerified {
			s.fail(w, r, 403, "forbidden", "admin required")
			return
		}
		if s.mfaCipher == nil {
			s.fail(w, r, 503, "mfa_unavailable", "two-factor authentication is not configured on the server")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	u := own(r)
	if u.MFAEnabled {
		s.fail(w, r, 409, "mfa_already_enabled", "two-factor authentication is already enabled")
		return
	}
	secret, err := mfa.GenerateSecret()
	if err != nil {
		s.internal(w, r, "mfa setup failed", err)
		return
	}
	enc, err := s.mfaCipher.Seal(secret, u.ID)
	if err == nil {
		_, err = s.db.ExecContext(r.Context(), `UPDATE users SET totp_pending_enc=$2 WHERE id=$1 AND totp_enabled_at IS NULL`, u.ID, enc)
	}
	if err != nil {
		s.internal(w, r, "mfa setup failed", err)
		return
	}
	reply(w, 200, map[string]any{"secret": secret, "otpauth_uri": mfa.URI(secret, u.Email, "The Hive Society")})
}

func (s *Server) mfaLimited(w http.ResponseWriter, r *http.Request) bool {
	if res := s.limiter.Peek(r.Context(), s.limits.MFAPerAccount, ratelimit.Key("mfa", own(r).ID)); !res.Allowed {
		s.tooMany(w, r, res)
		return true
	}
	return false
}

func (s *Server) mfaFailed(w http.ResponseWriter, r *http.Request) {
	s.limiter.Hit(r.Context(), s.limits.MFAPerAccount, ratelimit.Key("mfa", own(r).ID))
	s.fail(w, r, 401, "invalid_code", "that code is not valid")
}

func (s *Server) markSessionMFA(r *http.Request, q execQuerier) error {
	_, err := q.ExecContext(r.Context(), `UPDATE sessions SET mfa_verified_at=now() WHERE id_hash=$1`, own(r).sessionHash)
	return err
}

// mfaEnable confirms the pending secret with a code, enables TOTP, returns
// one-time recovery codes and marks this session as MFA-verified.
func (s *Server) mfaEnable(w http.ResponseWriter, r *http.Request) {
	if s.mfaLimited(w, r) {
		return
	}
	u := own(r)
	var v struct{ Code string }
	if !s.parse(w, r, &v) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "mfa enable failed", err)
		return
	}
	defer tx.Rollback()
	var pending []byte
	err = tx.QueryRowContext(r.Context(), `SELECT totp_pending_enc FROM users WHERE id=$1 AND totp_enabled_at IS NULL FOR UPDATE`, u.ID).Scan(&pending)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && pending == nil) {
		s.fail(w, r, 409, "mfa_setup_required", "start two-factor setup first")
		return
	}
	if err != nil {
		s.internal(w, r, "mfa enable failed", err)
		return
	}
	secret, err := s.mfaCipher.Open(pending, u.ID)
	if err != nil {
		s.internal(w, r, "mfa enable failed", err)
		return
	}
	step, ok := mfa.Validate(secret, v.Code, time.Now(), 0)
	if !ok {
		s.mfaFailed(w, r)
		return
	}
	codes, err := mfa.RecoveryCodes(10)
	if err != nil {
		s.internal(w, r, "mfa enable failed", err)
		return
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE users SET totp_secret_enc=totp_pending_enc, totp_pending_enc=NULL, totp_enabled_at=now(), totp_last_step=$2 WHERE id=$1`, u.ID, step)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM mfa_recovery_codes WHERE user_id=$1`, u.ID)
	}
	for _, c := range codes {
		if err != nil {
			break
		}
		_, err = tx.ExecContext(r.Context(), `INSERT INTO mfa_recovery_codes(user_id,code_hash) VALUES($1,$2)`, u.ID, mfa.HashRecoveryCode(c))
	}
	if err == nil {
		err = s.markSessionMFA(r, tx)
	}
	if err == nil {
		s.audit(r.Context(), tx, u.ID, "mfa.enabled", u.ID, "")
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "mfa enable failed", err)
		return
	}
	s.limiter.Reset(r.Context(), s.limits.MFAPerAccount, ratelimit.Key("mfa", u.ID))
	reply(w, 200, map[string]any{"ok": true, "recovery_codes": codes})
}

// mfaVerify step-up: accepts a TOTP code (never the same time step twice) or
// an unused recovery code.
func (s *Server) mfaVerify(w http.ResponseWriter, r *http.Request) {
	if s.mfaLimited(w, r) {
		return
	}
	u := own(r)
	if !u.MFAEnabled {
		s.fail(w, r, 409, "mfa_enrollment_required", "set up two-factor authentication first")
		return
	}
	var v struct{ Code, RecoveryCode string }
	if !s.parse(w, r, &v) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "mfa verification failed", err)
		return
	}
	defer tx.Rollback()
	method := "totp"
	if v.RecoveryCode != "" {
		method = "recovery_code"
		var res sql.Result
		res, err = tx.ExecContext(r.Context(), `UPDATE mfa_recovery_codes SET used_at=now() WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL`, u.ID, mfa.HashRecoveryCode(v.RecoveryCode))
		if err == nil {
			if n, _ := res.RowsAffected(); n == 0 {
				s.mfaFailed(w, r)
				return
			}
		}
	} else {
		var enc []byte
		var last sql.NullInt64
		err = tx.QueryRowContext(r.Context(), `SELECT totp_secret_enc, totp_last_step FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&enc, &last)
		var secret string
		if err == nil {
			secret, err = s.mfaCipher.Open(enc, u.ID)
		}
		if err == nil {
			step, ok := mfa.Validate(secret, v.Code, time.Now(), last.Int64)
			if !ok {
				s.mfaFailed(w, r)
				return
			}
			_, err = tx.ExecContext(r.Context(), `UPDATE users SET totp_last_step=$2 WHERE id=$1`, u.ID, step)
		}
	}
	if err == nil {
		err = s.markSessionMFA(r, tx)
	}
	if err == nil {
		s.audit(r.Context(), tx, u.ID, "mfa.verified", u.ID, method)
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "mfa verification failed", err)
		return
	}
	s.limiter.Reset(r.Context(), s.limits.MFAPerAccount, ratelimit.Key("mfa", u.ID))
	reply(w, 200, map[string]any{"ok": true})
}
