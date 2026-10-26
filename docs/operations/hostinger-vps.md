# Hostinger VPS — single-server deployment readiness

**Development preparation only. Not yet deployed, tested on Hostinger, or approved for production.**
The maintained code is in `Kadir70-dev/hive`. The separately deployed static Vercel
`the-hive-society-ui` is NOT the authenticated application and must not be
mistaken for a working member backend.

## Topology

```
Browser -- HTTPS --> Caddy :443
                     |
                Next.js :3000 (private Docker network)
                     | same-origin /api/backend/*
                     +--> Go API :8080 (private)
                                +--> PostgreSQL 17 (private)
                                +--> Redis 7 (private)
                     +--> encrypted Postgres backup service
```

Only Caddy publishes 80/443. No public Postgres, Redis, Go API, or Next.js
port. The same-origin Next.js proxy retains secure HttpOnly session cookies.
Incoming `X-Hive-Verified-Client-IP` is overwritten by Caddy; Next trusts
it only in the explicitly configured `HIVE_SITE_EDGE=caddy` mode.
Do not expose the web container directly to the internet or put an
untrusted proxy in front of Caddy without revisiting the IP trust model.

## Before provisioning (owner/operator decisions)

- Hostinger VPS with Docker/Compose; firewall permits SSH via keys and
  inbound TCP 80/443 only. Do not expose 3000/8080/5432/6379.
- Choose an **isolated staging hostname** (e.g. staging.thehivesociety.ae),
  owned/approved by the domain owner. No production DNS cutover.
- Owner-approved mail sending domain in Resend (keep the existing root
  domain's Titan MX/SPF/DMARC untouched). Use `MAIL_PROVIDER=log` only
  during isolated development or staging.
- Store private age backup identity **offline**, create a valid age PUBLIC
  recipient for `BACKUP_AGE_RECIPIENT`, and verify a restore.
- Decide profile photo R2 credentials, record retention, legal policies,
  legacy member migration, and two verified 2FA admin users.
- **No real secrets, access tokens or VPS root credentials in Git or chat.**

## Prepare configuration (on operator machine/VPS, after approval)

```sh
git clone https://github.com/Kadir70-dev/hive.git
cd hive
# Review the commit/PR before deploying (no automatic pull of main).
cp backend/.env.example backend/.env.hostinger
chmod 600 backend/.env.hostinger
```

Set these in the private `backend/.env.hostinger` file:

| Variable | Rule |
|---|---|
| `SITE_DOMAIN` | approved staging hostname, hostname only |
| `PUBLIC_APP_URL` | `https://${SITE_DOMAIN}` |
| `ALLOWED_ORIGINS` | exact matching `https://${SITE_DOMAIN}` |
| `API_DOMAIN` | unused in Hostinger single-server mode; use a non-production staging name for legacy preflight compatibility |
| `POSTGRES_PASSWORD`, `REDIS_PASSWORD` | unique random 32+ characters, with URL-safe characters |
| `HIVE_PROXY_SHARED_SECRET` | separate random 32+ character value, server-side only |
| `MFA_ENCRYPTION_KEY` | base64-encoded 32 random bytes, backed up offline |
| `BACKUP_AGE_RECIPIENT` | valid age public recipient; private key stays offline |
| `MAIL_PROVIDER` | `log` for staging, `resend` with verified sender and key for production |
| `R2_*` | private bucket credentials for production photo uploads |

The compose overlay sets `HIVE_API_INTERNAL_URL=http://api:8080`,
`HIVE_SITE_EDGE=caddy`, and `NEXT_PUBLIC_SITE_URL` internally.
Never configure a public Go API URL or store the proxy secret in NEXT_PUBLIC_*.

Validate locally without showing secrets:

```sh
node scripts/staging/config-preflight.mjs --mode staging --api-env backend/.env.hostinger
docker compose --env-file backend/.env.hostinger \
  -f docker-compose.yml -f docker-compose.hostinger.yml config -q
```

The first command validates API settings but not a live server, DNS or delivery.
The second checks the *merged* Compose configuration and required variables.
Only after staging approval, launch with:

```sh
docker compose --env-file backend/.env.hostinger \
  -f docker-compose.yml -f docker-compose.hostinger.yml up -d --build
docker compose --env-file backend/.env.hostinger \
  -f docker-compose.yml -f docker-compose.hostinger.yml ps
```

The migrations must complete before the API starts; the web then waits for
the API readiness check. **Do not point the production domain here yet.**

## Staging acceptance gates

1. `https://staging.example/health` is a webpage/404, not the Go readiness
   endpoint; check Go `/ready` from inside the private Docker network.
2. New test member: signup, verify email, login, refresh session, pending state.
3. Admin: allowlisted account confirmation, CLI promotion, TOTP enrolment,
   approve/decline member, create/edit gathering, roster/check-in.
4. Members: keep place, cancel, FIFO waitlist promotion, waitlist emails,
   delete account; confirm members never see other members' identities.
5. Live Resend: test delivery, retry, bounce handling and from-domain DNS.
6. Profile image: R2 upload, private read, replace and delete, unauthorized denial.
7. Backups: encrypted backup, restore into a disposable database, no lost data;
   configure off-site backup and health alerts.
8. Accessibility, phone/browser QA, OWASP review, monitoring, TLS, cookies/CSP.
9. Legal owner approval for Terms, Privacy, retention and existing-member migration.

Before any DNS switch, run the read-only HTTP smoke test on the approved
staging hostname:

```sh
BASE_URL=https://staging.thehivesociety.ae bash scripts/hostinger/smoke.sh
```

It checks the homepage, auth page, public manifest, private API boundary,
anonymous session rejection and key security headers. It does not submit
real member data or prove email, admin 2FA, backup or waitlist behavior.

All these remain **unverified in a real Hostinger environment**.
Do not cut over production DNS until the owner accepts the staging results.

### Safe rollback

Keep the previous site and database intact. Revert app image/release
and restore database only from a pre-deployment encrypted backup after
review. Never use `docker compose down -v` on a live environment.
