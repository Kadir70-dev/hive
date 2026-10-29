package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/tokens"
)

const sessionCookie = "hive_session"

var errNoSession = errors.New("no valid session")

type user struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	FirstName     string   `json:"first_name"`
	DisplayName   string   `json:"display_name"`
	Phone         *string  `json:"phone"`
	Area          string   `json:"area"`
	Interests     []string `json:"interests"`
	Status        string   `json:"status"`
	IsAdmin       bool     `json:"isAdmin"`
	EmailUpdates  bool     `json:"email_updates"`
	AvatarPath    *string  `json:"-"` // storage key: never sent to clients
	HasPhoto      bool     `json:"has_photo"`
	PhotosEnabled bool     `json:"photos_enabled"`
	EmailVerified bool     `json:"email_verified"`
	MFAEnabled    bool     `json:"mfa_enabled"`
	MFAVerified   bool     `json:"mfa_verified"` // this session passed TOTP recently
	sessionHash   string
}

func randomToken() (string, error) { return tokens.Random() }

func digest(token string) string { return tokens.Digest(token) }

func normalizeEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

func (s *Server) setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: value, Path: "/", HttpOnly: true,
		Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func (s *Server) newSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := randomToken()
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(r.Context(), "INSERT INTO sessions(id_hash,user_id,expires_at) VALUES($1,$2,now()+interval '14 days')", digest(token), userID)
	if err != nil {
		return err
	}
	s.setSessionCookie(w, token, 14*86400)
	return nil
}

func (s *Server) current(r *http.Request) (*user, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil || cookie.Value == "" {
		return nil, errNoSession
	}
	u := user{sessionHash: digest(cookie.Value)}
	var interestsJSON string
	err = s.db.QueryRowContext(r.Context(), `
SELECT u.id::text,u.email,u.first_name,u.display_name,u.phone,u.area,array_to_json(u.interests)::text,u.status,u.is_admin,u.email_updates,u.avatar_path,
       u.email_verified_at IS NOT NULL, u.totp_enabled_at IS NOT NULL,
       COALESCE(s.mfa_verified_at > now() - $2::interval, false)
FROM users u JOIN sessions s ON u.id=s.user_id WHERE s.id_hash=$1 AND s.expires_at>now()`, u.sessionHash, s.mfaWindow).
		Scan(&u.ID, &u.Email, &u.FirstName, &u.DisplayName, &u.Phone, &u.Area, &interestsJSON, &u.Status, &u.IsAdmin, &u.EmailUpdates, &u.AvatarPath,
			&u.EmailVerified, &u.MFAEnabled, &u.MFAVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	u.HasPhoto = u.AvatarPath != nil
	_ = json.Unmarshal([]byte(interestsJSON), &u.Interests)
	if u.Interests == nil {
		u.Interests = []string{}
	}
	return &u, nil
}

// hashSlot bounds how long a request may queue for an Argon2id slot.
func (s *Server) hashSlot(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), s.limits.HashWait)
}

func (s *Server) busy(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Retry-After", "2")
	s.fail(w, r, 503, "busy", "the service is busy, please try again")
}

