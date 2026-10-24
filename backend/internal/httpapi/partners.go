package httpapi

import (
	"net/http"
	"net/url"
	"strings"
)

func (s *Server) partners(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), "SELECT id::text,name,description,logo_url,website_url FROM partners WHERE published ORDER BY name LIMIT 200")
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

func (s *Server) adminPartners(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r, 25, 100)
	like := likePattern(p.Query)
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM partners WHERE $1='' OR name ILIKE $2 OR description ILIKE $2`, p.Query, like).Scan(&total); err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT id::text,name,description,logo_url,website_url,published,updated_at::text FROM partners
WHERE $1='' OR name ILIKE $2 OR description ILIKE $2 ORDER BY name, id LIMIT $3 OFFSET $4`, p.Query, like, p.Size, p.offset())
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, name, desc, logo, site, updated string
		var pub bool
		if err := rows.Scan(&id, &name, &desc, &logo, &site, &pub, &updated); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "name": name, "description": desc, "logo_url": logo, "website_url": site, "published": pub, "updated_at": updated})
	}
	reply(w, 200, paged(items, p, total))
}

type partnerInput struct {
	Name, Description, LogoURL, WebsiteURL string
	Published                              *bool
}

func (v *partnerInput) normalise() bool {
	v.Name, v.Description = strings.TrimSpace(v.Name), strings.TrimSpace(v.Description)
	v.LogoURL, v.WebsiteURL = strings.TrimSpace(v.LogoURL), strings.TrimSpace(v.WebsiteURL)
	return v.Name != "" && validText(v.Name, 200, false) && validText(v.Description, 5000, true) &&
		optionalWebURL(v.LogoURL) && optionalWebURL(v.WebsiteURL)
}

const partnerInvalid = "a name is required and links must be full https:// addresses"

func (s *Server) createPartner(w http.ResponseWriter, r *http.Request) {
	var v partnerInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.normalise() {
		s.fail(w, r, 400, "invalid_partner", partnerInvalid)
		return
	}
	published := v.Published == nil || *v.Published
	var id string
	if err := s.db.QueryRowContext(r.Context(), "INSERT INTO partners(name,description,logo_url,website_url,published) VALUES($1,$2,$3,$4,$5) RETURNING id::text",
		v.Name, v.Description, v.LogoURL, v.WebsiteURL, published).Scan(&id); err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "partner.create", id, "")
	reply(w, 201, map[string]any{"ok": true, "id": id})
}

func (s *Server) updatePartner(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v partnerInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.normalise() || v.Published == nil {
		s.fail(w, r, 400, "invalid_partner", partnerInvalid)
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE partners SET name=$2,description=$3,logo_url=$4,website_url=$5,published=$6,updated_at=now() WHERE id=$1`,
		id, v.Name, v.Description, v.LogoURL, v.WebsiteURL, *v.Published)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "partner.update", id, "")
	reply(w, 200, map[string]any{"ok": true})
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
