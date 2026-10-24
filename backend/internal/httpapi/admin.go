package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"thehivesociety/api/internal/mail"

	"thehivesociety/api/internal/seats"
)

func (s *Server) members(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,email,first_name,status,is_admin,email_verified_at IS NOT NULL FROM users ORDER BY created_at DESC LIMIT 500")
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, email, name, status string
		var admin, verified bool
		if err := rows.Scan(&id, &email, &name, &status, &admin, &verified); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "email": email, "first_name": name, "status": status, "is_admin": admin, "email_verified": verified})
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
	// Lock the member row; admins are managed only with the admin CLI.
	var current, email, name string
	var verified bool
	err = tx.QueryRowContext(r.Context(), "SELECT status, email, first_name, email_verified_at IS NOT NULL FROM users WHERE id=$1 AND is_admin=false FOR UPDATE", id).
		Scan(&current, &email, &name, &verified)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 404, "not_found", "member not found")
		return
	}
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if v.Status == "approved" && !verified {
		s.fail(w, r, 409, "email_unverified", "the member has not confirmed their email address yet")
		return
	}
	if v.Status != "approved" {
		// Leaving "approved" releases the member's places and promotes the waitlist.
		var promoted []seats.Promotion
		promoted, err = seats.ReleaseUser(r.Context(), tx, id)
		if err == nil {
			err = s.notifyPromotions(r.Context(), tx, promoted)
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), "UPDATE users SET status=$1 WHERE id=$2", v.Status, id)
	}
	if err == nil && v.Status == "approved" && current != "approved" {
		err = mail.Enqueue(r.Context(), tx, id, mail.TemplateMemberApproved, mail.MemberApproved(email, name, s.appURL))
	}
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	s.audit(r.Context(), tx, own(r).ID, "member.status", id, v.Status)
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
	s.audit(r.Context(), s.db, own(r).ID, "attendance.checkin", v.UserID, v.GatheringID)
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
	var id string
	if err := s.db.QueryRowContext(r.Context(), "INSERT INTO announcements(title,body) VALUES($1,$2) RETURNING id::text", v.Title, v.Body).Scan(&id); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "announcement.create", id, v.Title)
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
	if v.Name == "" || len(v.Name) > 200 || len(v.Description) > 5000 || !optionalWebURL(v.LogoURL) || !optionalWebURL(v.WebsiteURL) {
		s.fail(w, r, 400, "invalid_partner", "a name is required and links must be full https:// addresses")
		return
	}
	var id string
	if err := s.db.QueryRowContext(r.Context(), "INSERT INTO partners(name,description,logo_url,website_url) VALUES($1,$2,$3,$4) RETURNING id::text", v.Name, v.Description, v.LogoURL, v.WebsiteURL).Scan(&id); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "partner.create", id, v.Name)
	reply(w, 201, map[string]any{"ok": true})
}

func (s *Server) removePartner(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "partners")
}

// optionalWebURL accepts "" or an absolute https URL with a host, so stored
// links can never be javascript:, data: or relative URLs.
func optionalWebURL(raw string) bool {
	if raw == "" {
		return true
	}
	if len(raw) > 2000 || strings.ContainsAny(raw, " \t\r\n") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }

// deleteQueries maps allowed tables to fixed statements (no string building).
var deleteQueries = map[string]string{
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
	s.audit(r.Context(), s.db, own(r).ID, strings.TrimSuffix(table, "s")+".delete", id, "")
	reply(w, 200, map[string]any{"ok": true})
}
