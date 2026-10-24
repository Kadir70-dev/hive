# Staging deployment readiness

Nothing here has been deployed. DNS, Netlify settings and server provisioning need
the owner's approval. Titan MX/SPF/DMARC records must not be changed.

## Topology

```
Browser ──HTTPS──> Netlify (Next.js 16, src/proxy.ts CSP)
                     │  /api/backend/* server-side proxy (src/lib/backend-proxy.ts)
                     │  adds X-Hive-Client-IP + X-Hive-Proxy-Secret
                     ▼
              Caddy (HTTPS, Let's Encrypt) ── api (Go) ── PostgreSQL 17
                                              │       └─ backup (encrypted, nightly)
                                              └─ Redis (rate limits)
```

Only Caddy publishes ports (80/443). PostgreSQL and Redis are on the private
Compose network.

## What staging needs (owner approval required)

1. A small Hetzner VPS (separate from production) with Docker and a firewall allowing 22 (key-only), 80, 443.
2. DNS: an `A` record such as `api-staging.thehivesociety.ae` → staging VPS. **No other record changes.**
3. Netlify: a branch deploy (e.g. branch `staging`) so the site has a stable origin,
   e.g. `https://staging--<site>.netlify.app`.
4. Resend: test API key + verified sending subdomain (`mailer.thehivesociety.ae`), or
   `MAIL_PROVIDER=log` for a staging without real email.

## Environment variables

API host — `backend/.env.staging` (untracked, mode 600):

| Variable | Staging value |
|---|---|
| `POSTGRES_PASSWORD`, `REDIS_PASSWORD` | new random values (`openssl rand -hex 32`) |
| `HIVE_PROXY_SHARED_SECRET` | random, 32+ chars; same value on Netlify (server-only) |
| `MFA_ENCRYPTION_KEY` | `openssl rand -base64 32`; separate from production |
| `MAIL_PROVIDER` / `RESEND_API_KEY` / `MAIL_FROM` | `resend` + test key + `The Hive Society <staging@mailer.thehivesociety.ae>`, or `log` |
| `BACKUP_AGE_RECIPIENT` | staging age public key |
| `API_DOMAIN` | `api-staging.thehivesociety.ae` |
| `ALLOWED_ORIGINS` | `https://staging--<site>.netlify.app` (exact origin, comma-separate several) |
| `PUBLIC_APP_URL` | same staging site origin (used in email links) |
| `HIVE_DOCKER_SUBNET` / `HIVE_CADDY_IP` | defaults are fine on a fresh VPS |

Netlify (branch-deploy context, server-only):

| Variable | Value |
|---|---|
| `HIVE_API_INTERNAL_URL` | `https://api-staging.thehivesociety.ae` |
| `HIVE_PROXY_SHARED_SECRET` | same as API |
| `NEXT_PUBLIC_SITE_URL` | staging site origin |

## Bring-up (on the staging VPS, after approval)

```bash
git clone https://github.com/Kadir70-dev/hive && cd hive && git checkout <reviewed commit>
docker compose --env-file backend/.env.staging -p hive-staging up -d --build
docker compose -p hive-staging ps            # db, redis healthy; migrate exited 0; api, backup, caddy up
curl -fsS https://api-staging.thehivesociety.ae/ready
```

Migrations run automatically (`migrate` service) and the API refuses to start if any are pending.

## Verification after bring-up

- `scripts/staging/proxy-e2e.sh` proves locally (and in CI) that the proxy keeps cookies,
  client-IP trust and security headers. On staging repeat the key checks manually:
  sign up, receive the email, confirm, log in, keep a place, cancel; admin CLI promote +
  TOTP enrolment; `curl -I` the site and API for CSP/HSTS headers.
- Confirm Netlify applies `next.config.ts` headers to `/hive.html` (static file) — not yet verified.
- Run a backup (`docker compose -p hive-staging exec backup backup.sh`) and a verified restore on an operator machine.

## Monitoring (minimum)

- External uptime check on `https://<api>/ready` (expects 200; returns 503 if PostgreSQL is down) and on the site.
- `docker compose ps`: health checks for db, redis, api (`/ready`) and backup (last success < 26 h).
- Logs: JSON on stdout (API without IPs/emails; Caddy access log without query strings/cookies), rotated at 5×10 MB per service.
- Alert on: `/ready` failing, backup unhealthy, `email abandoned` log lines, disk > 80%.
- Not implemented: metrics/alerting stack, log shipping. Choose a provider before production.
