package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/argon2"
)

type server struct {
	db     *sql.DB
	secure bool
	origin string
}
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
type key int

const userKey key = 1

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		log.Fatal(e)
	}
	db.SetMaxOpenConns(12)
	db.SetConnMaxLifetime(time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		log.Fatal(e)
	}
	s := &server{db: db, secure: os.Getenv("COOKIE_SECURE") != "false", origin: os.Getenv("PUBLIC_ORIGIN")}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer, middleware.RequestID)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]any{"ok": true}) })
	r.Route("/v1", func(r chi.Router) {
		r.Post("/signup", s.signup)
		r.Post("/login", s.login)
		r.Post("/logout", s.logout)
		r.Post("/founding-signups", s.foundingSignup)
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Get("/me", s.me)
			r.Patch("/me", s.updateMe)
			r.Delete("/me", s.deleteMe)
			r.Get("/gatherings", s.gatherings)
			r.Post("/gatherings/{id}/keep", s.keep)
			r.Post("/gatherings/{id}/cancel", s.cancel)
			r.Get("/announcements", s.announcements)
			r.Get("/partners", s.partners)
			r.Get("/blocks", s.blocks)
			r.Post("/blocks", s.block)
			r.Delete("/blocks/{id}", s.unblock)
			r.Post("/reports", s.report)
			r.Route("/admin", func(r chi.Router) {
				r.Use(s.admin)
				r.Get("/members", s.members)
				r.Patch("/members/{id}", s.memberStatus)
				r.Get("/gatherings", s.adminGatherings)
				r.Post("/gatherings", s.createGathering)
				r.Delete("/gatherings/{id}", s.deleteGathering)
				r.Get("/gatherings/{id}/attendees", s.attendees)
				r.Post("/checkin", s.checkin)
				r.Get("/reports", s.reports)
				r.Post("/announcements", s.createAnnouncement)
				r.Delete("/announcements/{id}", s.removeAnnouncement)
				r.Post("/partners", s.createPartner)
				r.Delete("/partners/{id}", s.removePartner)
			})
		})
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(http.ListenAndServe(":"+port, s.originMiddleware(r)))
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]any{"error": msg})
}
func parse(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		fail(w, 400, "invalid JSON")
		return false
	}
	return true
}
func (s *server) originMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" {
			origin := r.Header.Get("Origin")
			if s.origin != "" && origin != "" && origin != s.origin {
				fail(w, 403, "origin denied")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	p := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return fmt.Sprintf("%s.%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(p)), nil
}
func verifyPassword(stored, password string) bool {
	part := strings.Split(stored, ".")
	if len(part) != 2 {
		return false
	}
	salt, e := base64.RawStdEncoding.DecodeString(part[0])
	if e != nil || len(salt) != 16 {
		return false
	}
	expected, e := base64.RawStdEncoding.DecodeString(part[1])
	if e != nil || len(expected) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, expected) == 1
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	_, e := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), e
}
func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *server) newSession(w http.ResponseWriter, r *http.Request, id string) error {
	token, e := randomToken()
	if e != nil {
		return e
	}
	_, e = s.db.ExecContext(r.Context(), "INSERT INTO sessions(id_hash,user_id,expires_at) VALUES($1,$2,now()+interval '14 days')", digest(token), id)
	if e != nil {
		return e
	}
	http.SetCookie(w, &http.Cookie{Name: "hive_session", Value: token, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: 14 * 86400})
	return nil
}
func (s *server) current(r *http.Request) (*user, error) {
	cookie, e := r.Cookie("hive_session")
	if e != nil {
		return nil, e
	}
	var u user
	var interestsJSON string
	e = s.db.QueryRowContext(r.Context(), `SELECT u.id::text,u.email,u.first_name,u.display_name,u.phone,u.area,array_to_json(u.interests)::text,u.status,u.is_admin,u.email_updates,u.avatar_path FROM users u JOIN sessions s ON u.id=s.user_id WHERE s.id_hash=$1 AND s.expires_at>now()`, digest(cookie.Value)).Scan(&u.ID, &u.Email, &u.FirstName, &u.DisplayName, &u.Phone, &u.Area, &interestsJSON, &u.Status, &u.IsAdmin, &u.EmailUpdates, &u.AvatarPath)
	if e == nil {
		_ = json.Unmarshal([]byte(interestsJSON), &u.Interests)
	}
	return &u, e
}

