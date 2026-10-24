package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"time"

	"thehivesociety/api/internal/imageproc"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/storage"
)

// Profile photos live in a private R2 bucket under random keys. They are
// never served from the bucket directly: the API streams a member's own photo
// to them, and any member's photo to MFA-verified admins. Old objects are
// queued in storage_deletions in the same transaction that drops the
// reference, and a background job deletes them (retrying on failure).

const photoPath = "/v1/me/photo"

func (s *Server) photosEnabled(w http.ResponseWriter, r *http.Request) bool {
	if s.store == nil {
		s.fail(w, r, 503, "photos_unavailable", "profile photos are not available yet")
		return false
	}
	return true
}

func newPhotoKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "avatars/" + hex.EncodeToString(b) + ".jpg", nil
}

func queueDeletion(ctx context.Context, q execQuerier, key string) error {
	if key == "" {
		return nil
	}
	_, err := q.ExecContext(ctx, `INSERT INTO storage_deletions(object_key) VALUES($1) ON CONFLICT DO NOTHING`, key)
	return err
}

// readPhoto extracts the "photo" file from a multipart upload, reading at most
// MaxBytes+1 so oversized files are detected without buffering them.
func readPhoto(r *http.Request) ([]byte, error) {
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, imageproc.ErrUnsupported
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			return nil, imageproc.ErrUnsupported
		}
		if part.FormName() != "photo" {
			part.Close()
			continue
		}
		defer part.Close()
		data, err := io.ReadAll(io.LimitReader(part, imageproc.MaxBytes+1))
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				return nil, imageproc.ErrTooLarge
			}
			return nil, err
		}
		if len(data) > imageproc.MaxBytes {
			return nil, imageproc.ErrTooLarge
		}
		return data, nil
	}
}

func (s *Server) uploadPhoto(w http.ResponseWriter, r *http.Request) {
	if !s.photosEnabled(w, r) {
		return
	}
	if res := s.limiter.Hit(r.Context(), s.limits.PhotosPerMember, ratelimit.Key("photo", own(r).ID)); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	data, err := readPhoto(r)
	if err != nil {
		s.photoError(w, r, err)
		return
	}
	// Decoding is memory-heavy: bound concurrent work like password hashing.
	select {
	case s.imageSlots <- struct{}{}:
	case <-r.Context().Done():
		s.busy(w, r)
		return
	}
	img, err := imageproc.Process(data)
	<-s.imageSlots
	if err != nil {
		s.photoError(w, r, err)
		return
	}
	key, err := newPhotoKey()
	if err == nil {
		err = s.store.Put(r.Context(), key, img.JPEG, "image/jpeg")
	}
	if err != nil {
		s.log.Error("photo upload to storage failed", "request_id", requestID(r), "err", err)
		s.fail(w, r, 503, "storage_unavailable", "we couldn't save your photo right now; please try again")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.abandonObject(key)
		s.internal(w, r, "photo update failed", err)
		return
	}
	defer tx.Rollback()
	var old sql.NullString
	err = tx.QueryRowContext(r.Context(), `SELECT avatar_path FROM users WHERE id=$1 FOR UPDATE`, own(r).ID).Scan(&old)
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE users SET avatar_path=$2 WHERE id=$1`, own(r).ID, key)
	}
	if err == nil {
		err = queueDeletion(r.Context(), tx, old.String)
	}
	if err == nil {
		s.audit(r.Context(), tx, own(r).ID, "photo.upload", own(r).ID, "")
		err = tx.Commit()
	}
	if err != nil {
		s.abandonObject(key)
		s.internal(w, r, "photo update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "width": img.Width, "height": img.Height})
}

// abandonObject queues a just-uploaded object whose database update failed.
func (s *Server) abandonObject(key string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := queueDeletion(ctx, s.db, key); err != nil {
		s.log.Error("could not queue orphaned photo for deletion", "err", err)
	}
}

func (s *Server) photoError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, imageproc.ErrTooLarge):
		s.fail(w, r, 413, "photo_too_large", err.Error())
	case errors.Is(err, imageproc.ErrUnsupported), errors.Is(err, imageproc.ErrCorrupt), errors.Is(err, imageproc.ErrDimensions):
		s.fail(w, r, 400, "invalid_photo", err.Error())
	default:
		s.internal(w, r, "photo upload failed", err)
	}
}

func (s *Server) deletePhoto(w http.ResponseWriter, r *http.Request) {
	if !s.photosEnabled(w, r) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "photo removal failed", err)
		return
	}
	defer tx.Rollback()
	var old sql.NullString
	err = tx.QueryRowContext(r.Context(), `SELECT avatar_path FROM users WHERE id=$1 FOR UPDATE`, own(r).ID).Scan(&old)
	if err == nil && !old.Valid {
		s.fail(w, r, 404, "no_photo", "you have no photo")
		return
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `UPDATE users SET avatar_path=NULL WHERE id=$1`, own(r).ID)
	}
	if err == nil {
		err = queueDeletion(r.Context(), tx, old.String)
	}
	if err == nil {
		s.audit(r.Context(), tx, own(r).ID, "photo.delete", own(r).ID, "")
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "photo removal failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}

func (s *Server) streamPhoto(w http.ResponseWriter, r *http.Request, userID string) {
	if !s.photosEnabled(w, r) {
		return
	}
	var key sql.NullString
	err := s.db.QueryRowContext(r.Context(), `SELECT avatar_path FROM users WHERE id=$1`, userID).Scan(&key)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!key.Valid || !storage.SafeKey(key.String))) {
		s.fail(w, r, 404, "no_photo", "no photo")
		return
	}
	if err != nil {
		s.internal(w, r, "photo lookup failed", err)
		return
	}
	body, _, err := s.store.Get(r.Context(), key.String)
	if errors.Is(err, storage.ErrNotFound) {
		s.fail(w, r, 404, "no_photo", "no photo")
		return
	}
	if err != nil {
		s.log.Error("photo download failed", "request_id", requestID(r), "err", err)
		s.fail(w, r, 503, "storage_unavailable", "photo temporarily unavailable")
		return
	}
	defer body.Close()
	h := w.Header()
	h.Set("Content-Type", "image/jpeg") // we only ever store re-encoded JPEGs
	h.Set("Cache-Control", "private, no-store")
	h.Set("Content-Disposition", "inline")
	h.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(200)
	_, _ = io.Copy(w, body)
}

func (s *Server) myPhoto(w http.ResponseWriter, r *http.Request) { s.streamPhoto(w, r, own(r).ID) }

func (s *Server) memberPhoto(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	s.streamPhoto(w, r, id)
}
