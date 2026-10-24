package httpapi

import (
	"context"
	"database/sql"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/seats"
)

// Notifications are enqueued in the caller's transaction, so a member is only
// emailed about a change that actually committed. Booking and membership
// emails are service messages and are sent regardless of the optional
// "email updates" preference, which covers announcements only.

type queryer interface {
	execQuerier
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func gatheringInfo(ctx context.Context, q execQuerier, gatheringID string) (mail.Gathering, error) {
	var g mail.Gathering
	err := q.QueryRowContext(ctx, `SELECT title, to_char(starts_on,'Dy DD Mon YYYY'), to_char(starts_at,'HH24:MI'), place FROM gatherings WHERE id=$1`, gatheringID).
		Scan(&g.Title, &g.Date, &g.Time, &g.Place)
	return g, err
}

func memberContact(ctx context.Context, q execQuerier, userID string) (email, name string, err error) {
	err = q.QueryRowContext(ctx, `SELECT email, first_name FROM users WHERE id=$1`, userID).Scan(&email, &name)
	return
}

// notifyPromotions emails every member who just received a confirmed place.
func (s *Server) notifyPromotions(ctx context.Context, q execQuerier, promoted []seats.Promotion) error {
	for _, p := range promoted {
		g, err := gatheringInfo(ctx, q, p.GatheringID)
		if err != nil {
			return err
		}
		email, name, err := memberContact(ctx, q, p.UserID)
		if err != nil {
			return err
		}
		if err := mail.Enqueue(ctx, q, p.UserID, mail.TemplateWaitlistPromote, mail.WaitlistPromoted(email, name, s.appURL, g)); err != nil {
			return err
		}
		s.audit(ctx, q, "", "reservation.promoted", p.UserID, p.GatheringID)
	}
	return nil
}

// notifyBooking confirms a new reservation (confirmed place or waitlist).
func (s *Server) notifyBooking(ctx context.Context, q execQuerier, gatheringID, userID, status string) error {
	g, err := gatheringInfo(ctx, q, gatheringID)
	if err != nil {
		return err
	}
	email, name, err := memberContact(ctx, q, userID)
	if err != nil {
		return err
	}
	return mail.Enqueue(ctx, q, userID, mail.TemplateBookingConfirm, mail.BookingConfirmation(email, name, s.appURL, g, status == seats.Waitlist))
}

func (s *Server) notifyCancellation(ctx context.Context, q execQuerier, gatheringID, userID string) error {
	g, err := gatheringInfo(ctx, q, gatheringID)
	if err != nil {
		return err
	}
	email, name, err := memberContact(ctx, q, userID)
	if err != nil {
		return err
	}
	return mail.Enqueue(ctx, q, userID, mail.TemplateBookingCancel, mail.BookingCancelled(email, name, g))
}

// notifyReservationHolders emails everyone with a reservation for a gathering.
// build receives the member's email and name.
func notifyReservationHolders(ctx context.Context, q queryer, gatheringID, template string, build func(email, name string) mail.Message) error {
	rows, err := q.QueryContext(ctx, `SELECT u.id::text, u.email, u.first_name FROM reservations r JOIN users u ON u.id=r.user_id WHERE r.gathering_id=$1 ORDER BY r.created_at, r.id`, gatheringID)
	if err != nil {
		return err
	}
	type holder struct{ id, email, name string }
	var hs []holder
	for rows.Next() {
		var h holder
		if err := rows.Scan(&h.id, &h.email, &h.name); err != nil {
			rows.Close()
			return err
		}
		hs = append(hs, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, h := range hs {
		if err := mail.Enqueue(ctx, q, h.id, template, build(h.email, h.name)); err != nil {
			return err
		}
	}
	return nil
}

// notifyAdminsNewMember alerts every verified admin that a member confirmed
// their address and awaits approval.
func (s *Server) notifyAdminsNewMember(ctx context.Context, q queryer, userID string) error {
	var name, area string
	var isAdmin bool
	if err := q.QueryRowContext(ctx, `SELECT first_name, area, is_admin FROM users WHERE id=$1`, userID).Scan(&name, &area, &isAdmin); err != nil {
		return err
	}
	if isAdmin {
		return nil
	}
	rows, err := q.QueryContext(ctx, `SELECT email FROM users WHERE is_admin AND email_verified_at IS NOT NULL ORDER BY email`)
	if err != nil {
		return err
	}
	var admins []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			rows.Close()
			return err
		}
		admins = append(admins, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, a := range admins {
		if err := mail.Enqueue(ctx, q, "", mail.TemplateNewMemberAlert, mail.NewMemberAlert(a, name, area, s.appURL)); err != nil {
			return err
		}
	}
	return nil
}
