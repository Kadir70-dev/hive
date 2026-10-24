package seats_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"thehivesociety/api/internal/seats"
	"thehivesociety/api/internal/testdb"
)

func inTx(t *testing.T, db *sql.DB, fn func(*sql.Tx) error) error {
	t.Helper()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func reserve(t *testing.T, db *sql.DB, g, u string) string {
	t.Helper()
	var b seats.Booking
	if err := inTx(t, db, func(tx *sql.Tx) (err error) { b, err = seats.Reserve(context.Background(), tx, g, u); return }); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	return b.Status
}

func statuses(t *testing.T, db *sql.DB, g string) map[string]string {
	t.Helper()
	rows, err := db.Query(`SELECT user_id::text,status FROM reservations WHERE gathering_id=$1`, g)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var u, s string
		if err := rows.Scan(&u, &s); err != nil {
			t.Fatal(err)
		}
		out[u] = s
	}
	return out
}

func confirmedCount(t *testing.T, db *sql.DB, g string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM reservations WHERE gathering_id=$1 AND status='confirmed'`, g).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// setup: capacity 1, one confirmed member, three waitlisted members in order.
func setup(t *testing.T) (db *sql.DB, g, confirmed string, waiting []string) {
	db = testdb.Open(t)
	g = testdb.Gathering(t, db, 1)
	confirmed = testdb.User(t, db, "c@example.test", "approved", false)
	if st := reserve(t, db, g, confirmed); st != seats.Confirmed {
		t.Fatalf("first member: got %s", st)
	}
	for i := 0; i < 3; i++ {
		u := testdb.User(t, db, fmt.Sprintf("w%d@example.test", i), "approved", false)
		if st := reserve(t, db, g, u); st != seats.Waitlist {
			t.Fatalf("waitlisted member %d: got %s", i, st)
		}
		waiting = append(waiting, u)
	}
	return
}

func assertOnlyPromoted(t *testing.T, db *sql.DB, g string, waiting []string, promoted int) {
	t.Helper()
	st := statuses(t, db, g)
	for i, u := range waiting {
		want := seats.Waitlist
		if i == promoted {
			want = seats.Confirmed
		}
		if st[u] != want {
			t.Errorf("waitlisted member %d: status %q, want %q", i, st[u], want)
		}
	}
	if n := confirmedCount(t, db, g); n != 1 {
		t.Errorf("confirmed count = %d, want 1", n)
	}
}

func TestCancelPromotesFirstWaitlisted(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	var promoted []seats.Promotion
	if err := inTx(t, db, func(tx *sql.Tx) (err error) {
		promoted, err = seats.Cancel(context.Background(), tx, g, confirmed)
		return
	}); err != nil {
		t.Fatal(err)
	}
	assertOnlyPromoted(t, db, g, waiting, 0)
	if len(promoted) != 1 || promoted[0].UserID != waiting[0] || promoted[0].GatheringID != g {
		t.Fatalf("promotions reported: %+v", promoted)
	}
}

func TestCancelWithoutReservation(t *testing.T) {
	db, g, _, _ := setup(t)
	stranger := testdb.User(t, db, "s@example.test", "approved", false)
	err := inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Cancel(context.Background(), tx, g, stranger); return err })
	if !errors.Is(err, seats.ErrNoReservation) {
		t.Fatalf("got %v, want ErrNoReservation", err)
	}
}

func TestAccountDeletionPromotesFirstWaitlisted(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	err := inTx(t, db, func(tx *sql.Tx) error {
		if _, err := seats.ReleaseUser(context.Background(), tx, confirmed); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM users WHERE id=$1`, confirmed)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyPromoted(t, db, g, waiting, 0)
}

