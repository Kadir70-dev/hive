package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

// Website Studio. Each page has one draft and one published document. Saving
// a draft never changes the public site; publishing copies the draft (at the
// revision the editor last saw) to published and records a version, which can
// later be restored. Documents only override the page's built-in copy, so an
// empty document means "show the website as shipped".

type cmsSEO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	OGImage     string `json:"og_image"`
	Canonical   string `json:"canonical"`
	Robots      string `json:"robots"`
}

// cmsDoc values are either text or a list of [heading, text] pairs (FAQ etc.).
type cmsDoc struct {
	EN  map[string]json.RawMessage `json:"en"`
	AR  map[string]json.RawMessage `json:"ar"`
	SEO cmsSEO                     `json:"seo"`
}

var (
	cmsPages  = map[string]bool{"home": true}
	cmsKey    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,47}$`)
	cmsEmTags = strings.NewReplacer("<em>", "", "</em>", "")
	// Image slots are named like visionImg1; their values are image URLs.
	cmsImageKey = regexp.MustCompile(`Img[0-9]?$`)
)

const (
	cmsMaxKeys  = 250
	cmsMaxText  = 4000
	cmsMaxPairs = 20
	cmsMaxPair  = 1500
)

func emptyDoc() cmsDoc {
	return cmsDoc{EN: map[string]json.RawMessage{}, AR: map[string]json.RawMessage{}}
}

func (d *cmsDoc) normalise() {
	if d.EN == nil {
		d.EN = map[string]json.RawMessage{}
	}
	if d.AR == nil {
		d.AR = map[string]json.RawMessage{}
	}
}

// cmsSafeURL accepts a site path or an https URL that can sit inside CSS url("…").
func cmsSafeURL(v string) bool {
	if v == "" {
		return true
	}
	if len(v) > 2048 || strings.ContainsAny(v, "\"'()\\<> \t\r\n") {
		return false
	}
	if strings.HasPrefix(v, "/") {
		return !strings.HasPrefix(v, "//")
	}
	u, err := url.Parse(v)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil
}

func cmsCleanText(v string, max int) bool {
	if !utf8.ValidString(v) || utf8.RuneCountInString(v) > max {
		return false
	}
	for _, r := range v {
		if r < 0x20 && r != '\n' {
			return false
		}
	}
	return true
}

func validCmsValue(key string, raw json.RawMessage) bool {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if cmsImageKey.MatchString(key) {
			return cmsSafeURL(text)
		}
		// Headings may emphasise words with <em>; no other markup is allowed.
		return cmsCleanText(text, cmsMaxText) && !strings.Contains(cmsEmTags.Replace(text), "<")
	}
	var pairs [][]string
	if json.Unmarshal(raw, &pairs) != nil || len(pairs) > cmsMaxPairs {
		return false
	}
	for _, p := range pairs {
		if len(p) != 2 || !cmsCleanText(p[0], cmsMaxPair) || !cmsCleanText(p[1], cmsMaxPair) {
			return false
		}
	}
	return true
}

func validCmsDoc(d cmsDoc) bool {
	if len(d.EN) > cmsMaxKeys || len(d.AR) > cmsMaxKeys {
		return false
	}
	for _, lang := range []map[string]json.RawMessage{d.EN, d.AR} {
		for k, v := range lang {
			if !cmsKey.MatchString(k) || k == "langBtn" || !validCmsValue(k, v) {
				return false
			}
		}
	}
	s := d.SEO
	return cmsCleanText(s.Title, 140) && !strings.Contains(s.Title, "\n") &&
		cmsCleanText(s.Description, 400) && cmsSafeURL(s.OGImage) &&
		(s.Canonical == "" || (strings.HasPrefix(s.Canonical, "/") && cmsSafeURL(s.Canonical))) &&
		(s.Robots == "" || s.Robots == "index" || s.Robots == "noindex")
}

func (s *Server) cmsSlug(w http.ResponseWriter, r *http.Request) (string, bool) {
	slug := chi.URLParam(r, "slug")
	if !cmsPages[slug] {
		s.fail(w, r, 404, "not_found", "page not found")
		return "", false
	}
	return slug, true
}

func decodeDoc(b []byte) (cmsDoc, error) {
	d := emptyDoc()
	if len(b) > 0 {
		if err := json.Unmarshal(b, &d); err != nil {
			return d, err
		}
	}
	d.normalise()
	return d, nil
}

// cmsPublic serves the published overrides for the public website.
func (s *Server) cmsPublic(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	var body []byte
	var rev int64
	err := s.db.QueryRowContext(r.Context(), `SELECT published,published_revision FROM cms_pages WHERE slug=$1`, slug).Scan(&body, &rev)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.internal(w, r, "content unavailable", err)
		return
	}
	doc, err := decodeDoc(body)
	if err != nil {
		s.internal(w, r, "content unavailable", err)
		return
	}
	reply(w, 200, map[string]any{"slug": slug, "document": doc, "published_revision": rev})
}

func (s *Server) cmsAdminPage(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	var draft, published []byte
	var dRev, pRev int64
	var publishedAt sql.NullTime
	err := s.db.QueryRowContext(r.Context(), `SELECT draft,published,draft_revision,published_revision,published_at FROM cms_pages WHERE slug=$1`, slug).
		Scan(&draft, &published, &dRev, &pRev, &publishedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		s.internal(w, r, "content unavailable", err)
		return
	}
	d, err1 := decodeDoc(draft)
	p, err2 := decodeDoc(published)
	if err = errors.Join(err1, err2); err != nil {
		s.internal(w, r, "content unavailable", err)
		return
	}
	var at *time.Time
	if publishedAt.Valid {
		at = &publishedAt.Time
	}
	reply(w, 200, map[string]any{"slug": slug, "draft": d, "published": p, "draft_revision": dRev, "published_revision": pRev, "published_at": at})
}

func (s *Server) cmsSaveDraft(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	var v struct {
		Document         cmsDoc `json:"document"`
		ExpectedRevision int64  `json:"expected_revision"`
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.Document.normalise()
	if v.ExpectedRevision < 0 || !validCmsDoc(v.Document) {
		s.fail(w, r, 422, "invalid_content", "some content is not allowed (check links, image addresses and length)")
		return
	}
	encoded, err := json.Marshal(v.Document)
	if err != nil {
		s.internal(w, r, "save failed", err)
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "save failed", err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO cms_pages(slug) VALUES($1) ON CONFLICT DO NOTHING`, slug); err != nil {
		s.internal(w, r, "save failed", err)
		return
	}
	var rev int64
	err = tx.QueryRowContext(r.Context(), `UPDATE cms_pages SET draft=$1,draft_revision=draft_revision+1,updated_by=$2,updated_at=now()
WHERE slug=$3 AND draft_revision=$4 RETURNING draft_revision`, encoded, own(r).ID, slug, v.ExpectedRevision).Scan(&rev)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 409, "draft_changed", "this draft was changed elsewhere; reload before saving")
		return
	}
	if err != nil {
		s.internal(w, r, "save failed", err)
		return
	}
	s.audit(r.Context(), tx, own(r).ID, "cms.draft", "", slug)
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "save failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "draft_revision": rev})
}

