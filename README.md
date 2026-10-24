# The Hive Society

Next.js 16 frontend + Go/Chi API + self-hosted PostgreSQL and Redis. Supabase is not required or used by the active application.

- `src/`: Next.js frontend and the server-side same-origin proxy to the Go API (`src/lib/backend-proxy.ts`)
- `backend/`: Go API (`cmd/api`), migration tool (`cmd/migrate`), embedded SQL migrations (`migrations/`)
- `docker-compose.yml`: Hetzner VPS services (PostgreSQL, Redis, one-shot `migrate`, API, Caddy)
- `.github/workflows/ci.yml`: backend, frontend and infrastructure checks
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

## Health checks

- `GET /health` — liveness (process is serving).
- `GET /ready` — readiness (PostgreSQL reachable). Used by the Docker healthcheck.