type pgStringArray []string

func (a *pgStringArray) Scan(src any) error {
	if src == nil {
		*a = []string{}
		return nil
	}
	switch v := src.(type) {
	case []string:
		*a = v
		return nil
	case string:
		return json.Unmarshal([]byte(v), a)
	default:
		return fmt.Errorf("unknown array type %T", src)
	}
}
func (s *server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, e := s.current(r)
		if e != nil {
			fail(w, 401, "authentication required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}
func own(r *http.Request) *user { return r.Context().Value(userKey).(*user) }
func (s *server) admin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !own(r).IsAdmin {
			fail(w, 403, "admin required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *server) signup(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Email, Password, FirstName, Phone, Area, DisplayName string
		Interests                                            []string
		EmailUpdates                                         bool
	}
	if !parse(w, r, &v) {
		return
	}
	v.Email = strings.ToLower(strings.TrimSpace(v.Email))
	if !strings.Contains(v.Email, "@") || len(v.Password) < 12 || len(v.Password) > 128 || len(strings.TrimSpace(v.FirstName)) == 0 || len(v.Email) > 254 {
		fail(w, 400, "invalid signup data (password needs 12+ characters)")
		return
	}
	hash, e := hashPassword(v.Password)
	if e != nil {
		fail(w, 500, "unable to register")
		return
	}
	var id string
	e = s.db.QueryRowContext(r.Context(), `INSERT INTO users(email,password_hash,first_name,phone,area,interests,display_name,email_updates) VALUES($1,$2,$3,NULLIF($4,''),$5,$6,$7,$8) RETURNING id::text`, v.Email, hash, v.FirstName, v.Phone, v.Area, v.Interests, v.DisplayName, v.EmailUpdates).Scan(&id)
	if e != nil {
		fail(w, 409, "account cannot be created")
		return
	}
	if e = s.newSession(w, r, id); e != nil {
		fail(w, 500, "session creation failed")
		return
	}
	reply(w, 201, map[string]any{"ok": true, "status": "pending"})
}
func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var v struct{ Email, Password string }
	if !parse(w, r, &v) {
		return
	}
	var id, hash string
	e := s.db.QueryRowContext(r.Context(), "SELECT id::text,password_hash FROM users WHERE email=$1", strings.ToLower(strings.TrimSpace(v.Email))).Scan(&id, &hash)
	if e != nil || !verifyPassword(hash, v.Password) {
		fail(w, 401, "invalid credentials")
		return
	}
	if e = s.newSession(w, r, id); e != nil {
		fail(w, 500, "session failed")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, e := r.Cookie("hive_session"); e == nil {
		_, _ = s.db.ExecContext(r.Context(), "DELETE FROM sessions WHERE id_hash=$1", digest(cookie.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: "hive_session", Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) me(w http.ResponseWriter, r *http.Request) { reply(w, 200, own(r)) }
func (s *server) updateMe(w http.ResponseWriter, r *http.Request) {
	var v struct {
		FirstName, DisplayName, Phone, Area string
		Interests                           []string
		EmailUpdates                        bool
	}
	if !parse(w, r, &v) {
		return
	}
	if len(v.FirstName) < 1 || len(v.FirstName) > 60 || len(v.DisplayName) > 60 || len(v.Phone) > 30 || len(v.Interests) > 30 {
		fail(w, 400, "invalid profile")
		return
	}
	_, e := s.db.ExecContext(r.Context(), `UPDATE users SET first_name=$1,display_name=$2,phone=NULLIF($3,''),area=$4,interests=$5,email_updates=$6 WHERE id=$7`, v.FirstName, v.DisplayName, v.Phone, v.Area, v.Interests, v.EmailUpdates, own(r).ID)
	if e != nil {
		fail(w, 500, "profile update failed")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) deleteMe(w http.ResponseWriter, r *http.Request) {
	_, e := s.db.ExecContext(r.Context(), "DELETE FROM users WHERE id=$1", own(r).ID)
	if e != nil {
		fail(w, 500, "deletion failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "hive_session", Path: "/", MaxAge: -1, Secure: s.secure, HttpOnly: true})
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) foundingSignup(w http.ResponseWriter, r *http.Request) {
	var v struct{ Name, Email, Phone, Why, Lang string }
	if !parse(w, r, &v) {
		return
	}
	if len(v.Name) < 1 || len(v.Email) > 254 || !strings.Contains(v.Email, "@") {
		fail(w, 400, "invalid data")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO founding_signups(name,email,phone,why,lang) VALUES($1,$2,$3,$4,$5)", v.Name, strings.ToLower(v.Email), v.Phone, v.Why, v.Lang)
	if e != nil {
		fail(w, 500, "signup failed")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
func (s *server) members(w http.ResponseWriter, r *http.Request) {
	rows, e := s.db.QueryContext(r.Context(), "SELECT id::text,email,first_name,status,is_admin FROM users ORDER BY created_at DESC LIMIT 500")
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, email, name, status string
		var admin bool
		if rows.Scan(&id, &email, &name, &status, &admin) == nil {
			out = append(out, map[string]any{"id": id, "email": email, "first_name": name, "status": status, "is_admin": admin})
		}
	}
	reply(w, 200, out)
}
func (s *server) memberStatus(w http.ResponseWriter, r *http.Request) {
	var v struct{ Status string }
	if !parse(w, r, &v) {
		return
	}
	if v.Status != "pending" && v.Status != "approved" && v.Status != "declined" {
		fail(w, 400, "invalid status")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "UPDATE users SET status=$1 WHERE id=$2 AND is_admin=false", v.Status, chi.URLParam(r, "id"))
	if e != nil {
		fail(w, 500, "update failed")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func approved(w http.ResponseWriter, r *http.Request) bool {
	if own(r).Status != "approved" && !own(r).IsAdmin {
		fail(w, 403, "approval required")
		return false
	}
	return true
}
func (s *server) gatherings(w http.ResponseWriter, r *http.Request) {
	if !approved(w, r) {
		return
	}
	rows, e := s.db.QueryContext(r.Context(), `SELECT g.id::text,g.title,g.type,g.description,g.starts_on::text,g.starts_at::text,g.place,g.capacity - (SELECT count(*) FROM reservations WHERE gathering_id=g.id AND status='confirmed') AS places_left, COALESCE((SELECT status FROM reservations WHERE gathering_id=g.id AND user_id=$1),'') FROM gatherings g ORDER BY g.starts_on,g.starts_at LIMIT 200`, own(r).ID)
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place, status string
		var left int
		if rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &left, &status) == nil {
			out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place, "places_left": left, "my_status": status})
		}
	}
	reply(w, 200, out)
}
func (s *server) keep(w http.ResponseWriter, r *http.Request)   { s.reserve(w, r, true) }
func (s *server) cancel(w http.ResponseWriter, r *http.Request) { s.reserve(w, r, false) }
func (s *server) reserve(w http.ResponseWriter, r *http.Request, keep bool) {
	if !approved(w, r) {
		return
	}
	tx, e := s.db.BeginTx(r.Context(), nil)
	if e != nil {
		fail(w, 500, "database unavailable")
		return
	}
	defer tx.Rollback()
	id := chi.URLParam(r, "id")
	var cap int
	e = tx.QueryRowContext(r.Context(), "SELECT capacity FROM gatherings WHERE id=$1 FOR UPDATE", id).Scan(&cap)
	if e != nil {
		fail(w, 404, "gathering not found")
		return
	}
	if keep {
		var count int
		e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'", id).Scan(&count)
		if e != nil {
			fail(w, 500, "query failed")
			return
		}
		status := "waitlist"
		if count < cap {
			status = "confirmed"
		}
		_, e = tx.ExecContext(r.Context(), "INSERT INTO reservations(gathering_id,user_id,status) VALUES($1,$2,$3) ON CONFLICT(gathering_id,user_id) DO NOTHING", id, own(r).ID, status)
	} else {
		_, e = tx.ExecContext(r.Context(), "DELETE FROM reservations WHERE gathering_id=$1 AND user_id=$2", id, own(r).ID)
		if e == nil {
			var count int
			e = tx.QueryRowContext(r.Context(), "SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'", id).Scan(&count)
			if e == nil && count < cap {
				_, e = tx.ExecContext(r.Context(), `UPDATE reservations SET status='confirmed' WHERE id=(SELECT id FROM reservations WHERE gathering_id=$1 AND status='waitlist' ORDER BY created_at,id LIMIT 1)`, id)
			}
		}
	}
	if e != nil {
		fail(w, 500, "reservation failed")
		return
	}
	if e = tx.Commit(); e != nil {
		fail(w, 500, "reservation commit failed")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) announcements(w http.ResponseWriter, r *http.Request) {
	s.listPublic(w, r, "announcements", "id::text,title,body,created_at::text")
}
func (s *server) partners(w http.ResponseWriter, r *http.Request) {
	s.listPublic(w, r, "partners", "id::text,name,description,logo_url,website_url")
}
func (s *server) listPublic(w http.ResponseWriter, r *http.Request, table, columns string) {
	if !approved(w, r) {
		return
	}
	rows, e := s.db.QueryContext(r.Context(), "SELECT "+columns+" FROM "+table+" WHERE published=true ORDER BY created_at DESC LIMIT 200")
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, a, b, c, d string
		if table == "announcements" {
			if rows.Scan(&id, &a, &b, &c) == nil {
				out = append(out, map[string]any{"id": id, "title": a, "body": b, "created_at": c})
			}
		} else {
			if rows.Scan(&id, &a, &b, &c, &d) == nil {
				out = append(out, map[string]any{"id": id, "name": a, "description": b, "logo_url": c, "website_url": d})
			}
		}
	}
	reply(w, 200, out)
}
func (s *server) blocks(w http.ResponseWriter, r *http.Request) {
	if !approved(w, r) {
		return
	}
	rows, e := s.db.QueryContext(r.Context(), "SELECT blocked_id::text FROM member_blocks WHERE blocker_id=$1", own(r).ID)
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			out = append(out, id)
		}
	}
	reply(w, 200, out)
}
func (s *server) block(w http.ResponseWriter, r *http.Request) {
	if !approved(w, r) {
		return
	}
	var v struct{ ID string }
	if !parse(w, r, &v) {
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO member_blocks(blocker_id,blocked_id) VALUES($1,$2) ON CONFLICT DO NOTHING", own(r).ID, v.ID)
	if e != nil {
		fail(w, 400, "invalid member reference")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) unblock(w http.ResponseWriter, r *http.Request) {
	_, e := s.db.ExecContext(r.Context(), "DELETE FROM member_blocks WHERE blocker_id=$1 AND blocked_id=$2", own(r).ID, chi.URLParam(r, "id"))
	if e != nil {
		fail(w, 400, "invalid member reference")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) report(w http.ResponseWriter, r *http.Request) {
	if !approved(w, r) {
		return
	}
	var v struct{ ID, Reason string }
	if !parse(w, r, &v) {
		return
	}
	if len(v.Reason) < 10 || len(v.Reason) > 2000 {
		fail(w, 400, "reason must be 10 to 2000 characters")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO member_reports(reporter_id,reported_member_id,reason) VALUES($1,$2,$3)", own(r).ID, v.ID, v.Reason)
	if e != nil {
		fail(w, 400, "invalid member reference")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
func (s *server) reports(w http.ResponseWriter, r *http.Request) {
	rows, e := s.db.QueryContext(r.Context(), "SELECT id::text,reporter_id::text,COALESCE(reported_member_id::text,''),reason,status FROM member_reports ORDER BY created_at DESC LIMIT 200")
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, reporter, target, reason, status string
		if rows.Scan(&id, &reporter, &target, &reason, &status) == nil {
			out = append(out, map[string]any{"id": id, "reporter_id": reporter, "reported_member_id": target, "reason": reason, "status": status})
		}
	}
	reply(w, 200, out)
}
func (s *server) adminGatherings(w http.ResponseWriter, r *http.Request) {
	rows, e := s.db.QueryContext(r.Context(), "SELECT id::text,title,type,description,starts_on::text,starts_at::text,place,capacity FROM gatherings ORDER BY starts_on")
	if e != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place string
		var cap int
		if rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &cap) == nil {
			out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place, "capacity": cap})
		}
	}
	reply(w, 200, out)
}
func (s *server) createGathering(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Title, Type, Description, StartsOn, StartsAt, Place string
		Capacity                                            int
	}
	if !parse(w, r, &v) {
		return
	}
	if v.Capacity < 1 || v.Title == "" {
		fail(w, 400, "invalid gathering")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO gatherings(title,type,description,starts_on,starts_at,place,capacity) VALUES($1,$2,$3,$4,$5,$6,$7)", v.Title, v.Type, v.Description, v.StartsOn, v.StartsAt, v.Place, v.Capacity)
	if e != nil {
		fail(w, 400, "invalid gathering date or time")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
func (s *server) deleteGathering(w http.ResponseWriter, r *http.Request) {
	_, e := s.db.ExecContext(r.Context(), "DELETE FROM gatherings WHERE id=$1", chi.URLParam(r, "id"))
	if e != nil {
		fail(w, 400, "invalid id")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) attendees(w http.ResponseWriter, r *http.Request) {
	rows, e := s.db.QueryContext(r.Context(), `SELECT u.id::text,u.first_name,u.email,r.status,COALESCE(a.attended,false) FROM reservations r JOIN users u ON u.id=r.user_id LEFT JOIN attendance a ON a.gathering_id=r.gathering_id AND a.user_id=u.id WHERE r.gathering_id=$1 ORDER BY r.created_at`, chi.URLParam(r, "id"))
	if e != nil {
		fail(w, 400, "query failed")
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, name, email, status string
		var attended bool
		if rows.Scan(&id, &name, &email, &status, &attended) == nil {
			out = append(out, map[string]any{"user_id": id, "first_name": name, "email": email, "status": status, "attended": attended})
		}
	}
	reply(w, 200, out)
}
func (s *server) checkin(w http.ResponseWriter, r *http.Request) {
	var v struct {
		GatheringID, UserID string
		Attended            bool
	}
	if !parse(w, r, &v) {
		return
	}
	_, e := s.db.ExecContext(r.Context(), `INSERT INTO attendance(gathering_id,user_id,attended,checked_at) SELECT $1,$2,$3,now() WHERE EXISTS(SELECT 1 FROM reservations WHERE gathering_id=$1 AND user_id=$2 AND status='confirmed') ON CONFLICT(gathering_id,user_id) DO UPDATE SET attended=excluded.attended,checked_at=now()`, v.GatheringID, v.UserID, v.Attended)
	if e != nil {
		fail(w, 400, "invalid checkin")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
func (s *server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	var v struct{ Title, Body string }
	if !parse(w, r, &v) {
		return
	}
	if v.Title == "" || v.Body == "" {
		fail(w, 400, "title and body required")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO announcements(title,body) VALUES($1,$2)", v.Title, v.Body)
	if e != nil {
		fail(w, 500, "insert failed")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
func (s *server) removeAnnouncement(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "announcements")
}
func (s *server) createPartner(w http.ResponseWriter, r *http.Request) {
	var v struct{ Name, Description, LogoURL, WebsiteURL string }
	if !parse(w, r, &v) {
		return
	}
	if v.Name == "" {
		fail(w, 400, "name required")
		return
	}
	_, e := s.db.ExecContext(r.Context(), "INSERT INTO partners(name,description,logo_url,website_url) VALUES($1,$2,$3,$4)", v.Name, v.Description, v.LogoURL, v.WebsiteURL)
	if e != nil {
		fail(w, 500, "insert failed")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
func (s *server) removePartner(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "partners")
}
func (s *server) deleteByID(w http.ResponseWriter, r *http.Request, table string) {
	_, e := s.db.ExecContext(r.Context(), "DELETE FROM "+table+" WHERE id=$1", chi.URLParam(r, "id"))
	if e != nil {
		fail(w, 400, "invalid id")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

var _ = errors.New
var _ = strconv.Atoi
