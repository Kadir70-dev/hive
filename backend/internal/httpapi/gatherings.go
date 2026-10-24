package httpapi

import (
	"errors"
	"net/http"

	"thehivesociety/api/internal/seats"
)

func (s *Server) gatherings(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT g.id::text,g.title,g.type,g.description,g.starts_on::text,g.starts_at::text,g.place,
GREATEST(g.capacity - (SELECT count(*) FROM reservations WHERE gathering_id=g.id AND status='confirmed'),0) AS places_left,
COALESCE((SELECT status FROM reservations WHERE gathering_id=g.id AND user_id=$1),'')
FROM gatherings g ORDER BY g.starts_on,g.starts_at LIMIT 200`, own(r).ID)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place, status string
		var left int
		if err := rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &left, &status); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place, "places_left": left, "my_status": status})
	}
	reply(w, 200, out)
}

func (s *Server) keep(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "reservation failed", err)
		return
	}
	defer tx.Rollback()
	status, err := seats.Reserve(r.Context(), tx, id, own(r).ID)
	if err == nil {
		err = tx.Commit()
	}
	switch {
	case errors.Is(err, seats.ErrGatheringNotFound):
		s.fail(w, r, 404, "not_found", "gathering not found")
	case errors.Is(err, seats.ErrNotEligible):
		s.fail(w, r, 403, "approval_required", "approval required")
	case err != nil:
		s.internal(w, r, "reservation failed", err)
	default:
		reply(w, 200, map[string]any{"ok": true, "status": status})
	}
}

func (s *Server) cancel(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "cancellation failed", err)
		return
	}
	defer tx.Rollback()
	err = seats.Cancel(r.Context(), tx, id, own(r).ID)
	if err == nil {
		err = tx.Commit()
	}
	switch {
	case errors.Is(err, seats.ErrGatheringNotFound):
		s.fail(w, r, 404, "not_found", "gathering not found")
	case errors.Is(err, seats.ErrNoReservation):
		s.fail(w, r, 404, "no_reservation", "you have no place at this gathering")
	case err != nil:
		s.internal(w, r, "cancellation failed", err)
	default:
		reply(w, 200, map[string]any{"ok": true})
	}
}
