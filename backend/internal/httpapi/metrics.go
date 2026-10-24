package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// metrics serves Prometheus text-format gauges for monitoring. It is disabled
// unless METRICS_TOKEN is set, requires "Authorization: Bearer <token>", and
// Caddy refuses /internal/* from the internet, so it is scraped only from the
// private Docker network. Values are counts and ages, never personal data.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	if s.metricsToken == "" {
		s.fail(w, r, 404, "not_found", "not found")
		return
	}
	given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(given), []byte(s.metricsToken)) != 1 {
		s.fail(w, r, 401, "unauthenticated", "authentication required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var b strings.Builder
	gauge := func(name, help string, v any) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %v\n", name, help, name, name, v)
	}
	gauge("hive_up", "API process is serving.", 1)
	dbUp := 1
	var pending, failed24h, deletions, stuck, sessions, conflicts, reports int
	var oldest float64
	err := s.db.QueryRowContext(ctx, `SELECT
  (SELECT count(*) FROM email_outbox WHERE sent_at IS NULL AND failed_at IS NULL),
  COALESCE((SELECT EXTRACT(EPOCH FROM now()-min(created_at)) FROM email_outbox WHERE sent_at IS NULL AND failed_at IS NULL),0),
  (SELECT count(*) FROM email_outbox WHERE failed_at > now()-interval '24 hours'),
  (SELECT count(*) FROM storage_deletions),
  (SELECT count(*) FROM storage_deletions WHERE attempts >= 20),
  (SELECT count(*) FROM sessions WHERE expires_at > now()),
  (SELECT count(*) FROM (`+conflictQuery+`) c),
  (SELECT count(*) FROM member_reports WHERE status IN ('open','in_review'))`).
		Scan(&pending, &oldest, &failed24h, &deletions, &stuck, &sessions, &conflicts, &reports)
	if err != nil {
		dbUp = 0
		s.log.Error("metrics query failed", "err", err)
	}
	gauge("hive_db_up", "PostgreSQL answered the metrics query.", dbUp)
	redisDegraded := 0
	if s.limiter.Degraded() {
		redisDegraded = 1
	}
	gauge("hive_ratelimit_redis_degraded", "Rate limiting fell back to in-process counters.", redisDegraded)
	gauge("hive_storage_configured", "Photo storage (R2) is configured.", map[bool]int{true: 1, false: 0}[s.store != nil])
	if dbUp == 1 {
		gauge("hive_email_outbox_pending", "Emails waiting to be delivered.", pending)
		gauge("hive_email_outbox_oldest_pending_seconds", "Age of the oldest undelivered email.", int64(oldest))
		gauge("hive_email_failed_24h", "Emails abandoned in the last 24 hours.", failed24h)
		gauge("hive_storage_deletions_pending", "Photo objects waiting to be deleted.", deletions)
		gauge("hive_storage_deletions_stuck", "Photo deletions that exhausted their retries.", stuck)
		gauge("hive_sessions_active", "Signed-in sessions.", sessions)
		gauge("hive_safety_conflicts_open", "Undecided safety conflicts on upcoming gatherings.", conflicts)
		gauge("hive_reports_open", "Safety reports open or in review.", reports)
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(b.String()))
}
