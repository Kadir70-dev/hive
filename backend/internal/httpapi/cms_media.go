package httpapi

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"thehivesociety/api/internal/imageproc"
	"thehivesociety/api/internal/ratelimit"
)

// Website images uploaded in the Studio. They are public by design (they
// appear on the website), re-encoded to strip EXIF/GPS metadata, and stored in
// PostgreSQL so they are covered by the existing encrypted backups.

const cmsMediaPath = "/v1/admin/cms/media"

var cmsMediaID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func cmsMediaURL(id string) string { return "/api/backend/cms/media/" + id }

func readUpload(r *http.Request, field string) ([]byte, string, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, "", imageproc.ErrUnsupported
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, "", imageproc.ErrUnsupported
		}
		if part.FormName() != field {
			part.Close()
			continue
		}
		defer part.Close()
		data, err := io.ReadAll(io.LimitReader(part, imageproc.MaxBytes+1))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, "", imageproc.ErrTooLarge
			}
			return nil, "", err
		}
		if len(data) > imageproc.MaxBytes {
			return nil, "", imageproc.ErrTooLarge
		}
		return data, part.FileName(), nil
	}
}

// mediaName keeps a short, printable base name for the library listing.
func mediaName(name string) string {
	name = strings.TrimSuffix(path.Base(strings.ReplaceAll(name, "\\", "/")), path.Ext(name))
	var b strings.Builder
	for _, r := range name {
		if b.Len() >= 60 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' || r == '.' {
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "image.jpg"
	}
	return b.String() + ".jpg"
}

func (s *Server) cmsUploadMedia(w http.ResponseWriter, r *http.Request) {
	if res := s.limiter.Hit(r.Context(), s.limits.PhotosPerMember, ratelimit.Key("cms-media", own(r).ID)); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	data, name, err := readUpload(r, "file")
	if err == nil {
		select {
		case s.imageSlots <- struct{}{}:
		case <-r.Context().Done():
			s.busy(w, r)
			return
		}
		var img imageproc.Result
		img, err = imageproc.ProcessSite(data)
		<-s.imageSlots
		if err == nil {
			var id string
			err = s.db.QueryRowContext(r.Context(), `INSERT INTO cms_media(filename,content_type,data,byte_size,width,height,uploaded_by)
VALUES($1,'image/jpeg',$2,$3,$4,$5,$6) RETURNING id::text`, mediaName(name), img.JPEG, len(img.JPEG), img.Width, img.Height, own(r).ID).Scan(&id)
			if err == nil {
				s.audit(r.Context(), s.db, own(r).ID, "cms.media.upload", "", id)
				reply(w, 201, map[string]any{"id": id, "url": cmsMediaURL(id), "filename": mediaName(name), "alt_text": "", "width": img.Width, "height": img.Height})
				return
			}
		}
	}
	switch {
	case errors.Is(err, imageproc.ErrTooLarge):
		s.fail(w, r, 413, "image_too_large", "images must be 5 MB or smaller")
	case errors.Is(err, imageproc.ErrUnsupported), errors.Is(err, imageproc.ErrCorrupt), errors.Is(err, imageproc.ErrDimensions):
		s.fail(w, r, 400, "invalid_image", "upload a JPEG, PNG or WebP image (5 MB max)")
	default:
		s.internal(w, r, "upload failed", err)
	}
}

func (s *Server) cmsListMedia(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT id::text,filename,alt_text,byte_size,width,height,created_at FROM cms_media ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		s.internal(w, r, "media unavailable", err)
		return
	}
	defer rows.Close()
	type item struct {
		ID        string    `json:"id"`
		URL       string    `json:"url"`
		Filename  string    `json:"filename"`
		Alt       string    `json:"alt_text"`
		Bytes     int       `json:"bytes"`
		Width     int       `json:"width"`
		Height    int       `json:"height"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := []item{}
	for rows.Next() {
		var it item
		if err = rows.Scan(&it.ID, &it.Filename, &it.Alt, &it.Bytes, &it.Width, &it.Height, &it.CreatedAt); err != nil {
			s.internal(w, r, "media unavailable", err)
			return
		}
		it.URL = cmsMediaURL(it.ID)
		out = append(out, it)
	}
	if err = rows.Err(); err != nil {
		s.internal(w, r, "media unavailable", err)
		return
	}
	reply(w, 200, map[string]any{"data": out})
}

func (s *Server) cmsUpdateMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var v struct {
		Alt string `json:"alt_text"`
	}
	if !cmsMediaID.MatchString(id) {
		s.fail(w, r, 404, "not_found", "image not found")
		return
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.Alt = strings.TrimSpace(v.Alt)
	if !cmsCleanText(v.Alt, 300) || strings.Contains(v.Alt, "\n") {
		s.fail(w, r, 422, "invalid_alt", "image descriptions must be one line of at most 300 characters")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `UPDATE cms_media SET alt_text=$1 WHERE id=$2`, v.Alt, id)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "image not found")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "cms.media.alt", "", id)
	reply(w, 200, map[string]any{"ok": true, "alt_text": v.Alt})
}

// cmsServeMedia is public: website images are shown to every visitor.
func (s *Server) cmsServeMedia(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !cmsMediaID.MatchString(id) {
		s.fail(w, r, 404, "not_found", "image not found")
		return
	}
	var data []byte
	err := s.db.QueryRowContext(r.Context(), `SELECT data FROM cms_media WHERE id=$1`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		s.fail(w, r, 404, "not_found", "image not found")
		return
	}
	if err != nil {
		s.internal(w, r, "image unavailable", err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("Content-Length", strconv.Itoa(len(data)))
	// Media IDs are never reused, so the bytes behind a URL never change.
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(200)
	_, _ = w.Write(data)
}
