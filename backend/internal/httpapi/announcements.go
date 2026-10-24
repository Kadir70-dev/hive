package httpapi

import (
	"net/http"
	"strings"

	"thehivesociety/api/internal/mail"
)

// announcements is the member feed: published only, newest first, paginated.
func (s *Server) announcements(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	p := pageParams(r, 20, 50)
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM announcements WHERE published`).Scan(&total); err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id::text,title,body,COALESCE(published_at,created_at)::text FROM announcements
WHERE published ORDER BY COALESCE(published_at,created_at) DESC, id LIMIT $1 OFFSET $2`, p.Size, p.offset())
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, title, body, at string
		if err := rows.Scan(&id, &title, &body, &at); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "body": body, "published_at": at})
	}
	reply(w, 200, paged(items, p, total))
}

func (s *Server) adminAnnouncements(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r, 25, 100)
	like := likePattern(p.Query)
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM announcements WHERE $1='' OR title ILIKE $2 OR body ILIKE $2`, p.Query, like).Scan(&total); err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT a.id::text,a.title,a.body,a.published,a.created_at::text,a.updated_at::text,COALESCE(a.published_at::text,''),
       (SELECT count(*) FROM announcement_deliveries d WHERE d.announcement_id=a.id)
FROM announcements a WHERE $1='' OR a.title ILIKE $2 OR a.body ILIKE $2
ORDER BY a.created_at DESC, a.id LIMIT $3 OFFSET $4`, p.Query, like, p.Size, p.offset())
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, title, body, created, updated, published string
		var pub bool
		var emailed int
		if err := rows.Scan(&id, &title, &body, &pub, &created, &updated, &published, &emailed); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "title": title, "body": body, "published": pub, "created_at": created,
			"updated_at": updated, "published_at": published, "emailed_count": emailed})
	}
	reply(w, 200, paged(items, p, total))
}

type announcementInput struct {
	Title, Body string
	Published   *bool
}

func (v *announcementInput) normalise() bool {
	v.Title, v.Body = strings.TrimSpace(v.Title), strings.TrimSpace(v.Body)
	return v.Title != "" && v.Body != "" && validText(v.Title, 200, false) && validText(v.Body, 10000, true)
}

func (s *Server) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	var v announcementInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.normalise() {
		s.fail(w, r, 400, "invalid_announcement", "a title (up to 200 characters) and text (up to 10,000) are required")
		return
	}
	published := v.Published == nil || *v.Published
	var id string
	if err := s.db.QueryRowContext(r.Context(), `INSERT INTO announcements(title,body,published,published_at) VALUES($1,$2,$3,CASE WHEN $3 THEN now() END) RETURNING id::text`,
		v.Title, v.Body, published).Scan(&id); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "announcement.create", id, "")
	reply(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) updateAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v announcementInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.normalise() || v.Published == nil {
		s.fail(w, r, 400, "invalid_announcement", "title, text and published are required")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE announcements SET title=$2, body=$3, published=$4, updated_at=now(),
published_at = CASE WHEN $4 AND NOT published THEN now() WHEN $4 THEN COALESCE(published_at, now()) ELSE published_at END WHERE id=$1`,
		id, v.Title, v.Body, *v.Published)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	action := "announcement.update"
	s.audit(r.Context(), s.db, own(r).ID, action, id, map[bool]string{true: "published", false: "unpublished"}[*v.Published])
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) removeAnnouncement(w http.ResponseWriter, r *http.Request) {
	s.deleteByID(w, r, "announcements")
}

// emailAnnouncement queues the announcement for every approved, verified
// member who opted in to email updates. The delivery table's primary key makes
// this idempotent: repeating the request (or retrying after a timeout) never
// emails anyone twice, and members who opted out are never included.
func (s *Server) emailAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "send failed", err)
		return
	}
	defer tx.Rollback()
	var title, body string
	var published bool
	err = tx.QueryRowContext(r.Context(), `SELECT title, body, published FROM announcements WHERE id=$1 FOR UPDATE`, id).Scan(&title, &body, &published)
	if isNoRows(err) {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	if err != nil {
		s.internal(w, r, "send failed", err)
		return
	}
	if !published {
		s.fail(w, r, 409, "not_published", "publish the announcement before emailing it")
		return
	}
	rows, err := tx.QueryContext(r.Context(), `
INSERT INTO announcement_deliveries(announcement_id, user_id)
SELECT $1, u.id FROM users u
WHERE u.email_updates AND u.status='approved' AND u.email_verified_at IS NOT NULL
ON CONFLICT DO NOTHING
RETURNING user_id::text`, id)
	if err != nil {
		s.internal(w, r, "send failed", err)
		return
	}
	var recipients []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			s.internal(w, r, "send failed", err)
			return
		}
		recipients = append(recipients, uid)
	}
	rows.Close()
	for _, uid := range recipients {
		email, name, err := memberContact(r.Context(), tx, uid)
		if err == nil {
			err = mail.Enqueue(r.Context(), tx, uid, mail.TemplateAnnouncement, mail.Announcement(email, name, s.appURL, title, body))
		}
		if err != nil {
			s.internal(w, r, "send failed", err)
			return
		}
	}
	s.audit(r.Context(), tx, own(r).ID, "announcement.email", id, "")
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "send failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "queued": len(recipients)})
}
