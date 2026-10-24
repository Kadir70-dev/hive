// Package httpapi implements The Hive Society JSON API.
package httpapi

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"

	"thehivesociety/api/internal/clientip"
	"thehivesociety/api/internal/mfa"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
	"thehivesociety/api/internal/storage"
)

// Limits groups the rate-limit rules. Zero values fall back to DefaultLimits.
type Limits struct {
	LoginPerIP       ratelimit.Rule
	SignupPerIP      ratelimit.Rule
	LoginPerAccount  ratelimit.Rule // counts failed logins per email
	APIPerIP         ratelimit.Rule
	TokenPerIP       ratelimit.Rule // verify/reset/forgot/resend calls per IP
	EmailPerAccount  ratelimit.Rule // verification or reset emails per address
	ExistsNotice     ratelimit.Rule // "account exists" notices per address
	MFAPerAccount    ratelimit.Rule // failed TOTP/recovery attempts per admin
	ReportsPerMember ratelimit.Rule // safety reports per member
	PhotosPerMember  ratelimit.Rule // profile photo uploads per member
	HashWait         time.Duration  // how long a request may wait for a hashing slot
}

// DefaultLimits are the production limits.
func DefaultLimits() Limits {
	return Limits{
		LoginPerIP:       ratelimit.Rule{Name: "login-ip", Limit: 30, Window: 10 * time.Minute},
		SignupPerIP:      ratelimit.Rule{Name: "signup-ip", Limit: 10, Window: time.Hour},
		LoginPerAccount:  ratelimit.Rule{Name: "login-acct", Limit: 5, Window: 15 * time.Minute},
		APIPerIP:         ratelimit.Rule{Name: "api-ip", Limit: 600, Window: time.Minute},
		TokenPerIP:       ratelimit.Rule{Name: "token-ip", Limit: 20, Window: 15 * time.Minute},
		EmailPerAccount:  ratelimit.Rule{Name: "email-acct", Limit: 3, Window: time.Hour},
		ExistsNotice:     ratelimit.Rule{Name: "exists-acct", Limit: 1, Window: time.Hour},
		MFAPerAccount:    ratelimit.Rule{Name: "mfa-acct", Limit: 5, Window: 15 * time.Minute},
		ReportsPerMember: ratelimit.Rule{Name: "report-member", Limit: 10, Window: 24 * time.Hour},
		PhotosPerMember:  ratelimit.Rule{Name: "photo-member", Limit: 10, Window: time.Hour},
		HashWait:         3 * time.Second,
	}
}

// withDefaults fills any unset rule field (limit, name, window) from DefaultLimits.
func withDefaults(l Limits) Limits {
	d := DefaultLimits()
	for _, p := range []struct{ got, def *ratelimit.Rule }{
		{&l.LoginPerIP, &d.LoginPerIP}, {&l.SignupPerIP, &d.SignupPerIP}, {&l.LoginPerAccount, &d.LoginPerAccount},
		{&l.APIPerIP, &d.APIPerIP}, {&l.TokenPerIP, &d.TokenPerIP}, {&l.EmailPerAccount, &d.EmailPerAccount},
		{&l.ExistsNotice, &d.ExistsNotice}, {&l.MFAPerAccount, &d.MFAPerAccount}, {&l.ReportsPerMember, &d.ReportsPerMember},
		{&l.PhotosPerMember, &d.PhotosPerMember},
	} {
		if p.got.Limit <= 0 {
			p.got.Limit = p.def.Limit
		}
		if p.got.Name == "" {
			p.got.Name = p.def.Name
		}
		if p.got.Window <= 0 {
			p.got.Window = p.def.Window
		}
	}
	if l.HashWait <= 0 {
		l.HashWait = d.HashWait
	}
	return l
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
	AppURL         string        // public site origin used in email links
	MFACipher      *mfa.Cipher   // nil disables admin MFA enrolment (admin routes then stay locked)
	VerifyTTL      time.Duration // email verification link lifetime (default 24h)
	ResetTTL       time.Duration // password reset link lifetime (default 30m)
	MFAWindow      time.Duration // how long a TOTP step-up lasts on a session (default 12h)
	Store          storage.Store // private photo storage (R2); nil disables photos
	ImageSlots     int           // concurrent image decodes (default 2)
	MetricsToken   string        // enables GET /internal/metrics with this bearer token
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

	appURL    string
	mfaCipher *mfa.Cipher
	verifyTTL time.Duration
	resetTTL  time.Duration
	mfaWindow string // Postgres interval literal

	store        storage.Store
	imageSlots   chan struct{}
	metricsToken string
}

