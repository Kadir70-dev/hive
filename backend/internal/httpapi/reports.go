package httpapi

import (
	"database/sql"
	"net/http"
	"strings"

	"thehivesociety/api/internal/ratelimit"
)

// Safety reports are admin-mediated. Members never see or select other
// members: they describe who was involved (and optionally which gathering),
// and an administrator links the report to a member, records an outcome and,
// if the reporter asked for it, applies a block on the reporter's behalf.
// Audit events record that something happened, never the report contents.

var reportCategories = map[string]bool{"harassment": true, "safety": true, "inappropriate": true, "other": true}
var reportStatuses = map[string]bool{"open": true, "in_review": true, "resolved": true, "dismissed": true}

func (s *Server) createReport(w http.ResponseWriter, r *http.Request) {
	if !s.approved(w, r) {
		return
	}
	if res := s.limiter.Hit(r.Context(), s.limits.ReportsPerMember, ratelimit.Key("report", own(r).ID)); !res.Allowed {
		s.tooMany(w, r, res)
		return
	}
	var v struct {
		Category, GatheringID, SubjectDescription, Reason string
		BlockRequested                                    bool
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.Reason, v.SubjectDescription = strings.TrimSpace(v.Reason), strings.TrimSpace(v.SubjectDescription)
	if v.Category == "" {
		v.Category = "other"
	}
	if !reportCategories[v.Category] || len(v.Reason) < 10 || !validText(v.Reason, 2000, true) || !validText(v.SubjectDescription, 500, true) ||
		(v.GatheringID != "" && !validUUID(v.GatheringID)) {
		s.fail(w, r, 400, "invalid_report", "please choose a category and describe what happened (10 to 2000 characters)")
		return
	}
	var gathering any
	if v.GatheringID != "" {
		// Only gatherings the reporter booked can be referenced.
		var ok bool
		err := s.db.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM reservations WHERE gathering_id=$1 AND user_id=$2)
  OR EXISTS(SELECT 1 FROM attendance WHERE gathering_id=$1 AND user_id=$2)`, v.GatheringID, own(r).ID).Scan(&ok)
		if err != nil {
			s.internal(w, r, "report failed", err)
			return
		}
		if !ok {
			s.fail(w, r, 400, "invalid_gathering", "you can only refer to gatherings you booked")
			return
		}
		gathering = v.GatheringID
	}
	var id string
	err := s.db.QueryRowContext(r.Context(), `INSERT INTO member_reports(reporter_id,category,gathering_id,subject_description,reason,block_requested)
VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text`, own(r).ID, v.Category, gathering, v.SubjectDescription, v.Reason, v.BlockRequested).Scan(&id)
	if err != nil {
		s.internal(w, r, "report failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "report.create", id, v.Category)
	reply(w, 201, map[string]any{"ok": true, "id": id})
}

// myReports lists the reporter's own reports with their status only;
// administrators' notes stay internal.
func (s *Server) myReports(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), `SELECT m.id::text, m.category, COALESCE(g.title,''), m.status, m.created_at::text, m.updated_at::text
FROM member_reports m LEFT JOIN gatherings g ON g.id=m.gathering_id WHERE m.reporter_id=$1 ORDER BY m.created_at DESC LIMIT 100`, own(r).ID)
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	out := []any{}
	for rows.Next() {
		var id, cat, gathering, status, created, updated string
		if err := rows.Scan(&id, &cat, &gathering, &status, &created, &updated); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		out = append(out, map[string]any{"id": id, "category": cat, "gathering_title": gathering, "status": status, "created_at": created, "updated_at": updated})
	}
	reply(w, 200, out)
}

func (s *Server) adminReports(w http.ResponseWriter, r *http.Request) {
	p := pageParams(r, 25, 100)
	status := r.URL.Query().Get("status")
	if status != "" && !reportStatuses[status] {
		s.fail(w, r, 400, "invalid_status", "invalid status filter")
		return
	}
	var total int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM member_reports WHERE $1='' OR status=$1`, status).Scan(&total); err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `
SELECT m.id::text, m.category, m.status, m.reason, m.subject_description, m.block_requested, m.resolution_note,
       m.created_at::text, COALESCE(m.resolved_at::text,''),
       m.reporter_id::text, rep.first_name,
       COALESCE(m.reported_member_id::text,''), COALESCE(sub.first_name,''),
       COALESCE(m.gathering_id::text,''), COALESCE(g.title,''),
       EXISTS(SELECT 1 FROM member_blocks b WHERE b.report_id=m.id)
FROM member_reports m
JOIN users rep ON rep.id=m.reporter_id
LEFT JOIN users sub ON sub.id=m.reported_member_id
LEFT JOIN gatherings g ON g.id=m.gathering_id
WHERE $1='' OR m.status=$1
ORDER BY (m.status IN ('open','in_review')) DESC, m.created_at DESC, m.id LIMIT $2 OFFSET $3`, status, p.Size, p.offset())
	if err != nil {
		s.internal(w, r, "query failed", err)
		return
	}
	defer rows.Close()
	items := []any{}
	for rows.Next() {
		var id, cat, st, reason, subject, note, created, resolved, reporterID, reporterName, subjectID, subjectName, gID, gTitle string
		var blockReq, blocked bool
		if err := rows.Scan(&id, &cat, &st, &reason, &subject, &blockReq, &note, &created, &resolved, &reporterID, &reporterName,
			&subjectID, &subjectName, &gID, &gTitle, &blocked); err != nil {
			s.internal(w, r, "query failed", err)
			return
		}
		items = append(items, map[string]any{"id": id, "category": cat, "status": st, "reason": reason, "subject_description": subject,
			"block_requested": blockReq, "resolution_note": note, "created_at": created, "resolved_at": resolved,
			"reporter_id": reporterID, "reporter_name": reporterName, "reported_member_id": subjectID, "reported_member_name": subjectName,
			"gathering_id": gID, "gathering_title": gTitle, "block_applied": blocked})
	}
	reply(w, 200, paged(items, p, total))
}