// signup creates an unverified account and emails a confirmation link. The
// response is identical whether or not the address already has an account
// (the owner of an existing account gets an "account exists" email instead),
// so signup cannot be used to discover members. No session is created until
// the address is confirmed and the member logs in.
func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if res := s.limiter.Hit(r.Context(), s.limits.SignupPerIP, ratelimit.Key(s.ips.IP(r))); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var v struct {
		Email        string
		Password     string
		FirstName    string
		Phone        string
		Area         string
		DisplayName  string
		Interests    []string
		EmailUpdates bool
		UTMSource    string `json:"utmSource"`
		UTMMedium    string `json:"utmMedium"`
		UTMCampaign  string `json:"utmCampaign"`
		UTMContent   string `json:"utmContent"`
		UTMTerm      string `json:"utmTerm"`
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.Email = normalizeEmail(v.Email)
	v.FirstName = strings.TrimSpace(v.FirstName)
	v.Area = strings.TrimSpace(v.Area)
	var phoneOK bool
	v.Phone, phoneOK = normalizeUAEMobile(v.Phone)
	if !phoneOK || v.Phone == "" {
		s.fail(w, r, 400, "invalid_phone", "UAE mobile number is required (e.g. +971 50 123 4567)")
		return
	}
	if !validAbuDhabiArea(v.Area) {
		s.fail(w, r, 400, "invalid_area", "choose an area in Abu Dhabi")
		return
	}
	if !validEmail(v.Email) || len(v.Password) < 12 || len(v.Password) > 128 ||
		v.FirstName == "" || len(v.FirstName) > 60 || len(v.DisplayName) > 60 || len(v.Phone) > 30 ||
		len(v.Area) > 80 || len(v.Interests) > 30 {
		s.fail(w, r, 400, "invalid_signup", "invalid signup data (password needs 12+ characters)")
		return
	}
	for _, i := range v.Interests {
		if len(i) > 60 {
			s.fail(w, r, 400, "invalid_signup", "invalid signup data")
			return
		}
	}
	if v.Interests == nil {
		v.Interests = []string{}
	}
	// UTM fields are optional first-party attribution hints, not trusted identifiers.
	for _, value := range []string{v.UTMSource, v.UTMMedium, v.UTMCampaign, v.UTMContent, v.UTMTerm} {
		if len(value) > 80 || strings.ContainsAny(value, "\r\n\x00") {
			s.fail(w, r, 400, "invalid_campaign", "invalid campaign data")
			return
		}
	}
	// Always hash, so new and existing addresses take the same time.
	ctx, cancel := s.hashSlot(r)
	hash, err := s.hasher.Hash(ctx, v.Password)
	cancel()
	if errors.Is(err, password.ErrBusy) {
		s.busy(w, r)
		return
	}
	if err != nil {
		s.internal(w, r, "unable to register", err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "unable to register", err)
		return
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(r.Context(), `INSERT INTO users(email,password_hash,first_name,phone,area,interests,display_name,email_updates) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8) ON CONFLICT (email) DO NOTHING RETURNING id::text`,
		v.Email, hash, v.FirstName, v.Phone, v.Area, v.Interests, v.DisplayName, v.EmailUpdates).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		// Existing account: notify its owner at most once an hour per address.
		if res := s.limiter.Hit(r.Context(), s.limits.ExistsNotice, ratelimit.Key(v.Email)); res.Allowed {
			// Link the notice to the existing account so deleting it cascades.
			var existing string
			if err = tx.QueryRowContext(r.Context(), `SELECT id::text FROM users WHERE email=$1`, v.Email).Scan(&existing); err == nil {
				err = mail.Enqueue(r.Context(), tx, existing, mail.TemplateAccountExists, mail.AccountExists(v.Email, s.appURL))
			}
		} else {
			err = nil
		}
	case err == nil:
		err = s.sendVerification(r.Context(), tx, id, v.Email, v.FirstName)
		if err == nil && (v.UTMSource != "" || v.UTMMedium != "" || v.UTMCampaign != "" || v.UTMContent != "" || v.UTMTerm != "") {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO signup_attribution(user_id,utm_source,utm_medium,utm_campaign,utm_content,utm_term) VALUES($1,$2,$3,$4,$5,$6)`, id, strings.TrimSpace(v.UTMSource), strings.TrimSpace(v.UTMMedium), strings.TrimSpace(v.UTMCampaign), strings.TrimSpace(v.UTMContent), strings.TrimSpace(v.UTMTerm))
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "unable to register", err)
		return
	}
	reply(w, 202, map[string]any{"ok": true, "status": "verification_sent"})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := s.ips.IP(r)
	if res := s.limiter.Hit(r.Context(), s.limits.LoginPerIP, ratelimit.Key(ip)); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var v struct{ Email, Password string }
	if !s.parse(w, r, &v) {
		return
	}
	email := normalizeEmail(v.Email)
	if email == "" || len(email) > 254 || v.Password == "" || len(v.Password) > 128 {
		s.fail(w, r, 401, "invalid_credentials", "invalid email or password")
		return
	}
	// Per-account lock applies whether or not the account exists, so it reveals nothing.
	acct := ratelimit.Key(email)
	if res := s.limiter.Peek(r.Context(), s.limits.LoginPerAccount, acct); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var id, hash string
	var verified, mfaEnabled bool
	err := s.db.QueryRowContext(r.Context(), "SELECT id::text,password_hash,email_verified_at IS NOT NULL,totp_enabled_at IS NOT NULL FROM users WHERE email=$1", email).
		Scan(&id, &hash, &verified, &mfaEnabled)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.internal(w, r, "login failed", err)
		return
	}
	ctx, cancel := s.hashSlot(r)
	ok, verr := s.hasher.Verify(ctx, hash, v.Password) // empty hash runs a timing-equal dummy check
	cancel()
	if errors.Is(verr, password.ErrBusy) {
		s.busy(w, r)
		return
	}
	if !ok || id == "" {
		if res := s.limiter.Hit(r.Context(), s.limits.LoginPerAccount, acct); !res.Allowed {
			s.tooMany(w, r, res)
			return
		}
		s.fail(w, r, 401, "invalid_credentials", "invalid email or password")
		return
	}
	s.limiter.Reset(r.Context(), s.limits.LoginPerAccount, acct)
	// Only reached with the correct password, so this reveals nothing to guessers.
	if !verified {
		s.fail(w, r, 403, "email_unverified", "please confirm your email address first; we can send a new link")
		return
	}
	if err = s.newSession(w, r, id); err != nil {
		s.internal(w, r, "session failed", err)
		return
	}
	s.audit(r.Context(), s.db, id, "login.success", id, "")
	reply(w, 200, map[string]any{"ok": true, "mfa_required": mfaEnabled})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookie); err == nil {
		if _, err := s.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id_hash=$1", digest(cookie.Value)); err != nil {
			s.internal(w, r, "logout failed", err)
			return
		}
	}
	s.setSessionCookie(w, "", -1)
	reply(w, 200, map[string]any{"ok": true})
}

func validEmail(e string) bool {
	at := strings.LastIndex(e, "@")
	return at > 0 && at < len(e)-3 && len(e) <= 254 && strings.Contains(e[at:], ".") && !strings.ContainsAny(e, " \t\r\n<>,;\"")
}
