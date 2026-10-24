package httpapi

import (
	"net/http"
	"strings"

	"thehivesociety/api/internal/seats"
)

func (s *Server) me(w http.ResponseWriter, r *http.Request) { reply(w, 200, own(r)) }

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var v struct {
		FirstName, DisplayName, Phone, Area string
		Interests                           []string
		EmailUpdates                        bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.FirstName = strings.TrimSpace(v.FirstName)
	if v.FirstName == "" || len(v.FirstName) > 60 || len(v.DisplayName) > 60 || len(v.Phone) > 30 || len(v.Area) > 80 || len(v.Interests) > 30 {
		s.fail(w, r, 400, "invalid_profile", "invalid profile")
		return
	}
	if v.Interests == nil {
		v.Interests = []string{}
	}
	_, err := s.db.ExecContext(r.Context(), `UPDATE users SET first_name=$1,display_name=$2,phone=NULLIF($3,''),area=$4,interests=$5,email_updates=$6 WHERE id=$7`,
		v.FirstName, v.DisplayName, v.Phone, v.Area, v.Interests, v.EmailUpdates, own(r).ID)
	if err != nil {
		s.internal(w, r, "profile update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

// deleteMe releases the member's seats (promoting the waitlist) and deletes the
// account in one transaction; sessions, blocks and attendance cascade.
func (s *Server) deleteMe(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	defer tx.Rollback()
	if err = seats.ReleaseUser(r.Context(), tx, own(r).ID); err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	if _, err = tx.ExecContext(r.Context(), "DELETE FROM users WHERE id=$1", own(r).ID); err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	s.setSessionCookie(w, "", -1)
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) foundingSignup(w http.ResponseWriter, r *http.Request) {
	var v struct{ Name, Email, Phone, Why, Lang string }
	if !s.parse(w, r, &v) {
		return
	}
	v.Email = normalizeEmail(v.Email)
	if v.Name == "" || len(v.Name) > 120 || len(v.Email) > 254 || !strings.Contains(v.Email, "@") || len(v.Phone) > 30 || len(v.Why) > 2000 {
		s.fail(w, r, 400, "invalid_data", "invalid data")
		return
	}
	if v.Lang != "ar" {
		v.Lang = "en"
	}
	_, err := s.db.ExecContext(r.Context(), "INSERT INTO founding_signups(name,email,phone,why,lang) VALUES($1,$2,$3,$4,$5)", v.Name, v.Email, v.Phone, v.Why, v.Lang)
	if err != nil {
		s.internal(w, r, "signup failed", err)
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}

func (s *Server) announcements(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,title,body,created_at::text FROM announcements WHERE published=true ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, body, created string
		if err := rows.Scan(&id, &title, &body, &created); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "body": body, "created_at": created})
	}
	reply(w, 200, out)
}

func (s *Server) partners(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,name,description,logo_url,website_url FROM partners WHERE published=true ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, name, desc, logo, site string
		if err := rows.Scan(&id, &name, &desc, &logo, &site); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "name": name, "description": desc, "logo_url": logo, "website_url": site})
	}
	reply(w, 200, out)
}

func (s *Server) blocks(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT blocked_id::text FROM member_blocks WHERE blocker_id=$1", own(r).ID)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, id)
	}
	reply(w, 200, out)
}

func (s *Server) block(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	var v struct{ ID string }
	if !s.parse(w, r, &v) {
		return
	}
	if !validUUID(v.ID) || v.ID == own(r).ID {
		s.fail(w, r, 400, "invalid_member", "invalid member reference")
		return
	}
	res, err := s.db.ExecContext(r.Context(), "INSERT INTO member_blocks(blocker_id,blocked_id) SELECT $1,id FROM users WHERE id=$2 ON CONFLICT DO NOTHING", own(r).ID, v.ID)
	if err != nil {
		s.internal(w, r, "block failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var exists bool
		_ = s.db.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM member_blocks WHERE blocker_id=$1 AND blocked_id=$2)", own(r).ID, v.ID).Scan(&exists)
		if !exists {
			s.fail(w, r, 400, "invalid_member", "invalid member reference")
			return
		}
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) unblock(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "DELETE FROM member_blocks WHERE blocker_id=$1 AND blocked_id=$2", own(r).ID, id); err != nil {
		s.internal(w, r, "unblock failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	var v struct{ ID, Reason string }
	if !s.parse(w, r, &v) {
		return
	}
	if len(v.Reason) < 10 || len(v.Reason) > 2000 {
		s.fail(w, r, 400, "invalid_reason", "reason must be 10 to 2000 characters")
		return
	}
	if !validUUID(v.ID) || v.ID == own(r).ID {
		s.fail(w, r, 400, "invalid_member", "invalid member reference")
		return
	}
	res, err := s.db.ExecContext(r.Context(), "INSERT INTO member_reports(reporter_id,reported_member_id,reason) SELECT $1,id,$3 FROM users WHERE id=$2", own(r).ID, v.ID, v.Reason)
	if err != nil {
		s.internal(w, r, "report failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 400, "invalid_member", "invalid member reference")
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}
