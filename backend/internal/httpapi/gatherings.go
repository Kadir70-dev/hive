package httpapi

import (
	"errors"
	"net/http"
	"time"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/seats"
)

func (s *Server) gatherings(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT g.id::text,g.title,g.type,g.description,g.starts_on::text,g.starts_at::text,g.place,
GREATEST(g.capacity - (SELECT count(*) FROM reservations WHERE gathering_id=g.id AND status='confirmed'),0) AS places_left,
COALESCE(mine.status,''),
CASE WHEN mine.status='waitlist' THEN (SELECT count(*) FROM reservations w WHERE w.gathering_id=g.id AND w.status='waitlist'
  AND (w.created_at, w.id) <= (mine.created_at, mine.id)) ELSE 0 END,
((g.starts_on + g.starts_at) AT TIME ZONE 'Asia/Dubai') <= now()
FROM gatherings g LEFT JOIN reservations mine ON mine.gathering_id=g.id AND mine.user_id=$1
WHERE ((g.starts_on + g.starts_at) AT TIME ZONE 'Asia/Dubai') > now() - interval '1 day' OR mine.id IS NOT NULL
ORDER BY g.starts_on,g.starts_at LIMIT 200`, own(r).ID)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, title, typ, desc, date, t, place, status string
		var left, position int
		var started bool
		if err := rows.Scan(&id, &title, &typ, &desc, &date, &t, &place, &left, &status, &position, &started); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "title": title, "type": typ, "description": desc, "starts_on": date, "starts_at": t, "place": place,
			"places_left": left, "my_status": status, "waitlist_position": position, "started": started})
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
	var input struct {
		WomenOnlyAcknowledged bool `json:"womenOnlyAcknowledged"`
	}
	// Legacy clients with an empty body may still reserve mixed gatherings.
	if r.ContentLength != 0 && !s.parse(w, r, &input) {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "reservation failed", err)
		return
	}
	defer tx.Rollback()
	b, err := seats.ReserveWithAttestation(r.Context(), tx, id, own(r).ID, input.WomenOnlyAcknowledged)
	if err == nil && b.New {
		err = s.notifyBooking(r.Context(), tx, id, own(r).ID, b.Status)
	}
	if err == nil {
		err = s.notifyPromotions(r.Context(), tx, b.Promoted)
	}
	if err == nil {
		err = tx.Commit()
	}
	switch {
	case errors.Is(err, seats.ErrGatheringNotFound):
		s.fail(w, r, 404, "not_found", "gathering not found")
	case errors.Is(err, seats.ErrNotEligible):
		s.fail(w, r, 403, "approval_required", "approval required")
	case errors.Is(err, seats.ErrWomenOnlyConfirmationRequired):
		s.fail(w, r, 403, "women_only_confirmation_required", "confirm you meet this women-only gathering's eligibility before reserving")
	case errors.Is(err, seats.ErrGatheringStarted):
		s.fail(w, r, 409, "gathering_started", "this gathering has already started")
	case errors.Is(err, seats.ErrRestricted):
		s.fail(w, r, 403, "not_available", "this gathering isn't available to you; please contact hello@thehivesociety.ae")
	case err != nil:
		s.internal(w, r, "reservation failed", err)
	default:
		reply(w, 200, map[string]any{"ok": true, "status": b.Status})
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
	promoted, err := seats.Cancel(r.Context(), tx, id, own(r).ID)
	if err == nil {
		err = s.notifyCancellation(r.Context(), tx, id, own(r).ID)
	}
	if err == nil {
		err = s.notifyPromotions(r.Context(), tx, promoted)
	}
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

type gatheringInput struct {
	Title, Type, Description, StartsOn, StartsAt, Place string
	Capacity                                            int
}

func (v *gatheringInput) valid() bool {
	_, dateErr := time.Parse("2006-01-02", v.StartsOn)
	_, timeErr := time.Parse("15:04", v.StartsAt)
	if timeErr != nil {
		_, timeErr = time.Parse("15:04:05", v.StartsAt)
	}
	if v.Type == "" {
		v.Type = "community"
	}
	return v.Title != "" && len(v.Title) <= 200 && len(v.Type) <= 60 && len(v.Description) <= 5000 && v.Place != "" && len(v.Place) <= 300 &&
		v.Capacity >= 1 && v.Capacity <= 10000 && dateErr == nil && timeErr == nil
}

func (s *Server) createGathering(w http.ResponseWriter, r *http.Request) {
	var v gatheringInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.valid() {
		s.fail(w, r, 400, "invalid_gathering", "invalid gathering")
		return
	}
	var id string
	err := s.db.QueryRowContext(r.Context(), "INSERT INTO gatherings(title,type,description,starts_on,starts_at,place,capacity) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id::text",
		v.Title, v.Type, v.Description, v.StartsOn, v.StartsAt, v.Place, v.Capacity).Scan(&id)
	if err != nil {
		s.internal(w, r, "insert failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "gathering.create", id, v.Title)
	reply(w, 201, map[string]any{"ok": true, "id": id})
}

// updateGathering edits a gathering. Capacity changes go through
// seats.SetCapacity (FIFO promotion, no overbooking, no demotion); members
// with a reservation are emailed when the date, time or place changes.
func (s *Server) updateGathering(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v gatheringInput
	if !s.parse(w, r, &v) {
		return
	}
	if !v.valid() {
		s.fail(w, r, 400, "invalid_gathering", "invalid gathering")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	defer tx.Rollback()
	// SetCapacity takes the gathering lock first, so everything below is serialised
	// with bookings and cancellations.
	promoted, err := seats.SetCapacity(r.Context(), tx, id, v.Capacity)
	var capErr *seats.CapacityError
	switch {
	case errors.Is(err, seats.ErrGatheringNotFound):
		s.fail(w, r, 404, "not_found", "gathering not found")
		return
	case errors.As(err, &capErr):
		s.fail(w, r, 409, "capacity_below_confirmed", capErr.Error())
		return
	case err != nil:
		s.internal(w, r, "update failed", err)
		return
	}
	// A category switch could retroactively make existing bookings ineligible.
	// Require the organiser to resolve existing seats before changing category.
	var existingType string
	var existingBookings int
	if err = tx.QueryRowContext(r.Context(), `SELECT type, (SELECT count(*) FROM reservations WHERE gathering_id=$1) FROM gatherings WHERE id=$1`, id).Scan(&existingType, &existingBookings); err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if existingType != v.Type && existingBookings > 0 {
		s.fail(w, r, 409, "category_change_with_bookings", "cancel existing reservations before changing the gathering category")
		return
	}
	var scheduleChanged bool
	err = tx.QueryRowContext(r.Context(), `
WITH old AS (SELECT starts_on, starts_at, place FROM gatherings WHERE id=$1)
UPDATE gatherings g SET title=$2,type=$3,description=$4,starts_on=$5,starts_at=$6,place=$7
FROM old WHERE g.id=$1
RETURNING (old.starts_on, old.starts_at, old.place) IS DISTINCT FROM (g.starts_on, g.starts_at, g.place)`,
		id, v.Title, v.Type, v.Description, v.StartsOn, v.StartsAt, v.Place).Scan(&scheduleChanged)
	if err == nil && scheduleChanged {
		var g mail.Gathering
		if g, err = gatheringInfo(r.Context(), tx, id); err == nil {
			err = notifyReservationHolders(r.Context(), tx, id, mail.TemplateGatheringUpdate, func(email, name string) mail.Message {
				return mail.GatheringUpdated(email, name, s.appURL, g)
			})
		}
	}
	if err == nil {
		err = s.notifyPromotions(r.Context(), tx, promoted)
	}
	if err == nil {
		s.audit(r.Context(), tx, own(r).ID, "gathering.update", id, v.Title)
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "promoted": len(promoted)})
}

// deleteGathering cancels a gathering and tells everyone who had reserved.
func (s *Server) deleteGathering(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "delete failed", err)
		return
	}
	defer tx.Rollback()
	var started bool
	err = tx.QueryRowContext(r.Context(), `SELECT ((starts_on + starts_at) AT TIME ZONE 'Asia/Dubai') <= now() FROM gatherings WHERE id=$1 FOR UPDATE`, id).Scan(&started)
	if err != nil {
		if isNoRows(err) {
			s.fail(w, r, 404, "not_found", "not found")
			return
		}
		s.internal(w, r, "delete failed", err)
		return
	}
	g, err := gatheringInfo(r.Context(), tx, id)
	if err == nil && !started { // no "cancelled" emails for gatherings that already happened
		err = notifyReservationHolders(r.Context(), tx, id, mail.TemplateGatheringCancel, func(email, name string) mail.Message {
			return mail.GatheringCancelled(email, name, g)
		})
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `DELETE FROM gatherings WHERE id=$1`, id)
	}
	if err == nil {
		s.audit(r.Context(), tx, own(r).ID, "gathering.delete", id, g.Title)
		err = tx.Commit()
	}
	if err != nil {
		s.internal(w, r, "delete failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
