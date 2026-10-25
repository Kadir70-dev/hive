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

## Offline configuration preflight (no network or secret disclosure)

When the owner provisions an isolated staging VPS and Netlify branch context,
prepare untracked API and frontend env files on the operator machine. Check that
API and Netlify share the same proxy secret and expected HTTPS origins:

```bash
node scripts/staging/config-preflight.mjs --mode staging \
  --api-env backend/.env.staging --site-env .env.staging
```

For an eventual production configuration, use `--mode production`. The checker
fails on unset/placeholder secrets, mismatched proxy configuration, unsafe HTTP
origins, staging settings pointed at the live domain, partial private R2 settings
and production mail configured as `log`. It does **not** source/evaluate the
env files, display secret values, connect to services or mutate settings.

**This is an offline format check, not a deployment or proof of deliverability.**
On Netlify, environment variables are normally stored in the UI, not an env file:
a matching operator-created temporary file is only for cross-checking and should
be destroyed immediately after validation; never commit it or paste credentials
into the chat. CI runs the preflight unit tests using deliberately fake secrets.

## Tracked signup campaign staging test

Using disposable test accounts and the approved staging domain:

1. Visit `/?utm_source=staging_test&utm_medium=qa&utm_campaign=prelaunch`.
2. Submit a *new* signup; confirm `POST /v1/signup` returns a generic `202` and sends a confirmation email through the approved sender.
3. Before confirming the email, the MFA-gated Admin → Signups page should show one attributed signup and **zero** verified conversions.
4. Confirm using the one-time email link, log in and verify it now shows exactly one verified conversion. Replaying the token must not increase the count.
5. Delete the disposable member account; its campaign attribution must disappear.
6. Confirm a normal member cannot access `/v1/admin/campaigns` and no member email/name/ID appears in admin aggregate reporting.

GitHub CI exercises this sequence against a disposable PostgreSQL database;
real sender delivery and the real Netlify→Go environment remain separate checks.

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
