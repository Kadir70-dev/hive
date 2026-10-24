// Package seats owns every change to gathering reservations so capacity and
// strict FIFO waitlist order are enforced in one place.
//
// Locking protocol (always in this order, to avoid deadlocks):
//  1. the member row (FOR SHARE when reserving, FOR UPDATE when releasing),
//  2. the gathering row(s) (FOR UPDATE), several in ascending id order.
//
// Every change to a gathering's reservations happens while its row is locked.
// All functions must run inside the caller's transaction.
package seats

import (
	"context"
	"database/sql"
	"errors"
)

// Reservation statuses.
const (
	Confirmed = "confirmed"
	Waitlist  = "waitlist"
)

var (
	ErrGatheringNotFound = errors.New("gathering not found")
	ErrNoReservation     = errors.New("no reservation")
	ErrNotEligible       = errors.New("member is not approved")
)

// Querier is satisfied by *sql.Tx.
type Querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func lockGathering(ctx context.Context, q Querier, gatheringID string) (int, error) {
	var capacity int
	err := q.QueryRowContext(ctx, `SELECT capacity FROM gatherings WHERE id=$1 FOR UPDATE`, gatheringID).Scan(&capacity)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrGatheringNotFound
	}
	return capacity, err
}

// fill promotes the earliest eligible waitlisted members into any free seats.
// The gathering row must already be locked. Returns the number promoted.
func fill(ctx context.Context, q Querier, gatheringID string, capacity int) (int, error) {
	res, err := q.ExecContext(ctx, `
UPDATE reservations SET status='confirmed'
WHERE id IN (
  SELECT r.id FROM reservations r JOIN users u ON u.id=r.user_id
  WHERE r.gathering_id=$1 AND r.status='waitlist' AND (u.status='approved' OR u.is_admin)
  ORDER BY r.created_at, r.id
  LIMIT GREATEST($2::int - (SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'), 0)
)`, gatheringID, capacity)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// Fill locks the gathering and promotes waitlisted members into free seats.
// Use after any capacity increase (e.g. the future gathering edit endpoint).
func Fill(ctx context.Context, q Querier, gatheringID string) (int, error) {
	capacity, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return 0, err
	}
	return fill(ctx, q, gatheringID, capacity)
}

// Reserve keeps a place for userID, or joins the waitlist when full. Existing
// waitlisted members are always served first, so a new request can never
// bypass the queue. If a reservation already exists its status is returned.
func Reserve(ctx context.Context, q Querier, gatheringID, userID string) (string, error) {
	var memberStatus string
	var isAdmin bool
	err := q.QueryRowContext(ctx, `SELECT status, is_admin FROM users WHERE id=$1 FOR SHARE`, userID).Scan(&memberStatus, &isAdmin)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotEligible
	}
	if err != nil {
		return "", err
	}
	if memberStatus != "approved" && !isAdmin {
		return "", ErrNotEligible
	}
	capacity, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return "", err
	}
	var existing string
	err = q.QueryRowContext(ctx, `SELECT status FROM reservations WHERE gathering_id=$1 AND user_id=$2`, gatheringID, userID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	// Repair any free seat for people already queued before considering this request.
	if _, err = fill(ctx, q, gatheringID, capacity); err != nil {
		return "", err
	}
	var confirmed, waiting int
	err = q.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE status='confirmed'), count(*) FILTER (WHERE status='waitlist') FROM reservations WHERE gathering_id=$1`, gatheringID).Scan(&confirmed, &waiting)
	if err != nil {
		return "", err
	}
	status := Waitlist
	if confirmed < capacity && waiting == 0 {
		status = Confirmed
	}
	// clock_timestamp() records the moment the lock was held, giving true FIFO order.
	_, err = q.ExecContext(ctx, `INSERT INTO reservations(gathering_id,user_id,status,created_at) VALUES($1,$2,$3,clock_timestamp())`, gatheringID, userID, status)
	if err != nil {
		return "", err
	}
	return status, nil
}

// Cancel removes userID's reservation and promotes the next eligible member.
func Cancel(ctx context.Context, q Querier, gatheringID, userID string) error {
	capacity, err := lockGathering(ctx, q, gatheringID)
	if err != nil {
		return err
	}
	res, err := q.ExecContext(ctx, `DELETE FROM reservations WHERE gathering_id=$1 AND user_id=$2`, gatheringID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNoReservation
	}
	_, err = fill(ctx, q, gatheringID, capacity)
	return err
}

// ReleaseUser removes every reservation held by userID and promotes the next
// eligible members. Call it in the same transaction that deletes or declines
// the member, before that change is committed.
func ReleaseUser(ctx context.Context, q Querier, userID string) error {
	// Blocks concurrent Reserve calls for this member until we commit.
	if _, err := q.ExecContext(ctx, `SELECT 1 FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		return err
	}
	rows, err := q.QueryContext(ctx, `
SELECT g.id::text, g.capacity FROM gatherings g
WHERE g.id IN (SELECT gathering_id FROM reservations WHERE user_id=$1)
ORDER BY g.id FOR UPDATE OF g`, userID)
	if err != nil {
		return err
	}
	type locked struct {
		id       string
		capacity int
	}
	var gs []locked
	for rows.Next() {
		var g locked
		if err := rows.Scan(&g.id, &g.capacity); err != nil {
			rows.Close()
			return err
		}
		gs = append(gs, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if _, err := q.ExecContext(ctx, `DELETE FROM reservations WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, g := range gs {
		if _, err := fill(ctx, q, g.id, g.capacity); err != nil {
			return err
		}
	}
	return nil
}
