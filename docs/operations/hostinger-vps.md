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
| `HIVE_ENVIRONMENT` | `staging` until production cutover; gates noindex/robots |
| `HIVE_IMAGE_TAG` | exact 12-40 character reviewed git SHA, e.g. `$(git rev-parse HEAD)`; retain prior images for rollback |
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
`HIVE_SITE_EDGE=caddy`, and `NEXT_PUBLIC_SITE_URL` from `PUBLIC_APP_URL` at build time. Staging builds get robots.txt `Disallow: /` plus Caddy's `X-Robots-Tag: noindex, nofollow`. Production must explicitly set `HIVE_ENVIRONMENT=production` in the reviewed release configuration.
Never configure a public Go API URL or store the proxy secret in NEXT_PUBLIC_*.

Validate locally without showing secrets:

```sh
node scripts/staging/config-preflight.mjs --mode staging --api-env backend/.env.hostinger
docker compose --env-file backend/.env.hostinger \
  -f docker-compose.yml -f docker-compose.hostinger.yml config -q
```

The first command validates API settings but not a live server, DNS or delivery.
The second checks the *merged* Compose configuration and required variables.
Only after staging approval, launch with the reviewed SHA tag:

```sh
export HIVE_IMAGE_TAG="$(git rev-parse HEAD)"
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

### Tagged image releases and safe rollback

Hostinger images `hive-api`, `hive-web` and `hive-backup` are tagged
with `HIVE_IMAGE_TAG` (the reviewed git SHA). Keep the previous release's
images on the VPS; don't prune them during deployment. Record the previous
SHA **before** updating code and keep the previous .env/release notes.

For a **schema-compatible app-only** rollback, after confirming the prior
images exist and the *forward-only* migrations are backward compatible:

```sh
ENV_FILE=backend/.env.hostinger \
PREVIOUS_TAG=<previous-12-to-40-char-git-sha> \
CONFIRM_SCHEMA_COMPATIBLE=YES \
bash scripts/hostinger/rollback.sh
```

This selects earlier API/Next.js images without rebuilding, dropping
volumes or attempting migration downgrade. After rollback run
`scripts/hostinger/smoke.sh` and test member sessions/bookings.
If schema compatibility is uncertain, **do not run the script**: enter
maintenance, restore only from a separately verified encrypted backup
after operator review. No live Hostinger rollback has been performed;
CI exercises the image tag switching using the same schema.

Staging Caddy request limits: a 1 MB cap for ordinary requests and a 6 MB
cap only for `POST /api/backend/me/photo`. The 5 MB unauthenticated
probe verifies the request is **not rejected by Caddy size limits**; it
does not prove a successful photo upload. Authenticated R2 upload still
requires staging acceptance.

If a Cloudflare proxy is added later, Caddy will see the proxy address
rather than the real visitor's IP. Do not enable proxying until a
trusted-proxy/header policy is reviewed and tested.

Never use `docker compose down -v` on a live environment.

## Production cutover (thehivesociety.ae → this VPS)

The site currently on `thehivesociety.ae`/`www` is the old Lovable deployment
(`A 185.158.133.1`). This repository has no Lovable or Supabase runtime
dependency; existing members are brought over only via offline exports
(`docs/operations/member-migration.md`). **Lovable and its DNS stay untouched
until member migration and launch tests pass.**

### Layout: staging and production on one VPS behind one Caddy

```
Internet :80/:443 -> hive-edge Caddy (deploy/edge)
   ├─ hive-edge-staging (internal) -> hive  stack: web -> api -> db/redis (10.250.70.0/24)
   └─ hive-edge-prod    (internal) -> hive-prod stack: web -> api -> db/redis (10.250.71.0/24)