func TestDeclinePromotesFirstWaitlisted(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	err := inTx(t, db, func(tx *sql.Tx) error {
		if _, err := seats.ReleaseUser(context.Background(), tx, confirmed); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE users SET status='declined' WHERE id=$1`, confirmed)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	assertOnlyPromoted(t, db, g, waiting, 0)
}

func TestPromotionSkipsIneligibleMembers(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	// The first in line was declined without going through ReleaseUser (legacy data).
	if _, err := db.Exec(`UPDATE users SET status='declined' WHERE id=$1`, waiting[0]); err != nil {
		t.Fatal(err)
	}
	if err := inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Cancel(context.Background(), tx, g, confirmed); return err }); err != nil {
		t.Fatal(err)
	}
	st := statuses(t, db, g)
	if st[waiting[0]] != seats.Waitlist || st[waiting[1]] != seats.Confirmed || st[waiting[2]] != seats.Waitlist {
		t.Fatalf("unexpected statuses: %v", st)
	}
}

func TestNewMemberCannotBypassWaitlist(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	// Simulate a seat freed outside the seat package (e.g. a legacy cascade delete).
	if _, err := db.Exec(`DELETE FROM reservations WHERE gathering_id=$1 AND user_id=$2`, g, confirmed); err != nil {
		t.Fatal(err)
	}
	newcomer := testdb.User(t, db, "new@example.test", "approved", false)
	if st := reserve(t, db, g, newcomer); st != seats.Waitlist {
		t.Fatalf("newcomer got %s, want waitlist", st)
	}
	assertOnlyPromoted(t, db, g, waiting, 0)
}

func TestReserveIsIdempotent(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	if st := reserve(t, db, g, confirmed); st != seats.Confirmed {
		t.Fatalf("repeat reserve: %s", st)
	}
	if st := reserve(t, db, g, waiting[1]); st != seats.Waitlist {
		t.Fatalf("repeat waitlist reserve: %s", st)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM reservations WHERE gathering_id=$1`, g).Scan(&n)
	if n != 4 {
		t.Fatalf("reservations = %d, want 4", n)
	}
}

func TestReserveRejectsUnapprovedAndMissing(t *testing.T) {
	db := testdb.Open(t)
	g := testdb.Gathering(t, db, 3)
	pending := testdb.User(t, db, "p@example.test", "pending", false)
	err := inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Reserve(context.Background(), tx, g, pending); return err })
	if !errors.Is(err, seats.ErrNotEligible) {
		t.Fatalf("pending member: %v", err)
	}
	ok := testdb.User(t, db, "ok@example.test", "approved", false)
	err = inTx(t, db, func(tx *sql.Tx) error {
		_, err := seats.Reserve(context.Background(), tx, "00000000-0000-0000-0000-000000000000", ok)
		return err
	})
	if !errors.Is(err, seats.ErrGatheringNotFound) {
		t.Fatalf("missing gathering: %v", err)
	}
}

func TestSetCapacityPromotesInOrderAndRefusesOverbooking(t *testing.T) {
	db, g, _, waiting := setup(t)
	var promoted []seats.Promotion
	if err := inTx(t, db, func(tx *sql.Tx) (err error) {
		promoted, err = seats.SetCapacity(context.Background(), tx, g, 3)
		return
	}); err != nil {
		t.Fatal(err)
	}
	st := statuses(t, db, g)
	if len(promoted) != 2 || promoted[0].UserID != waiting[0] || promoted[1].UserID != waiting[1] ||
		st[waiting[0]] != seats.Confirmed || st[waiting[1]] != seats.Confirmed || st[waiting[2]] != seats.Waitlist {
		t.Fatalf("promoted %+v, statuses %v", promoted, st)
	}
	err := inTx(t, db, func(tx *sql.Tx) error { _, err := seats.SetCapacity(context.Background(), tx, g, 2); return err })
	var capErr *seats.CapacityError
	if !errors.As(err, &capErr) || capErr.Confirmed != 3 {
		t.Fatalf("lowering below confirmed: %v", err)
	}
	var capacity int
	_ = db.QueryRow(`SELECT capacity FROM gatherings WHERE id=$1`, g).Scan(&capacity)
	if capacity != 3 || confirmedCount(t, db, g) != 3 {
		t.Fatalf("refused change must not apply: capacity=%d confirmed=%d", capacity, confirmedCount(t, db, g))
	}
	// Lowering to exactly the confirmed count is allowed and promotes nobody.
	if err := inTx(t, db, func(tx *sql.Tx) (err error) {
		promoted, err = seats.SetCapacity(context.Background(), tx, g, 3)
		return
	}); err != nil || len(promoted) != 0 {
		t.Fatalf("no-op capacity: %v %v", promoted, err)
	}
}