// cmsPublish makes the draft live, but only the draft revision the editor saw.
func (s *Server) cmsPublish(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	var v struct {
		ExpectedRevision int64 `json:"expected_revision"`
	}
	if !s.parse(w, r, &v) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "publish failed", err)
		return
	}
	defer tx.Rollback()
	var snapshot []byte
	var version int64
	err = tx.QueryRowContext(r.Context(), `UPDATE cms_pages SET published=draft,published_revision=published_revision+1,published_at=now(),updated_by=$2,updated_at=now()
WHERE slug=$1 AND draft_revision=$3 AND draft_revision>0 RETURNING published,published_revision`, slug, own(r).ID, v.ExpectedRevision).Scan(&snapshot, &version)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 409, "draft_changed", "save the latest draft before publishing")
		return
	}
	if err == nil {
		err = s.recordVersion(r, tx, slug, version, snapshot)
	}
	if err != nil {
		s.internal(w, r, "publish failed", err)
		return
	}
	s.audit(r.Context(), tx, own(r).ID, "cms.publish", "", slug)
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "publish failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "published_revision": version})
}

func (s *Server) recordVersion(r *http.Request, tx *sql.Tx, slug string, version int64, snapshot []byte) error {
	_, err := tx.ExecContext(r.Context(), `INSERT INTO cms_versions(page_slug,version,snapshot,published_by) VALUES($1,$2,$3,$4)`, slug, version, snapshot, own(r).ID)
	return err
}

func (s *Server) cmsVersions(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT v.version,v.created_at,COALESCE(u.first_name,'') FROM cms_versions v
LEFT JOIN users u ON u.id=v.published_by WHERE v.page_slug=$1 ORDER BY v.version DESC LIMIT 30`, slug)
	if err != nil {
		s.internal(w, r, "history unavailable", err)
		return
	}
	defer rows.Close()
	type item struct {
		Version   int64     `json:"version"`
		CreatedAt time.Time `json:"created_at"`
		By        string    `json:"published_by"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		if err = rows.Scan(&it.Version, &it.CreatedAt, &it.By); err != nil {
			s.internal(w, r, "history unavailable", err)
			return
		}
		out = append(out, it)
	}
	if err = rows.Err(); err != nil {
		s.internal(w, r, "history unavailable", err)
		return
	}
	reply(w, 200, map[string]any{"data": out})
}

// cmsRestore republishes an earlier version (and makes it the draft too).
func (s *Server) cmsRestore(w http.ResponseWriter, r *http.Request) {
	slug, ok := s.cmsSlug(w, r)
	if !ok {
		return
	}
	var v struct {
		Version int64 `json:"version"`
	}
	if !s.parse(w, r, &v) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "restore failed", err)
		return
	}
	defer tx.Rollback()
	var snapshot []byte
	err = tx.QueryRowContext(r.Context(), `SELECT snapshot FROM cms_versions WHERE page_slug=$1 AND version=$2`, slug, v.Version).Scan(&snapshot)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 404, "not_found", "version not found")
		return
	}
	var version int64
	if err == nil {
		err = tx.QueryRowContext(r.Context(), `UPDATE cms_pages SET draft=$2,published=$2,draft_revision=draft_revision+1,published_revision=published_revision+1,
published_at=now(),updated_by=$3,updated_at=now() WHERE slug=$1 RETURNING published_revision`, slug, snapshot, own(r).ID).Scan(&version)
	}
	if err == nil {
		err = s.recordVersion(r, tx, slug, version, snapshot)
	}
	if err != nil {
		s.internal(w, r, "restore failed", err)
		return
	}
	s.audit(r.Context(), tx, own(r).ID, "cms.restore", "", slug)
	if err = tx.Commit(); err != nil {
		s.internal(w, r, "restore failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "published_revision": version})
}