```

- Each environment is its own Compose project with its own database, Redis,
  backups, secrets and env file; only its Next.js container joins its own
  internal edge network. Caddy is the only container on both edge networks,
  so staging cannot reach production (verified: by name and by IP).
- Security headers, body limits, client-IP overwrite and log redaction come
  from one shared snippet (`caddy/hive-site.caddy`) for every layout.
- Staging is always `noindex` at the edge; production is indexable.
- Production's web image is `hive-web-production:<sha>` (its site URL is
  baked in at build time), so it never overwrites the staging image.
- Sizing: the VPS has ~8 GB RAM / 96 GB disk; one stack uses about 1 GB.

Per-stack command (add `-f docker-compose.edge.yml`):

```sh
docker compose --env-file backend/.env.<env> -f docker-compose.yml \
  -f docker-compose.hostinger.yml -f docker-compose.edge.yml up -d
```

| Setting | staging (`/opt/hive`, `backend/.env.hostinger`) | production (`/opt/hive-prod`, `backend/.env.production`) |
|---|---|---|
| `COMPOSE_PROJECT_NAME` | unset (`hive`, keeps existing volumes) | `hive-prod` |
| `HIVE_EDGE_ALIAS` / `HIVE_EDGE_NETWORK` | `hive-staging-web` / `hive-edge-staging` | `hive-prod-web` / `hive-edge-prod` |
| `HIVE_DOCKER_SUBNET` / `HIVE_CADDY_IP` | defaults | `10.250.71.0/24` / `10.250.71.10` |
| `HIVE_WEB_IMAGE_SUFFIX` | unset | `-production` |
| `HIVE_ENVIRONMENT` | `staging` | `production` |
| `SITE_DOMAIN` / `PUBLIC_APP_URL` / `ALLOWED_ORIGINS` | `staging.thehivesociety.ae` | `thehivesociety.ae` |
| `MAIL_PROVIDER` / `MAIL_FROM` | `resend`, `staging@mailer.thehivesociety.ae` | `resend`, `hello@mailer.thehivesociety.ae` |
| `RESEND_API_KEY` | own key | own key |
| `R2_BUCKET` | `hive-photos-staging` | `hive-photos-prod` |
| `BACKUP_RCLONE_REMOTE` | `offsite:hive-backups/staging` | `offsite:hive-backups/production` |
| `MFA_ENCRYPTION_KEY`, DB/Redis/proxy secrets | own values | **new** values, never copied |

Edge env (`deploy/edge/.env`, untracked): `ACME_EMAIL`,
`STAGING_DOMAIN=staging.thehivesociety.ae`, and only at cutover
`PROD_DOMAIN=thehivesociety.ae`, `PROD_REDIRECT_FROM=www.thehivesociety.ae`.

`node scripts/staging/config-preflight.mjs --mode <staging|production> --api-env <file>`
enforces these per-environment values.

### External services to provision (owner accounts)

**Resend (email):** add the domain `mailer.thehivesociety.ae` in Resend and
create the records it shows in OnlyDomains. They are all on the `mailer`
subdomain (typically MX + SPF TXT on `send.mailer`, DKIM TXT
`resend._domainkey.mailer`), so the root domain's Titan MX/SPF/DMARC stay
untouched. Create one *sending-only* API key per environment, restricted to
that domain. Check with
`docker compose … run --rm --entrypoint admin api test-email --email <owner address>`.

**Cloudflare R2 (photos + off-site backups):** in one Cloudflare account,
create private buckets `hive-photos-staging`, `hive-photos-prod` and
`hive-backups` (no public access, no custom domain). Add a bucket lock rule
on `hive-backups` (e.g. 30 days) and a lifecycle rule deleting objects
after 35 days. Create separate *Object Read & Write* tokens per bucket:
photos-staging, photos-prod, backups. Backup env (per environment, own path):

```sh
BACKUP_RCLONE_REMOTE=offsite:hive-backups/production   # or /staging
RCLONE_CONFIG_OFFSITE_TYPE=s3
RCLONE_CONFIG_OFFSITE_PROVIDER=Cloudflare
RCLONE_CONFIG_OFFSITE_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID=…   RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY=…
RCLONE_CONFIG_OFFSITE_NO_CHECK_BUCKET=true   # bucket-scoped tokens cannot list buckets
BACKUP_REMOTE_PRUNE=0                       # the bucket lock/lifecycle owns retention
```

Both environments write `hive-hive-<time>.dump.age`, so they **must** use
different paths; off-site pruning only touches the top level of its own path.

**Alerts:** one private HTTPS webhook (e.g. a private Slack/Discord channel
or ntfy topic) per environment for `BACKUP_ALERT_WEBHOOK`.

**Backup keys:** one age key pair per environment. Only the public key goes on
the VPS; keep the private key offline in two places (e.g. a password manager
plus an encrypted USB key) and prove a restore with it.

### Move staging behind the edge (staging-only, a few seconds downtime)

1. Add `HIVE_EDGE_ALIAS=hive-staging-web` and `HIVE_EDGE_NETWORK=hive-edge-staging`
   to `backend/.env.hostinger`; run the staging preflight.
2. `docker compose --env-file deploy/edge/.env -f deploy/edge/compose.yml create`
   (creates the edge networks without binding ports).
3. Recreate staging `web` with the edge overlay, then stop and remove the
   per-stack `caddy` container only (never `down -v`), and
   `up -d` the edge Caddy. It re-issues the staging certificate.
4. Run `scripts/hostinger/smoke.sh` against staging.

### Bring production up privately (before any DNS change)

1. Clone the reviewed commit into `/opt/hive-prod`; create
   `backend/.env.production` with new secrets; production preflight must pass.
2. Build and start the production stack with the edge overlay. With
   `PROD_DOMAIN` unset at the edge it is reachable only inside the VPS, so
   admins, imports and checks can run through an SSH tunnel to its web container.
3. Rehearse the member import there after staging (see member migration).

### Go/no-go gates (all required)

- Staging acceptance above, plus a public-URL run: signup, verification,
  login, member privacy, admin TOTP and Studio.
- Resend verified on the `mailer.thehivesociety.ae` **subdomain**; the root
  domain's Titan MX/SPF/DMARC records are never edited.
- R2 photo buckets, off-site backup bucket with lock, alert webhook, a second
  offline copy of each age identity, a verified restore per environment.
- Two real allowlisted admins with TOTP; legal approval of Terms/Privacy.
- Legacy member export reconciled and imported into production with counts
  matching the Lovable source.
- SSH key-only and fail2ban (recommended before going public).

### Cutover order

Caddy obtains certificates over HTTP-01, so `thehivesociety.ae` and `www`
can only be issued **after** DNS points here. OnlyDomains has no TTL control
(records publish with TTL 86400s). Query its servers with `dig +norecurse`:
with recursion requested they have returned stale answers (NXDOMAIN for a
record that real resolvers already saw).

1. Save a DNS zone export; take encrypted backups of both databases.
2. Freeze sign-ups on the Lovable site if possible, then take the final
   member export and import the delta (imports are idempotent).
3. In OnlyDomains edit only `@ A` and `www A` → `201.18.200.202`. Leave MX,
   SPF, DMARC, Google verification and every other record untouched.
4. Poll `dig +norecurse @ns1.onlydomains.com` and a public resolver; as soon as both names answer, set
   `PROD_DOMAIN`/`PROD_REDIRECT_FROM` in the edge env and restart the edge
   Caddy; confirm both certificates.
5. Verify: `BASE_URL=https://thehivesociety.ae EXPECT_STAGING_NOINDEX=0
   WWW_ORIGIN=https://www.thehivesociety.ae bash scripts/hostinger/smoke.sh`,
   then signup/verification email/login/admin TOTP/Studio/member privacy, and
   send the member invitation batches.
6. Keep the Lovable deployment up for ~48h while resolver caches expire, then
   delete the `_lovable`, `_lovable.www` and `_lovable-email` TXT records and
   retire the Lovable project.

Rollback: restore `@`/`www` to `185.158.133.1` (only while Lovable is still
up), unset `PROD_DOMAIN` at the edge, and use `rollback.sh` for images.

The single-stack layout (per-stack Caddy with `SITE_REDIRECT_FROM`) remains
supported for a one-environment server.