func TestFillReportsNothingWhenFull(t *testing.T) {
	db, g, _, _ := setup(t)
	var promoted []seats.Promotion
	if err := inTx(t, db, func(tx *sql.Tx) (err error) { promoted, err = seats.Fill(context.Background(), tx, g); return }); err != nil || len(promoted) != 0 {
		t.Fatalf("fill on a full gathering: %v %v", promoted, err)
	}
}

func TestStartedGatheringRejectsBookingsAndNeverPromotes(t *testing.T) {
	db, g, confirmed, waiting := setup(t)
	if _, err := db.Exec(`UPDATE gatherings SET starts_on=current_date - 1 WHERE id=$1`, g); err != nil {
		t.Fatal(err)
	}
	late := testdb.User(t, db, "late@example.test", "approved", false)
	err := inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Reserve(context.Background(), tx, g, late); return err })
	if !errors.Is(err, seats.ErrGatheringStarted) {
		t.Fatalf("booking a past gathering: %v", err)
	}
	if st := reserve(t, db, g, confirmed); st != seats.Confirmed {
		t.Fatalf("existing reservation status: %s", st)
	}
	var promoted []seats.Promotion
	if err := inTx(t, db, func(tx *sql.Tx) (err error) {
		promoted, err = seats.Cancel(context.Background(), tx, g, confirmed)
		return
	}); err != nil {
		t.Fatal(err)
	}
	if len(promoted) != 0 || statuses(t, db, g)[waiting[0]] != seats.Waitlist {
		t.Fatal("nobody should be promoted into a gathering that already happened")
	}
}

// Capacity changes racing bookings and cancellations never overbook.
func TestConcurrentCapacityChangesNeverOverbook(t *testing.T) {
	db := testdb.Open(t)
	g := testdb.Gathering(t, db, 2)
	var users []string
	for i := 0; i < 20; i++ {
		users = append(users, testdb.User(t, db, fmt.Sprintf("cc%d@example.test", i), "approved", false))
	}
	var wg sync.WaitGroup
	for i, u := range users {
		wg.Add(1)
		go func(i int, u string) {
			defer wg.Done()
			_ = inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Reserve(context.Background(), tx, g, u); return err })
			if i%4 == 0 {
				_ = inTx(t, db, func(tx *sql.Tx) error { _, err := seats.SetCapacity(context.Background(), tx, g, 2+i/4); return err })
			}
			if i%5 == 0 {
				_ = inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Cancel(context.Background(), tx, g, u); return err })
			}
		}(i, u)
	}
	wg.Wait()
	var capacity, confirmed, waiting int
	_ = db.QueryRow(`SELECT capacity FROM gatherings WHERE id=$1`, g).Scan(&capacity)
	_ = db.QueryRow(`SELECT count(*) FILTER (WHERE status='confirmed'), count(*) FILTER (WHERE status='waitlist') FROM reservations WHERE gathering_id=$1`, g).Scan(&confirmed, &waiting)
	if confirmed > capacity {
		t.Fatalf("overbooked: %d confirmed for capacity %d", confirmed, capacity)
	}
	if waiting > 0 && confirmed != capacity {
		t.Fatalf("free seat left while %d wait: confirmed %d capacity %d", waiting, confirmed, capacity)
	}
	// Strict FIFO: confirmed places always form a prefix of the queue, so no
	// waitlisted member reserved before any confirmed one.
	var inverted int
	_ = db.QueryRow(`SELECT count(*) FROM reservations c JOIN reservations w ON c.gathering_id=w.gathering_id
WHERE c.gathering_id=$1 AND c.status='confirmed' AND w.status='waitlist' AND (c.created_at, c.id) > (w.created_at, w.id)`, g).Scan(&inverted)
	if inverted != 0 {
		t.Fatalf("%d confirmed/waitlist pairs out of FIFO order", inverted)
	}
}

