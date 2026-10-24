package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"thehivesociety/api/internal/ratelimit"
)

type ctxKey int

const userKey ctxKey = 1

// reply writes a JSON response that is never cached.
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// fail writes a safe error. "error" stays a plain string for existing clients;
// "code" and "request_id" are additive. Internal details are never included.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	reply(w, status, map[string]any{"error": msg, "code": code, "request_id": middleware.GetReqID(r.Context())})
}

func (s *Server) internal(w http.ResponseWriter, r *http.Request, msg string, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		s.log.Warn("request timed out", "request_id", middleware.GetReqID(r.Context()), "op", msg)
		s.fail(w, r, 503, "timeout", "the service is busy, please try again")
		return
	}
	s.log.Error(msg, "request_id", middleware.GetReqID(r.Context()), "err", err)
	s.fail(w, r, 500, "internal", msg)
}

func (s *Server) tooMany(w http.ResponseWriter, r *http.Request, res ratelimit.Result) {
	secs := int(res.RetryAfter.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	reply(w, 429, map[string]any{
		"error": "too many attempts, please try again later", "code": "rate_limited",
		"retry_after": secs, "request_id": middleware.GetReqID(r.Context()),
	})
}

// parse decodes a JSON body. Unknown size beyond maxBody is rejected by limitBody.
func (s *Server) parse(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			s.fail(w, r, 413, "body_too_large", "request body too large")
			return false
		}
		s.fail(w, r, 400, "invalid_json", "invalid JSON")
		return false
	}
	return true
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func validUUID(s string) bool { return uuidRe.MatchString(s) }

// pathID returns a validated UUID URL parameter or writes 404.
func (s *Server) pathID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := chi.URLParam(r, "id")
	if !validUUID(id) {
		s.fail(w, r, 404, "not_found", "not found")
		return "", false
	}
	return id, true
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				s.log.Error("panic", "request_id", middleware.GetReqID(r.Context()), "panic", rec)
				s.fail(w, r, 500, "internal", "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// logRequests records method, route pattern, status and duration only — no
// IPs, emails, bodies or query strings.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		s.log.Info("request", "request_id", middleware.GetReqID(r.Context()), "method", r.Method,
			"route", route, "status", ww.Status(), "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > s.maxBody {
			s.fail(w, r, 413, "body_too_large", "request body too large")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.maxBody)
		next.ServeHTTP(w, r)
	})
}

// requestTimeout bounds all handler and database work for a request.
func (s *Server) requestTimeout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// checkOrigin is the CSRF defence for cookie-authenticated requests (on top
// of SameSite=Lax). A state-changing request that carries a session cookie
// must come with an allowed Origin header: browsers always send Origin on
// POST/PUT/PATCH/DELETE, and the Next.js proxy forwards it. Cookie-less
// server-to-server calls without Origin are still accepted.
func (s *Server) checkOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && len(s.origins) > 0 {
			o := r.Header.Get("Origin")
			_, cookieErr := r.Cookie(sessionCookie)
			if (o != "" && !s.origins[o]) || (o == "" && cookieErr == nil) {
				s.fail(w, r, 403, "origin_denied", "origin denied")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// apiRateLimit is a coarse per-IP ceiling for every API call.
func (s *Server) apiRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if res := s.limiter.Hit(r.Context(), s.limits.APIPerIP, ratelimit.Key(s.ips.IP(r))); !res.Allowed {
			s.tooMany(w, r, res)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, err := s.current(r)
		if errors.Is(err, errNoSession) {
			s.fail(w, r, 401, "unauthenticated", "authentication required")
			return
		}
		if err != nil {
			s.internal(w, r, "session lookup failed", err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, u)))
	})
}

func own(r *http.Request) *user { return r.Context().Value(userKey).(*user) }

// requireAdmin admits only verified admins whose session passed TOTP within
// the step-up window. Clients get a specific code so the UI can prompt.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := own(r)
		switch {
		case !u.IsAdmin || !u.EmailVerified:
			s.fail(w, r, 403, "forbidden", "admin required")
		case !u.MFAEnabled:
			s.fail(w, r, 403, "mfa_enrollment_required", "set up two-factor authentication to use admin tools")
		case !u.MFAVerified:
			s.fail(w, r, 403, "mfa_required", "enter your two-factor code to use admin tools")
		default:
			next.ServeHTTP(w, r)
		}
	})
}

func (s *Server) approved(w http.ResponseWriter, r *http.Request) bool {
	if own(r).Status != "approved" && !own(r).IsAdmin {
		s.fail(w, r, 403, "approval_required", "approval required")
		return false
	}
	return true
}
