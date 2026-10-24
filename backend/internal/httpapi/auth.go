package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
)

const sessionCookie = "hive_session"

var errNoSession = errors.New("no valid session")

type user struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	FirstName    string   `json:"first_name"`
	DisplayName  string   `json:"display_name"`
	Phone        *string  `json:"phone"`
	Area         string   `json:"area"`
	Interests    []string `json:"interests"`
	Status       string   `json:"status"`
	IsAdmin      bool     `json:"isAdmin"`
	EmailUpdates bool     `json:"email_updates"`
	AvatarPath   *string  `json:"avatar_path"`
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

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
	var u user
	var interestsJSON string
	err = s.db.QueryRowContext(r.Context(), `SELECT u.id::text,u.email,u.first_name,u.display_name,u.phone,u.area,array_to_json(u.interests)::text,u.status,u.is_admin,u.email_updates,u.avatar_path FROM users u JOIN sessions s ON u.id=s.user_id WHERE s.id_hash=$1 AND s.expires_at>now()`, digest(cookie.Value)).
		Scan(&u.ID, &u.Email, &u.FirstName, &u.DisplayName, &u.Phone, &u.Area, &interestsJSON, &u.Status, &u.IsAdmin, &u.EmailUpdates, &u.AvatarPath)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
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

func (s *Server) signup(w http.ResponseWriter, r *http.Request) {
	if res := s.limiter.Hit(r.Context(), s.limits.SignupPerIP, ratelimit.Key(s.ips.IP(r))); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var v struct {
		Email, Password, FirstName, Phone, Area, DisplayName string
		Interests                                            []string
		EmailUpdates                                         bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.Email = normalizeEmail(v.Email)
	v.FirstName = strings.TrimSpace(v.FirstName)
	if !strings.Contains(v.Email, "@") || len(v.Email) > 254 || len(v.Password) < 12 || len(v.Password) > 128 ||
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
	var id string
	err = s.db.QueryRowContext(r.Context(), `INSERT INTO users(email,password_hash,first_name,phone,area,interests,display_name,email_updates) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8) ON CONFLICT (email) DO NOTHING RETURNING id::text`,
		v.Email, hash, v.FirstName, v.Phone, v.Area, v.Interests, v.DisplayName, v.EmailUpdates).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 409, "signup_failed", "account cannot be created")
		return
	}
	if err != nil {
		s.internal(w, r, "unable to register", err)
		return
	}
	if err = s.newSession(w, r, id); err != nil {
		s.internal(w, r, "session creation failed", err)
		return
	}
	reply(w, 201, map[string]any{"ok": true, "status": "pending"})
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
	err := s.db.QueryRowContext(r.Context(), "SELECT id::text,password_hash FROM users WHERE email=$1", email).Scan(&id, &hash)
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
	if err = s.newSession(w, r, id); err != nil {
		s.internal(w, r, "session failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
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
