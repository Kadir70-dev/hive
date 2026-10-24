package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"thehivesociety/api/internal/seats"
)

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,email,first_name,status,is_admin FROM users ORDER BY created_at DESC LIMIT 500")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, email, name, status string
		var admin bool
		if err := rows.Scan(&id, &email, &name, &status, &admin); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "email": email, "first_name": name, "status": status, "is_admin": admin})
	}
	reply(w, 200, out)
}

// memberStatus changes approval. Moving a member out of "approved" releases
// their seats and promotes the waitlist in the same transaction.
func (s *Server) memberStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v struct{ Status string }
	if !s.parse(w, r, &v) {
		return
	}
	if v.Status != "pending" && v.Status != "approved" && v.Status != "declined" {
		s.fail(w, r, 400, "invalid_status", "invalid status")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	defer tx.Rollback()
	if v.Status != "approved" {
		var isAdmin bool
		err = tx.QueryRowContext(r.Context(), "SELECT is_admin FROM users WHERE id=$1", id).Scan(&isAdmin)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && isAdmin) {
			s.fail(w, r, 404, "not_found", "member not found")
			return
		}
		if err == nil {
			err = seats.ReleaseUser(r.Context(), tx, id)
		}
		if err != nil {
			s.internal(w, r, "update failed", err)
			return
		}
	}
	res, err := tx.ExecContext(r.Context(), "UPDATE users SET status=$1 WHERE id=$2 AND is_admin=false", v.Status, id)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "member not found")
		return
	}
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) reports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,reporter_id::text,COALESCE(reported_member_id::text,''),reason,status FROM member_reports ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, reporter, target, reason, status string
		if err := rows.Scan(&id, &reporter, &target, &reason, &status); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "reporter_id": reporter, "reported_member_id": target, "reason": reason, "status": status})
	}
	reply(w, 200, out)
}

func (s *Server) adminGatherings(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,title,type,description,starts_on::text,starts_at::text,place,capacity FROM gatherings ORDER BY starts_on")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place string
		var capacity int
		if err := rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &capacity); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place, "capacity": capacity})
	}
	reply(w, 200, out)
}

func (s *Server) createGathering(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Title, Type, Description, StartsOn, StartsAt, Place string
		Capacity                                            int
	}
	if !s.parse(w, r, &v) {
		return
	}
	_, dateErr := time.Parse("2006-01-02", v.StartsOn)
	_, timeErr := time.Parse("15:04", v.StartsAt)
	if timeErr != nil {
		_, timeErr = time.Parse("15:04:05", v.StartsAt)
	}
	if v.Title == "" || len(v.Title) > 200 || len(v.Type) > 60 || len(v.Description) > 5000 || v.Place == "" || len(v.Place) > 300 ||
		v.Capacity < 1 || v.Capacity > 10000 || dateErr != nil || timeErr != nil {
		s.fail(w, r, 400, "invalid_gathering", "invalid gathering")
		return
	}
	if v.Type == "" {
		v.Type = "community"
	}
	_, err := s.db.ExecContext(r.Context(), "INSERT INTO gatherings(title,type,description,starts_on,starts_at,place,capacity) VALUES($1,$2,$3,$4,$5,$6,$7)", v.Title, v.Type, v.Description, v.StartsOn, v.StartsAt, v.Place, v.Capacity)
	if err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}

func (s *Server) deleteGathering(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "gatherings")
}

func (s *Server) attendees(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT u.id::text,u.first_name,u.email,r.status,COALESCE(a.attended,false) FROM reservations r JOIN users u ON u.id=r.user_id LEFT JOIN attendance a ON a.gathering_id=r.gathering_id AND a.user_id=u.id WHERE r.gathering_id=$1 ORDER BY r.created_at, r.id`, id)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var uid, name, email, status string
		var attended bool
		if err := rows.Scan(&uid, &name, &email, &status, &attended); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"user_id": uid, "first_name": name, "email": email, "status": status, "attended": attended})
	}
	reply(w, 200, out)
}

// checkin records attendance only for confirmed reservations and reports
// failure when nothing was written.
func (s *Server) checkin(w http.ResponseWriter, r *http.Request) {
	var v struct {
		GatheringID, UserID string
		Attended            bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	if !validUUID(v.GatheringID) || !validUUID(v.UserID) {
		s.fail(w, r, 400, "invalid_checkin", "invalid checkin")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `INSERT INTO attendance(gathering_id,user_id,attended,checked_at) SELECT $1,$2,$3,now() WHERE EXISTS(SELECT 1 FROM reservations WHERE gathering_id=$1 AND user_id=$2 AND status='confirmed') ON CONFLICT(gathering_id,user_id) DO UPDATE SET attended=excluded.attended,checked_at=now()`, v.GatheringID, v.UserID, v.Attended)
	if err != nil {
		s.internal(w, r, "checkin failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 409, "not_confirmed", "only members with a confirmed place can be checked in")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	var v struct{ Title, Body string }
	if !s.parse(w, r, &v) {
		return
	}
	if v.Title == "" || v.Body == "" || len(v.Title) > 200 || len(v.Body) > 10000 {
		s.fail(w, r, 400, "invalid_announcement", "title and body required")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "INSERT INTO announcements(title,body) VALUES($1,$2)", v.Title, v.Body); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}

func (s *Server) removeAnnouncement(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "announcements")
}

func (s *Server) createPartner(w http.ResponseWriter, r *http.Request) {
	var v struct{ Name, Description, LogoURL, WebsiteURL string }
	if !s.parse(w, r, &v) {
		return
	}
	if v.Name == "" || len(v.Name) > 200 || len(v.Description) > 5000 || len(v.LogoURL) > 2000 || len(v.WebsiteURL) > 2000 {
		s.fail(w, r, 400, "invalid_partner", "name required")
		return
	}
	if _, err := s.db.ExecContext(r.Context(), "INSERT INTO partners(name,description,logo_url,website_url) VALUES($1,$2,$3,$4)", v.Name, v.Description, v.LogoURL, v.WebsiteURL); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	reply(w, 201, map[string]any{"ok": true})
}

func (s *Server) removePartner(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "partners")
}

// deleteQueries maps allowed tables to fixed statements (no string building).
var deleteQueries = map[string]string{
	"gatherings":    "DELETE FROM gatherings WHERE id=$1",
	"announcements": "DELETE FROM announcements WHERE id=$1",
	"partners":      "DELETE FROM partners WHERE id=$1",
}

func (s *Server) deleteByID(w http.ResponseWriter, r *http.Request, table string) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), deleteQueries[table], id)
	if err != nil {
		s.internal(w, r, "delete failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