// New builds the HTTP handler.
func New(c Config) http.Handler {
	s := &Server{
		db: c.DB, rdb: c.Redis, hasher: c.Hasher, limiter: c.Limiter, ips: c.IPs, log: c.Log,
		secure: c.SecureCookies, origins: map[string]bool{}, timeout: c.RequestTimeout,
		maxBody: c.MaxBodyBytes, limits: c.Limits, appURL: c.AppURL, mfaCipher: c.MFACipher,
		verifyTTL: c.VerifyTTL, resetTTL: c.ResetTTL,
	}
	if s.appURL == "" {
		s.appURL = "https://thehivesociety.ae"
	}
	if s.verifyTTL <= 0 {
		s.verifyTTL = 24 * time.Hour
	}
	if s.resetTTL <= 0 {
		s.resetTTL = 30 * time.Minute
	}
	if c.MFAWindow <= 0 {
		c.MFAWindow = 12 * time.Hour
	}
	s.mfaWindow = fmt.Sprintf("%d seconds", int(c.MFAWindow.Seconds()))
	s.store = c.Store
	s.metricsToken = c.MetricsToken
	if c.ImageSlots <= 0 {
		c.ImageSlots = 2
	}
	s.imageSlots = make(chan struct{}, c.ImageSlots)
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
	s.limits = withDefaults(s.limits)
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
	r.Get("/internal/metrics", s.metrics)
	r.Route("/v1", func(r chi.Router) {
		r.Use(s.checkOrigin, s.apiRateLimit)
		r.Post("/signup", s.signup)
		r.Post("/login", s.login)
		r.Post("/logout", s.logout)
		r.Post("/founding-signups", s.foundingSignup)
		r.Post("/verify-email", s.verifyEmail)
		r.Post("/verify-email/resend", s.resendVerification)
		r.Post("/password/forgot", s.forgotPassword)
		r.Post("/password/reset", s.resetPassword)
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
			r.Get("/me/gatherings", s.myGatherings)
			r.Get("/me/photo", s.myPhoto)
			r.Post("/me/photo", s.uploadPhoto)
			r.Delete("/me/photo", s.deletePhoto)
			r.Post("/reports", s.createReport)
			r.Get("/reports/mine", s.myReports)
			r.Route("/mfa", func(r chi.Router) {
				r.Use(s.requireAdminAccount)
				r.Post("/totp/setup", s.mfaSetup)
				r.Post("/totp/enable", s.mfaEnable)
				r.Post("/verify", s.mfaVerify)
			})
			r.Route("/admin", func(r chi.Router) {
				r.Use(s.requireAdmin)
				r.Get("/members", s.members)
				r.Patch("/members/{id}", s.memberStatus)
				r.Get("/members/{id}/photo", s.memberPhoto)
				r.Get("/gatherings", s.adminGatherings)
				r.Post("/gatherings", s.createGathering)
				r.Put("/gatherings/{id}", s.updateGathering)
				r.Delete("/gatherings/{id}", s.deleteGathering)
				r.Get("/gatherings/{id}/attendees", s.attendees)
				r.Post("/checkin", s.checkin)
				r.Get("/reports", s.adminReports)
				r.Patch("/reports/{id}", s.updateReport)
				r.Post("/reports/{id}/block", s.applyBlock)
				r.Delete("/reports/{id}/block", s.removeBlock)
				r.Get("/safety-conflicts", s.safetyConflicts)
				r.Post("/safety-conflicts/resolve", s.resolveConflict)
				r.Get("/announcements", s.adminAnnouncements)
				r.Post("/announcements", s.createAnnouncement)
				r.Put("/announcements/{id}", s.updateAnnouncement)
				r.Delete("/announcements/{id}", s.removeAnnouncement)
				r.Post("/announcements/{id}/email", s.emailAnnouncement)
				r.Get("/partners", s.adminPartners)
				r.Post("/partners", s.createPartner)
				r.Put("/partners/{id}", s.updatePartner)
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
	storageState := "not_configured"
	if s.store != nil {
		storageState = "ok"
		if err := s.store.Ping(ctx); err != nil {
			storageState = "degraded"
		}
	}
	// Redis and photo storage only degrade features, so they never fail readiness.
	reply(w, 200, map[string]any{"ok": true, "database": "ok", "redis": redisState, "storage": storageState})
}