// updateReport sets status, an internal resolution note and (optionally) the
// member the report concerns, chosen by the admin from the gathering roster.
func (s *Server) updateReport(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var v struct {
		Status, ResolutionNote, ReportedMemberID string
	}
	if !s.parse(w, r, &v) {
		return
	}
	v.ResolutionNote = strings.TrimSpace(v.ResolutionNote)
	if !reportStatuses[v.Status] || !validText(v.ResolutionNote, 2000, true) || (v.ReportedMemberID != "" && !validUUID(v.ReportedMemberID)) {
		s.fail(w, r, 400, "invalid_report_update", "invalid status, note or member")
		return
	}
	var reported any
	if v.ReportedMemberID != "" {
		reported = v.ReportedMemberID
	}
	res, err := s.db.ExecContext(r.Context(), `
UPDATE member_reports SET status=$2, resolution_note=$3, updated_at=now(),
  reported_member_id = CASE WHEN $4::uuid IS NULL THEN reported_member_id ELSE $4::uuid END,
  resolved_at = CASE WHEN $2 IN ('resolved','dismissed') THEN COALESCE(resolved_at, now()) ELSE NULL END,
  resolved_by = CASE WHEN $2 IN ('resolved','dismissed') THEN $5::uuid ELSE NULL END
WHERE id=$1 AND ($4::uuid IS NULL OR EXISTS(SELECT 1 FROM users WHERE id=$4::uuid AND id<>member_reports.reporter_id))`,
		id, v.Status, v.ResolutionNote, reported, own(r).ID)
	if err != nil {
		s.internal(w, r, "update failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "report or member not found")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "report.status", id, v.Status)
	reply(w, 200, map[string]any{"ok": true})
}

// applyBlock records, on the reporter's behalf, that the reporter and the
// linked member must not be booked into the same gathering.
func (s *Server) applyBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	var reporter, subject sql.NullString
	err := s.db.QueryRowContext(r.Context(), `SELECT reporter_id::text, reported_member_id::text FROM member_reports WHERE id=$1`, id).Scan(&reporter, &subject)
	if isNoRows(err) {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	if err != nil {
		s.internal(w, r, "block failed", err)
		return
	}
	if !subject.Valid {
		s.fail(w, r, 409, "member_not_linked", "link the report to a member first")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `INSERT INTO member_blocks(blocker_id,blocked_id,created_by,report_id) VALUES($1,$2,$3,$4)
ON CONFLICT (blocker_id,blocked_id) DO UPDATE SET report_id=EXCLUDED.report_id, created_by=EXCLUDED.created_by`,
		reporter.String, subject.String, own(r).ID, id)
	if err != nil {
		s.internal(w, r, "block failed", err)
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "block.create", id, "")
	// Tell the admin straight away if both already hold places somewhere.
	var conflicts int
	if err := s.db.QueryRowContext(r.Context(), `SELECT count(*) FROM (`+conflictQuery+` AND m.id=$1) c`, id).Scan(&conflicts); err != nil {
		s.internal(w, r, "block applied; conflict check failed", err)
		return
	}
	reply(w, 200, map[string]any{"ok": true, "conflicts": conflicts})
}

func (s *Server) removeBlock(w http.ResponseWriter, r *http.Request) {
	id, ok := s.pathID(w, r)
	if !ok {
		return
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM member_blocks WHERE report_id=$1`, id)
	if err != nil {
		s.internal(w, r, "unblock failed", err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		s.fail(w, r, 404, "not_found", "no block for this report")
		return
	}
	s.audit(r.Context(), s.db, own(r).ID, "block.remove", id, "")
	reply(w, 200, map[string]any{"ok": true})
}
