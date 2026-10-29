package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/seats"
)

// The photo capability is a non-secret feature flag. Never expose R2 object keys.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	u := *own(r)
	u.PhotosEnabled = s.store != nil
	reply(w, 200, u)
}

func (s *Server) updateMe(w http.ResponseWriter, r *http.Request) {
	var v struct {
		FirstName, DisplayName, Phone, Area string
		Interests                           []string
		EmailUpdates                        bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.FirstName, v.DisplayName = strings.TrimSpace(v.FirstName), strings.TrimSpace(v.DisplayName)
	v.Phone, v.Area = strings.TrimSpace(v.Phone), strings.TrimSpace(v.Area)
	if v.FirstName == "" || !validText(v.FirstName, 60, false) || !validText(v.DisplayName, 60, false) || !validText(v.Area, 80, false) {
		s.fail(w, r, 400, "invalid_profile", "first name is required; names up to 60 characters, area up to 80")
		return
	}
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
	interests := []string{}
	seen := map[string]bool{}
	for _, i := range v.Interests {
		i = strings.TrimSpace(i)
		if i == "" || seen[strings.ToLower(i)] {
			continue
		}
		if !validText(i, 40, false) {
			s.fail(w, r, 400, "invalid_interests", "interests can be up to 40 characters each")
			return
		}
		seen[strings.ToLower(i)] = true
		interests = append(interests, i)
	}
	if len(interests) > 10 {
		s.fail(w, r, 400, "invalid_interests", "choose up to 10 interests")
		return
	}
	_, err := s.db.ExecContext(r.Context(), `UPDATE users SET first_name=$1,display_name=$2,phone=NULLIF($3,''),area=$4,interests=$5,email_updates=$6 WHERE id=$7`,
		v.FirstName, v.DisplayName, v.Phone, v.Area, interests, v.EmailUpdates, own(r).ID)
	if err != nil {
		s.internal(w, r, "profile update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

// myGatherings lists the member's own reservations with their status and,
// when waitlisted, their position in the queue. Other members are never shown.
func (s *Server) myGatherings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `
SELECT g.id::text, g.title, g.starts_on::text, g.starts_at::text, g.place, r.status,
  CASE WHEN r.status='waitlist' THEN (SELECT count(*) FROM reservations w
       WHERE w.gathering_id=r.gathering_id AND w.status='waitlist' AND (w.created_at, w.id) <= (r.created_at, r.id)) END,
  ((g.starts_on + g.starts_at) AT TIME ZONE 'Asia/Dubai') <= now(),
  COALESCE(a.attended,false)
FROM reservations r JOIN gatherings g ON g.id=r.gathering_id
LEFT JOIN attendance a ON a.gathering_id=r.gathering_id AND a.user_id=r.user_id
WHERE r.user_id=$1 ORDER BY g.starts_on, g.starts_at`, own(r).ID)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, date, t, place, status string
		var position sql.NullInt64
		var past, attended bool
		if err := rows.Scan(&id, &title, &date, &t, &place, &status, &position, &past, &attended); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		row := map[string]any{"id": id, "title": title, "starts_on": date, "starts_at": t, "place": place, "status": status, "past": past, "attended": attended}
		if position.Valid {
			row["waitlist_position"] = position.Int64
		}
		out = append(out, row)
	}
	reply(w, 200, out)
}

// deleteMe requires the current password, then releases the member's seats
// (promoting and notifying the waitlist) and deletes the account in one
// transaction; sessions, tokens, blocks and attendance cascade.
func (s *Server) deleteMe(w http.ResponseWriter, r *http.Request) {
	var v struct{ Password string }
	if !s.parse(w, r, &v) {
		return
	}
	acct := ratelimit.Key(own(r).Email)
	if res := s.limiter.Peek(r.Context(), s.limits.LoginPerAccount, acct); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var hash string
	if err := s.db.QueryRowContext(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, own(r).ID).Scan(&hash); err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	ctx, cancel := s.hashSlot(r)
	ok, err := s.hasher.Verify(ctx, hash, v.Password)
	cancel()
	if errors.Is(err, password.ErrBusy) {
		s.busy(w, r)
		return
	}
	if !ok {
		s.limiter.Hit(r.Context(), s.limits.LoginPerAccount, acct)
		s.fail(w, r, 401, "invalid_password", "your password is incorrect")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "deletion failed", err)
		return
	}
	defer tx.Rollback()
	promoted, err := seats.ReleaseUser(r.Context(), tx, own(r).ID)
	if err == nil {
		err = s.notifyPromotions(r.Context(), tx, promoted)
	}
	if err == nil {
		// The photo object outlives the row only until the cleanup job runs.
		var photo sql.NullString
		if err = tx.QueryRowContext(r.Context(), `SELECT avatar_path FROM users WHERE id=$1`, own(r).ID).Scan(&photo); err == nil {
			err = queueDeletion(r.Context(), tx, photo.String)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "DELETE FROM users WHERE id=$1", own(r).ID)
	}
	if err == nil {
		// Linked mail cascades with the user row; also drop any legacy unlinked
		// mail still addressed to this person.
		_, err = tx.ExecContext(r.Context(), "DELETE FROM email_outbox WHERE user_id IS NULL AND lower(to_addr)=$1", own(r).Email)
	}
	if err == nil {
		// The actor row is gone; keep an anonymous record that a deletion happened.
		s.audit(r.Context(), tx, "", "account.delete", "", "self-service")
		err = tx.Commit()
	}
	if err != nil {
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
