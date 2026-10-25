package httpapi

import "net/http"

// adminCampaigns returns aggregated first-party signup sources.
// Only an authenticated administrator with recent MFA can access this route.
// It returns no email, name, user ID, IP address, or untrusted raw referrer.
func (s *Server) adminCampaigns(w http.ResponseWriter, r *http.Request) {
  rows, err := s.db.QueryContext(r.Context(), `
SELECT utm_source, utm_medium, utm_campaign,
       count(*)::bigint, count(*) FILTER (WHERE verified_at IS NOT NULL)::bigint
FROM signup_attribution
GROUP BY utm_source, utm_medium, utm_campaign
ORDER BY count(*) FILTER (WHERE verified_at IS NOT NULL) DESC, count(*) DESC
LIMIT 100`)
  if err != nil { s.internal(w, r, "query failed", err); return }
  defer rows.Close()
  items := []any{}
  var totalStarted, totalVerified int64
  for rows.Next() {
    var source, medium, campaign string
    var started, verified int64
    if err := rows.Scan(&source, &medium, &campaign, &started, &verified); err != nil {
      s.internal(w, r, "query failed", err); return
    }
    totalStarted += started
    totalVerified += verified
    items = append(items, map[string]any{
      "source": source, "medium": medium, "campaign": campaign,
      "started": started, "verified": verified,
    })
  }
  if err := rows.Err(); err != nil { s.internal(w, r, "query failed", err); return }
  reply(w, 200, map[string]any{"items": items, "tracked_started": totalStarted, "tracked_verified": totalVerified})
}