// Concurrent "keep" requests for a 1-seat gathering: exactly one confirmed,
// everyone else waitlisted, no duplicates, and FIFO order equals commit order.
func TestConcurrentReserveNeverOverbooks(t *testing.T) {
	db := testdb.Open(t)
	g := testdb.Gathering(t, db, 1)
	const n = 25
	users := make([]string, n)
	for i := range users {
		users[i] = testdb.User(t, db, fmt.Sprintf("u%d@example.test", i), "approved", false)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, n*2)
	for _, u := range users {
		for k := 0; k < 2; k++ { // each member double-clicks
			wg.Add(1)
			go func(u string) {
				defer wg.Done()
				<-start
				tx, err := db.BeginTx(context.Background(), nil)
				if err != nil {
					errs <- err
					return
				}
				defer tx.Rollback()
				if _, err := seats.Reserve(context.Background(), tx, g, u); err != nil {
					errs <- err
					return
				}
				errs <- tx.Commit()
			}(u)
		}
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent reserve: %v", err)
		}
	}
	st := statuses(t, db, g)
	if len(st) != n {
		t.Fatalf("reservations = %d, want %d (duplicates or losses)", len(st), n)
	}
	if c := confirmedCount(t, db, g); c != 1 {
		t.Fatalf("confirmed = %d, want 1", c)
	}

	// Concurrent cancellations by the confirmed member and two waitlisted ones
	// must still leave exactly one confirmed seat, given to the earliest remaining.
	var order []string
	rows, _ := db.Query(`SELECT user_id::text FROM reservations WHERE gathering_id=$1 ORDER BY created_at,id`, g)
	for rows.Next() {
		var u string
		_ = rows.Scan(&u)
		order = append(order, u)
	}
	rows.Close()
	cancelers := []string{order[0], order[1], order[2]}
	var wg2 sync.WaitGroup
	for _, u := range cancelers {
		wg2.Add(1)
		go func(u string) {
			defer wg2.Done()
			_ = inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Cancel(context.Background(), tx, g, u); return err })
		}(u)
	}
	wg2.Wait()
	st = statuses(t, db, g)
	if c := confirmedCount(t, db, g); c != 1 {
		t.Fatalf("after cancels confirmed = %d, want 1", c)
	}
	if st[order[3]] != seats.Confirmed {
		t.Fatalf("earliest remaining member not promoted: %v", st[order[3]])
	}
}

// A member reserving while an admin declines them must not keep a seat.
func TestDeclineRacingReserve(t *testing.T) {
	for i := 0; i < 10; i++ {
		db := testdb.Open(t)
		g := testdb.Gathering(t, db, 1)
		u := testdb.User(t, db, "r@example.test", "approved", false)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = inTx(t, db, func(tx *sql.Tx) error { _, err := seats.Reserve(context.Background(), tx, g, u); return err })
		}()
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(i) * time.Millisecond)
			_ = inTx(t, db, func(tx *sql.Tx) error {
				if _, err := seats.ReleaseUser(context.Background(), tx, u); err != nil {
					return err
				}
				_, err := tx.Exec(`UPDATE users SET status='declined' WHERE id=$1`, u)
				return err
			})
		}()
		wg.Wait()
		if c := confirmedCount(t, db, g); c != 0 {
			t.Fatalf("iteration %d: declined member holds %d seat(s)", i, c)
		}
	}
}
