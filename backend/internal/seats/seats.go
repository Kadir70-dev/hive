// Package seats owns every change to gathering reservations so capacity and
// strict FIFO waitlist order are enforced in one place.
//
// Locking protocol (always in this order, to avoid deadlocks):
//  1. the member row (FOR SHARE when reserving, FOR UPDATE when releasing),
//  2. the gathering row(s) (FOR UPDATE), several in ascending id order.
//
// Every change to a gathering's reservations happens while its row is locked.
// All functions must run inside the caller's transaction. Functions that can
// free or add seats return the members promoted from the waitlist so callers
// can notify them in the same transaction.
package seats

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Reservation statuses.
const (
	Confirmed = "confirmed"
	Waitlist  = "waitlist"
)

// Gatherings are scheduled in Abu Dhabi local time.
const timeZone = "Asia/Dubai"

var (
	ErrGatheringNotFound = errors.New("gathering not found")
	ErrNoReservation     = errors.New("no reservation")
	ErrNotEligible       = errors.New("member is not approved")
	ErrGatheringStarted  = errors.New("gathering has already started")
)

// CapacityError is returned when a new capacity is below the confirmed seats.
// Confirmed members are never demoted automatically.
type CapacityError struct{ Confirmed int }

func (e *CapacityError) Error() string {
	return fmt.Sprintf("capacity cannot be lower than the %d confirmed places", e.Confirmed)
}

// Promotion records a waitlisted member who received a confirmed place.
type Promotion struct {
	GatheringID string
	UserID      string
}

// Querier is satisfied by *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type gathering struct {
	capacity int
	started  bool
}

func lockGathering(ctx context.Context, q Querier, gatheringID string) (gathering, error) {
	var g gathering
	err := q.QueryRowContext(ctx, `SELECT capacity, ((starts_on + starts_at) AT TIME ZONE '`+timeZone+`') <= now()
FROM gatherings WHERE id=$1 FOR UPDATE`, gatheringID).Scan(&g.capacity, &g.started)
	if errors.Is(err, sql.ErrNoRows) {
		return g, ErrGatheringNotFound
	}
	return g, err
}

// fill promotes the earliest eligible waitlisted members into any free seats.
// The gathering row must already be locked. Past gatherings are left alone.
func fill(ctx context.Context, q Querier, gatheringID string, g gathering) ([]Promotion, error) {
	if g.started {
		return nil, nil
	}
	// RETURNING has no defined order, so re-sort to report promotions in queue order.
	rows, err := q.QueryContext(ctx, `
WITH promoted AS (
  UPDATE reservations SET status='confirmed'
  WHERE id IN (
    SELECT r.id FROM reservations r JOIN users u ON u.id=r.user_id
    WHERE r.gathering_id=$1 AND r.status='waitlist' AND (u.status='approved' OR u.is_admin)
    ORDER BY r.created_at, r.id
    LIMIT GREATEST($2::int - (SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'), 0)
  )
  RETURNING user_id, created_at, id
)
SELECT user_id::text FROM promoted ORDER BY created_at, id`, gatheringID, g.capacity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Promotion
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, Promotion{GatheringID: gatheringID, UserID: uid})
	}
	return out, rows.Err()
}

// Fill locks the gathering and promotes waitlisted members into free seats.
func Fill(ctx context.Context, q Querier, gatheringID string) ([]Promotion, error) {
	g, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return nil, err
	}
	return fill(ctx, q, gatheringID, g)
}

// SetCapacity changes a gathering's capacity under its lock. Raising it
// promotes waitlisted members in FIFO order; lowering it below the number of
// confirmed places is refused with *CapacityError (no automatic demotion).
func SetCapacity(ctx context.Context, q Querier, gatheringID string, capacity int) ([]Promotion, error) {
	g, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return nil, err
	}
	var confirmed int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'`, gatheringID).Scan(&confirmed); err != nil {
		return nil, err
	}
	if capacity < confirmed {
		return nil, &CapacityError{Confirmed: confirmed}
	}
	if _, err := q.ExecContext(ctx, `UPDATE gatherings SET capacity=$2 WHERE id=$1`, gatheringID, capacity); err != nil {
		return nil, err
	}
	g.capacity = capacity
	return fill(ctx, q, gatheringID, g)
}

