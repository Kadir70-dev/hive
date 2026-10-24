package httpapi

import (
	"errors"
	"net/http"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/seats"
)

// A safety conflict exists when an admin-applied block links two members who
// both still hold a reservation for the same upcoming gathering (because they
// booked before the block). Nothing is cancelled automatically: an admin
// decides, and neither member is ever told about the other or the report.

const conflictQuery = `
SELECT g.id::text, g.title, g.starts_on::text, g.starts_at::text, m.id::text,
       rep.id::text, rep.first_name, rr.status, sub.id::text, sub.first_name, sr.status
FROM member_blocks b
JOIN member_reports m ON m.id=b.report_id
JOIN reservations rr ON rr.user_id=b.blocker_id
JOIN reservations sr ON sr.user_id=b.blocked_id AND sr.gathering_id=rr.gathering_id
JOIN gatherings g ON g.id=rr.gathering_id
JOIN users rep ON rep.id=b.blocker_id
JOIN users sub ON sub.id=b.blocked_id
WHERE ((g.starts_on + g.starts_at) AT TIME ZONE 'Asia/Dubai') > now()
  AND NOT EXISTS (SELECT 1 FROM safety_conflict_decisions d WHERE d.gathering_id=g.id AND d.report_id=m.id)`

func (s *Server) safetyConflicts(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), conflictQuery+` ORDER BY g.starts_on, g.starts_at LIMIT 200`)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var gid, title, date, t, rid, repID, repName, repStatus, subID, subName, subStatus string
		if err := rows.Scan(&gid, &title, &date, &t, &rid, &repID, &repName, &repStatus, &subID, &subName, &subStatus); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"gathering_id": gid, "gathering_title": title, "starts_on": date, "starts_at": t, "report_id": rid,
			"reporter": map[string]any{"id": repID, "first_name": repName, "status": repStatus},
			"reported": map[string]any{"id": subID, "first_name": subName, "status": subStatus}})
	}
	reply(w, 200, out)
}

// resolveConflict records an explicit admin decision. Releasing a place goes
// through seats.Cancel (gathering lock, FIFO promotion, notifications).
func (s *Server) resolveConflict(w http.ResponseWriter, r *http.Request) {
	var v struct{ GatheringID, ReportID, Decision, Note string }
	if !s.parse(w, r, &v) {
		return
	}
	if !validUUID(v.GatheringID) || !validUUID(v.ReportID) || !validText(v.Note, 1000, true) ||
		(v.Decision != "acknowledged" && v.Decision != "released_reporter" && v.Decision != "released_reported") {
		s.fail(w, r, 400, "invalid_decision", "invalid conflict decision")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		s.internal(w, r, "resolution failed", err)
		return
	}
	defer tx.Rollback()
	// Serialise with bookings, cancellations and other admins on this gathering.
	if _, err = tx.ExecContext(r.Context(), `SELECT 1 FROM gatherings WHERE id=$1 FOR UPDATE`, v.GatheringID); err != nil {
		s.internal(w, r, "resolution failed", err)
		return
	}
	var reporter, reported string
	err = tx.QueryRowContext(r.Context(), conflictQuery+` AND g.id=$1 AND m.id=$2`, v.GatheringID, v.ReportID).
		Scan(new(string), new(string), new(string), new(string), new(string), &reporter, new(string), new(string), &reported, new(string), new(string))
	if isNoRows(err) {
		s.fail(w, r, 409, "conflict_gone", "this conflict no longer exists or was already decided")
		return
	}
	if err != nil {
		s.internal(w, r, "resolution failed", err)
		return
	}
	if v.Decision != "acknowledged" {
		release := reporter
		if v.Decision == "released_reported" {
			release = reported
		}
		var promoted []seats.Promotion
		promoted, err = seats.Cancel(r.Context(), tx, v.GatheringID, release)
		if err == nil {
			err = s.notifyPromotions(r.Context(), tx, promoted)
		}
		if err == nil {
			var g mail.Gathering
			var email, name string
			if g, err = gatheringInfo(r.Context(), tx, v.GatheringID); err == nil {
				if email, name, err = memberContact(r.Context(), tx, release); err == nil {
					err = mail.Enqueue(r.Context(), tx, release, mail.TemplatePlaceReleased, mail.PlaceReleased(email, name, g))
				}
			}
		}
	}
	if err == nil {
		_, err = tx.ExecContext(r.Context(), `INSERT INTO safety_conflict_decisions(gathering_id,report_id,decision,note,decided_by) VALUES($1,$2,$3,$4,$5)`,
			v.GatheringID, v.ReportID, v.Decision, v.Note, own(r).ID)
	}
	if err == nil {
		s.audit(r.Context(), tx, own(r).ID, "safety_conflict.resolve", v.ReportID, v.Decision)
		err = tx.Commit()
	}
	if errors.Is(err, seats.ErrNoReservation) {
		s.fail(w, r, 409, "conflict_gone", "this conflict no longer exists or was already decided")
		return
	}
	if err != nil {
		s.internal(w, r, "resolution failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true})
}
