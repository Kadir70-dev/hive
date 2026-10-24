// Command api serves The Hive Society JSON API.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"

	"thehivesociety/api/internal/clientip"
	"thehivesociety/api/internal/httpapi"
	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/maintenance"
	"thehivesociety/api/internal/mfa"
	"thehivesociety/api/internal/migrate"
	"thehivesociety/api/internal/password"
	"thehivesociety/api/internal/ratelimit"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s: invalid duration %q", key, v)
	}
	return d, nil
}

func intEnv(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("%s: invalid positive integer %q", key, v)
	}
	return n, nil
}

func run(log *slog.Logger) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL required")
	}
	var err error
	readHeader, read, write, idle, reqTimeout, shutdown := 5*time.Second, 15*time.Second, 20*time.Second, 60*time.Second, 10*time.Second, 20*time.Second
	for k, p := range map[string]*time.Duration{"HTTP_READ_HEADER_TIMEOUT": &readHeader, "HTTP_READ_TIMEOUT": &read,
		"HTTP_WRITE_TIMEOUT": &write, "HTTP_IDLE_TIMEOUT": &idle, "REQUEST_TIMEOUT": &reqTimeout, "SHUTDOWN_TIMEOUT": &shutdown} {
		if *p, err = durationEnv(k, *p); err != nil {
			return err
		}
	}
	if reqTimeout >= write {
		return errors.New("REQUEST_TIMEOUT must be shorter than HTTP_WRITE_TIMEOUT")
	}
	hashConcurrency, err := intEnv("PASSWORD_HASH_CONCURRENCY", 4)
	if err != nil {
		return err
	}
	maxBody, err := intEnv("MAX_BODY_BYTES", 64<<10)
	if err != nil {
		return err
	}
	dbConns, err := intEnv("DB_MAX_OPEN_CONNS", 12)
	if err != nil {
		return err
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(dbConns)
	db.SetMaxIdleConns(dbConns / 2)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(10 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	// Refuse to serve against an un-migrated schema; run `migrate up` first.
	if pending, err := migrate.Pending(ctx, db); err != nil {
		return fmt.Errorf("checking migrations: %w", err)
	} else if pending {
		return errors.New("database has pending migrations; run `migrate up` before starting the API")
	}

	var rdb redis.UniversalClient
	if url := os.Getenv("REDIS_URL"); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return errors.New("REDIS_URL is invalid")
		}
		opt.DialTimeout, opt.ReadTimeout, opt.WriteTimeout = time.Second, 500*time.Millisecond, 500*time.Millisecond
		rdb = redis.NewClient(opt)
		defer rdb.Close()
		if err := rdb.Ping(ctx).Err(); err != nil {
			log.Warn("Redis unreachable at startup; rate limits use the in-process fallback until it recovers")
		}
	} else {
		log.Warn("REDIS_URL not set; rate limits are per-process only")
	}

	proxySecret := os.Getenv("HIVE_PROXY_SHARED_SECRET")
	if proxySecret != "" && len(proxySecret) < 32 {
		return errors.New("HIVE_PROXY_SHARED_SECRET must be at least 32 characters")
	}
	if proxySecret == "" {
		log.Warn("HIVE_PROXY_SHARED_SECRET not set; X-Hive-Client-IP is ignored and Netlify traffic shares one rate-limit bucket")
	}
	ips, err := clientip.New(os.Getenv("TRUSTED_PROXY_CIDRS"), proxySecret)
	if err != nil {
		return err
	}

	var origins []string
	for _, o := range strings.Split(env("ALLOWED_ORIGINS", os.Getenv("PUBLIC_ORIGIN")), ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}

	sender, err := mailSender(log)
	if err != nil {
		return err
	}
	var mfaCipher *mfa.Cipher
	if key := os.Getenv("MFA_ENCRYPTION_KEY"); key != "" {
		if mfaCipher, err = mfa.NewCipher(key); err != nil {
			return err
		}
	} else {
		log.Warn("MFA_ENCRYPTION_KEY not set; admins cannot enrol two-factor authentication, so admin routes stay locked")
	}
	appURL := env("PUBLIC_APP_URL", "https://thehivesociety.ae")

	secureCookies := os.Getenv("COOKIE_SECURE") != "false"
	if secureCookies && len(origins) == 0 {
		return errors.New("ALLOWED_ORIGINS (or PUBLIC_ORIGIN) is required when COOKIE_SECURE is on; it is the CSRF origin allow-list")
	}

	handler := httpapi.New(httpapi.Config{
		DB: db, Redis: rdb, Hasher: password.NewHasher(hashConcurrency), Limiter: ratelimit.New(rdb, log),
		IPs: ips, Log: log, SecureCookies: secureCookies, AllowedOrigins: origins,
		RequestTimeout: reqTimeout, MaxBodyBytes: int64(maxBody), Limits: httpapi.DefaultLimits(),
		AppURL: appURL, MFACipher: mfaCipher,
	})
	srv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           handler,
		ReadHeaderTimeout: readHeader,
		ReadTimeout:       read,
		WriteTimeout:      write,
		IdleTimeout:       idle,
		MaxHeaderBytes:    32 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Background work stops with the server.
	bgCtx, stopBG := context.WithCancel(context.Background())
	defer stopBG()
	go (&mail.Worker{DB: db, Sender: sender, Log: log}).Run(bgCtx, 5*time.Second)
	go maintenance.Run(bgCtx, db, log, time.Hour)

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-sigCtx.Done():
		log.Info("shutdown signal received; draining connections")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdown)
		defer cancel()
		stopBG()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown: %w", err)
		}
	}
	log.Info("api stopped")
	return nil
}

// mailSender selects the transactional email provider. MAIL_PROVIDER must be
// explicit so a production deploy cannot silently drop email.
func mailSender(log *slog.Logger) (mail.Sender, error) {
	switch p := os.Getenv("MAIL_PROVIDER"); p {
	case "resend":
		return mail.NewResend(os.Getenv("RESEND_API_KEY"), os.Getenv("MAIL_FROM"), os.Getenv("MAIL_REPLY_TO"))
	case "log":
		log.Warn("MAIL_PROVIDER=log: emails are NOT delivered (development only)")
		return &mail.Log{Logger: log, LogBodies: os.Getenv("MAIL_LOG_BODIES") == "true"}, nil
	default:
		return nil, fmt.Errorf("MAIL_PROVIDER must be \"resend\" or \"log\" (got %q)", p)
	}
}