// Booking is the outcome of Reserve.
type Booking struct {
	Status   string      // Confirmed or Waitlist
	New      bool        // false when the member already had a reservation
	Promoted []Promotion // others promoted while repairing free seats
}

// Reserve keeps a place for userID, or joins the waitlist when full. Existing
// waitlisted members are always served first, so a new request can never
// bypass the queue. If a reservation already exists its status is returned.
func Reserve(ctx context.Context, q Querier, gatheringID, userID string) (Booking, error) {
	var memberStatus string
	var isAdmin bool
	err := q.QueryRowContext(ctx, `SELECT status, is_admin FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&memberStatus, &isAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return Booking{}, ErrNotEligible
	}
	if err != nil {
		return Booking{}, err
	}
	if memberStatus != "approved" && !isAdmin {
		return Booking{}, ErrNotEligible
	}
	g, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return Booking{}, err
	}
	var existing string
	err = q.QueryRowContext(ctx, `SELECT status FROM reservations WHERE gathering_id=$1 AND user_id=$2`, gatheringID, userID).Scan(&existing)
	if err == nil {
		return Booking{Status: existing}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Booking{}, err
	}
	if g.started {
		return Booking{}, ErrGatheringStarted
	}
	// Repair any free seat for people already queued before considering this request.
	promoted, err := fill(ctx, q, gatheringID, g)
	if err != nil {
		return Booking{}, err
	}
	var confirmed, waiting int
	err = q.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status='confirmed'), count(*) FILTER (WHERE status='waitlist') FROM reservations WHERE gathering_id=$1`, gatheringID).Scan(&confirmed, &waiting)
	if err != nil {
		return Booking{}, err
	}
	status := Waitlist
	if confirmed < g.capacity && waiting == 0 {
		status = Confirmed
	}
	// clock_timestamp() records the moment the lock was held, giving true FIFO order.
	_, err = q.ExecContext(ctx, `INSERT INTO reservations(gathering_id,user_id,status,created_at) VALUES($1,$2,$3,clock_timestamp())`, gatheringID, userID, status)
	if err != nil {
		return Booking{}, err
	}
	return Booking{Status: status, New: true, Promoted: promoted}, nil
}

// Cancel removes userID's reservation and promotes the next eligible member.
func Cancel(ctx context.Context, q Querier, gatheringID, userID string) ([]Promotion, error) {
	g, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return nil, err
	}
	res, err := q.ExecContext(ctx, `DELETE FROM reservations WHERE gathering_id=$1 AND user_id=$2`, gatheringID, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNoReservation
	}
	return fill(ctx, q, gatheringID, g)
}

// ReleaseUser removes every reservation held by userID and promotes the next
// eligible members. Call it in the same transaction that deletes or declines
// the member, before that change is committed.
func ReleaseUser(ctx context.Context, q Querier, userID string) ([]Promotion, error) {
	// Blocks concurrent Reserve calls for this member until we commit.
	if _, err := q.ExecContext(ctx, `SELECT 1 FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `
SELECT g.id::text, g.capacity, ((g.starts_on + g.starts_at) AT TIME ZONE '`+timeZone+`') <= now() FROM gatherings g
WHERE g.id IN (SELECT gathering_id FROM reservations WHERE user_id=$1)
ORDER BY g.id FOR UPDATE OF g`, userID)
	if err != nil {
		return nil, err
	}
	type locked struct {
		id string
		g  gathering
	}
	var gs []locked
	for rows.Next() {
		var l locked
		if err := rows.Scan(&l.id, &l.g.capacity, &l.g.started); err != nil {
			rows.Close()
			return nil, err
		}
		gs = append(gs, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM reservations WHERE user_id=$1`, userID); err != nil {
		return nil, err
	}
	var all []Promotion
	for _, l := range gs {
		p, err := fill(ctx, q, l.id, l.g)
		if err != nil {
			return nil, err
		}
		all = append(all, p...)
	}
	return all, nil
}
