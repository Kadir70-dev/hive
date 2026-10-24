// Package httpapi implements The Hive Society JSON API.
package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"thehivesociety/api/internal/clientip"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
)

// Limits groups the rate-limit rules. Zero values fall back to DefaultLimits.
type Limits struct {
	LoginPerIP      ratelimit.Rule
	SignupPerIP     ratelimit.Rule
	LoginPerAccount ratelimit.Rule // counts failed logins per email
	APIPerIP        ratelimit.Rule
	HashWait        time.Duration // how long a request may wait for a hashing slot
}

// DefaultLimits are the production limits.
func DefaultLimits() Limits {
	return Limits{
		LoginPerIP:      ratelimit.Rule{Name: "login-ip", Limit: 30, Window: 10 * time.Minute},
		SignupPerIP:     ratelimit.Rule{Name: "signup-ip", Limit: 10, Window: time.Hour},
		LoginPerAccount: ratelimit.Rule{Name: "login-acct", Limit: 5, Window: 15 * time.Minute},
		APIPerIP:        ratelimit.Rule{Name: "api-ip", Limit: 600, Window: time.Minute},
		HashWait:        3 * time.Second,
	}
}

// Config wires the API's dependencies.
type Config struct {
	DB             *sql.DB
	Redis          redis.UniversalClient // optional; nil uses in-process limits only
	Hasher         *password.Hasher
	Limiter        *ratelimit.Limiter
	IPs            *clientip.Resolver
	Log            *slog.Logger
	SecureCookies  bool
	AllowedOrigins []string
	RequestTimeout time.Duration // bounds handler + database work per request
	MaxBodyBytes   int64
	Limits         Limits
}

// Server holds handler state.
type Server struct {
	db      *sql.DB
	rdb     redis.UniversalClient
	hasher  *password.Hasher
	limiter *ratelimit.Limiter
	ips     *clientip.Resolver
	log     *slog.Logger
	secure  bool
	origins map[string]bool
	timeout time.Duration
	maxBody int64
	limits  Limits
}

// New builds the HTTP handler.
func New(c Config) http.Handler {
	s := &Server{
		db: c.DB, rdb: c.Redis, hasher: c.Hasher, limiter: c.Limiter, ips: c.IPs, log: c.Log,
		secure: c.SecureCookies, origins: map[string]bool{}, timeout: c.RequestTimeout,
		maxBody: c.MaxBodyBytes, limits: c.Limits,
	}
	if s.log == nil {
		s.log = slog.Default()
	}
	if s.limiter == nil {
		s.limiter = ratelimit.New(c.Redis, s.log)
	}
	if s.hasher == nil {
		s.hasher = password.NewHasher(4)
	}
	if s.ips == nil {
		s.ips, _ = clientip.New("", "")
	}
	if s.timeout <= 0 {
		s.timeout = 10 * time.Second
	}
	if s.maxBody <= 0 {
		s.maxBody = 64 << 10
	}
	if s.limits == (Limits{}) {
		s.limits = DefaultLimits()
	}
	for _, o := range c.AllowedOrigins {
		if o != "" {
			s.origins[o] = true
		}
	}
	return s.routes()
}

func (s *Server) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, s.recoverer, s.logRequests, s.securityHeaders, s.limitBody, s.requestTimeout)
	r.Get("/health", s.health)
	r.Get("/ready", s.ready)
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.checkOrigin, s.apiRateLimit)
		r.Post("/signup", s.signup)
		r.Post("/login", s.login)
		r.Post("/logout", s.logout)
		r.Post("/founding-signups", s.foundingSignup)
		r.Group(func(r chi.Router) {
			r.Use(s.requireUser)
			r.Get("/me", s.me)
			r.Patch("/me", s.updateMe)
			r.Delete("/me", s.deleteMe)
			r.Get("/gatherings", s.gatherings)
			r.Post("/gatherings/{id}/keep", s.keep)
			r.Post("/gatherings/{id}/cancel", s.cancel)
			r.Get("/announcements", s.announcements)
			r.Get("/partners", s.partners)
			r.Get("/blocks", s.blocks)
			r.Post("/blocks", s.block)
			r.Delete("/blocks/{id}", s.unblock)
			r.Post("/reports", s.report)
			r.Route("/admin", func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/members", s.members)
				r.Patch("/members/{id}", s.memberStatus)
				r.Get("/gatherings", s.adminGatherings)
				r.Post("/gatherings", s.createGathering)
				r.Delete("/gatherings/{id}", s.deleteGathering)
				r.Get("/gatherings/{id}/attendees", s.attendees)
				r.Post("/checkin", s.checkin)
				r.Get("/reports", s.reports)
				r.Post("/announcements", s.createAnnouncement)
				r.Delete("/announcements/{id}", s.removeAnnouncement)
				r.Post("/partners", s.createPartner)
				r.Delete("/partners/{id}", s.removePartner)
			})
		})
	})
	r.NotFound(func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, 404, "not_found", "not found") })
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		s.fail(w, r, 405, "method_not_allowed", "method not allowed")
	})
	return r
}

// health is a liveness probe: the process is up and serving.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	reply(w, 200, map[string]any{"ok": true})
}

// ready is a readiness probe: PostgreSQL must answer. Redis loss only
// degrades rate limiting to the in-process fallback, so it is reported but
// does not fail readiness.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.db.PingContext(ctx); err != nil {
		s.log.Error("readiness: database unavailable", "err", err)
		reply(w, 503, map[string]any{"ok": false, "database": "unavailable"})
		return
	}
	redisState := "not_configured"
	if s.rdb != nil {
		redisState = "ok"
		if err := s.rdb.Ping(ctx).Err(); err != nil {
			redisState = "degraded"
		}
	}
	reply(w, 200, map[string]any{"ok": true, "database": "ok", "redis": redisState})
}
