# The Hive Society

Next.js 16 frontend + Go/Chi API + self-hosted PostgreSQL and Redis. Member authentication, sessions and data access are owned by the Go application; no third-party backend or hosted authentication service is required.

- `src/`: Next.js frontend and the server-side same-origin proxy to the Go API (`src/lib/backend-proxy.ts`)
- `backend/`: Go API (`cmd/api`), migration tool (`cmd/migrate`), embedded SQL migrations (`migrations/`)
- `docker-compose.yml`: Hetzner VPS services (PostgreSQL, Redis, one-shot `migrate`, API, Caddy)
- `ops/backup/`: encrypted backup, restore and verify tooling (see `docs/operations/backup-and-restore.md`)
- `docs/operations/staging.md`: staging deployment guide, environment variables and monitoring
- `scripts/staging/proxy-e2e.sh`: end-to-end check of the Next.js proxy in front of the Go API
- `.github/workflows/ci.yml`: backend, frontend, infrastructure and proxy end-to-end checks
- `IMPLEMENTATION_STATUS.md`: honest feature status, limits and launch checklist

This is a **development checkpoint**, not a validated production-ready build.

## Development checks

```bash
# Frontend (Node 22; the lockfile was generated with npm 11 — `npm ci` works with npm 10+)
npm ci && npm run lint && npm run typecheck && npm test && npm run build

# Backend (Go 1.24). Integration tests need a disposable PostgreSQL server:
docker run -d --name hive-test-pg -e POSTGRES_PASSWORD=test -p 127.0.0.1:55439:5432 postgres:17-alpine
cd backend
TEST_DATABASE_URL='postgres://postgres:test@127.0.0.1:55439/postgres?sslmode=disable' go test -race ./...
```

Without `TEST_DATABASE_URL` the PostgreSQL integration tests are skipped, not failed.

## Database migrations

Migrations are versioned with [goose](https://github.com/pressly/goose) and embedded in the backend image.

```bash
docker compose --env-file backend/.env run --rm migrate             # apply pending migrations
docker compose --env-file backend/.env run --rm --entrypoint migrate migrate status
```

- `docker compose up` runs `migrate` first; the API starts only if it exits 0, and the API itself refuses to start while migrations are pending.
- Running `migrate up` again is a no-op. A Postgres advisory lock serialises concurrent runs.
- Databases created by hand from the old `001_init.sql` upgrade in place (the baseline is idempotent).
- **Rollback:** there is no `down`/reset command. Roll back by restoring a backup taken before the deploy.

## Rate limiting and the Netlify → API trust boundary

The browser talks to Next.js on Netlify, which forwards to `https://api.thehivesociety.ae` (Caddy → Go).

- The Go API takes the client IP from `X-Forwarded-For` **only** when the TCP peer is in `TRUSTED_PROXY_CIDRS` (Caddy's fixed Docker address), using the right-most untrusted hop.
- Netlify Functions have no fixed egress IPs, so the Next.js proxy authenticates with `HIVE_PROXY_SHARED_SECRET` and sends the browser IP (from Netlify's `x-nf-client-connection-ip`) as `X-Hive-Client-IP`. The API ignores that header unless the secret matches. Set the same 32+ character secret, server-side only, on Netlify and in `backend/.env`.
- Limits: login 30/10 min per IP, signup 10/hour per IP, 5 failed logins/15 min per account (applied to unknown emails too), 600 API calls/min per IP, and at most `PASSWORD_HASH_CONCURRENCY` (default 4) concurrent Argon2id operations; extra requests get `503` + `Retry-After` instead of queuing.
- Rate limits live in Redis. If Redis is down the API keeps enforcing the same limits per process and `/ready` reports `"redis":"degraded"`.

## Accounts, email and admin access

- **Signup** returns the same `202` whether or not the address already has an account; new accounts get a confirmation link, existing ones an "account exists" notice. Login is refused until the email is confirmed.
- **Links** (verification 24 h, reset 30 min) are single-use, stored only as SHA-256 hashes, and carry the token in the URL fragment (`#token=`) so it never reaches server or CDN logs. A reset signs the account out everywhere.
- **Email delivery** goes through a Postgres outbox written in the same transaction as the token, delivered by a worker inside the API (retries with backoff, `SKIP LOCKED` for multiple instances, bodies erased after delivery). Provider: `MAIL_PROVIDER=resend` (`RESEND_API_KEY`, `MAIL_FROM`) or `log` for development.
- **Resend sending domain** must be verified on a dedicated subdomain (e.g. `mailer.thehivesociety.ae`: DKIM TXT + SPF/MX on that subdomain only). This is a DNS change that needs owner approval; Titan MX/SPF/DMARC on the root domain are not touched.
- **Admin bootstrap** happens only from a shell on the server, never over HTTP:
  ```bash
  docker compose --env-file backend/.env run --rm --entrypoint admin api promote --email hello@thehivesociety.ae
  docker compose --env-file backend/.env run --rm --entrypoint admin api list
  docker compose --env-file backend/.env run --rm --entrypoint admin api reset-mfa --email ...   # lost phone
  ```
  `promote` requires the address to be in `HIVE_ADMIN_EMAILS` **and** already verified.
- **Admin 2FA**: every admin route requires a verified email, enrolled TOTP (RFC 6238, secrets AES-GCM encrypted with `MFA_ENCRYPTION_KEY`) and a code on the current session within 12 hours. Ten single-use recovery codes are issued at enrolment. Codes cannot be replayed; 5 failures per 15 minutes lock MFA attempts.
- **Notifications** (all through the outbox): admins are alerted when a member confirms their email; members get approval, booking/waitlist confirmation, cancellation, waitlist promotion and gathering changed/cancelled emails. `admin test-email --email you@…` queues a delivery test.
- **Gatherings**: admins can edit gatherings; raising capacity promotes the waitlist in order, lowering it below confirmed places is refused, and bookings close once a gathering starts (Asia/Dubai time).
- **Cleanup** runs hourly: expired sessions, week-old tokens, and stale or month-old outbox rows.

## Browser security headers

`src/proxy.ts` gives every page a per-request nonce CSP (`script-src 'nonce-…' 'strict-dynamic'`), so pages render dynamically. The static homepage `public/hive.html` gets a hash-pinned CSP computed at build time, so editing its inline script needs a rebuild (automatic on deploy). HSTS (production only, no `includeSubDomains`), `nosniff`, `Referrer-Policy`, `Permissions-Policy`, COOP and frame protections apply everywhere (`next.config.ts`). Netlify's handling of `next.config.ts` headers for static files must be confirmed on a deploy preview.

## Health checks

- `GET /health` — liveness (process is serving).
- `GET /ready` — readiness (PostgreSQL reachable). Used by the Docker healthcheck.

## Batch 5 (engineering; staging credentials still required)

- Private profile photos via S3-compatible R2, 5 MiB content-validated input,
  re-encoded JPEGs and deletion retries. See
  [R2 and monitoring](docs/operations/r2-and-monitoring.md).
- Confidential, MFA-gated safety conflict resolution tools.
- Offline legacy member CSV import and invitation preparation: see
  [member migration](docs/operations/member-migration.md).
- Audit retention is disabled until the legal period is approved.
- CI runs an authenticated, disposable SeaweedFS server for S3 integration.
- Do not deploy this commit as production-ready without real R2/Resend tests,
  off-site backups, staging verification and approved legal texts.
