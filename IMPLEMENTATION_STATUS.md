# The Hive Society — no-Supabase migration checkpoint

## Completed in source
- Removed Supabase SDK import/dependency, Supabase integration source, and old Supabase SQL migrations.
- Built a separate Go + Chi backend using PostgreSQL, Argon2id password hashing, server-side hashed session tokens and secure HttpOnly cookies.
- Added relational schema for accounts, approval status, gatherings, FIFO waitlist, attendance, announcements, partners, reports, blocks and founding signups.
- Converted Next.js member login/signup, authorization gate, profile, gatherings, admin basics, safety, announcements and partners to same-origin Next.js proxy calls to the Go backend.
- Added Docker Compose: Go API, PostgreSQL, Redis, Caddy. Redis backs rate limiting (Batch 1); Asynq background jobs are NOT implemented yet.
- Public signup proxied to Go; the homepage AI chat/recommendation widgets are hidden and disabled (their routes still return 503).

## P0 Batch 1 (branch fix/p0-foundation-security)
- Go build is reproducible (`go.sum` committed, Go 1.24); Docker image builds `api` and `migrate`.
- `package-lock.json` committed; lint, typecheck, tests and production build pass.
- goose migrations with an idempotent baseline; `migrate` runs before the API in Compose and the API refuses to start on pending migrations.
- Redis-backed rate limiting with in-process fallback, per-account login lockout, Argon2id concurrency cap, spoof-resistant client IP handling (see README).
- `http.Server` timeouts, body limits, per-request timeouts, graceful shutdown, `/health` (liveness) vs `/ready` (readiness).
- One transactional seat module (`backend/internal/seats`) for keep/cancel/account deletion/decline with strict FIFO promotion; tested under concurrency on PostgreSQL.
- CI: backend (verify, vet, build, race tests on PostgreSQL), frontend (lint, typecheck, test, build), infrastructure (compose config, image build, migrations).
- Homepage: Join opens sign-up, member detection uses the Go session, the AI widgets are disabled.

## P0 Batch 2 (branch feat/p0-batch2-auth-email, stacked on Batch 1)
- Transactional email: provider interface, Resend client, Postgres outbox + worker; no Lovable/managed backend.
- Email verification (24 h links), forgot/reset password (30 min links) with all sessions revoked on reset; enumeration-safe responses.
- Admin bootstrap CLI (allow-listed + verified only); admin TOTP 2FA with encrypted secrets, recovery codes, replay protection and a 12 h step-up window.
- Hourly cleanup of expired sessions, tokens and outbox rows.
- Nonce-based CSP for pages, hash-pinned CSP for the static homepage, HSTS and other headers.
- Removed the Lovable email preview endpoint, AI stubs, AI SDK and Lovable/react-email dependencies.

## P0 Batch 3 (branch feat/p0-batch3-production-readiness, stacked on Batch 2)
- Notifications through the existing outbox: admin alert when a member confirms their email, membership approval, booking/waitlist confirmation, cancellation, waitlist promotion, gathering changed/cancelled. `admin test-email` for delivery checks.
- Gathering editing (`PUT /v1/admin/gatherings/{id}`) via `seats.SetCapacity`: FIFO promotion on capacity increases, refusal below confirmed places (no demotion). Bookings refused once a gathering has started (Asia/Dubai time); no promotions into past gatherings.
- Security fixes: CSRF origin required for cookie-authenticated mutations; API refuses to start without an origin allow-list when cookies are secure; partner links must be https; account deletion requires the password; proxy rejects dot segments; audit events for logins, admin content changes, check-ins, promotions and deletions.
- Encrypted backups (age, tmpfs plaintext, sha256, retention, optional rclone off-site, health check) with restore/verify tooling and an automated end-to-end test.
- Privacy Policy and Terms rewritten as drafts describing actual behaviour (owner/legal review required).
- Staging readiness: parameterised and hardened Caddy, log rotation, `netlify.toml`, staging guide, proxy end-to-end test in CI.

## P1 Batch 4 (committed directly to main)
- Announcements: admin list/search/pagination, create, edit, publish/unpublish, delete; paginated member feed; email to opted-in, approved, verified members via the outbox with per-member de-duplication (`announcement_deliveries`).
- Safety: admin-mediated reports (category, optional own gathering, description of the person, block request); "my reports" status; admin moderation (status, internal note, link member from roster/search) and admin-applied blocks enforced at booking time in both directions, under the gathering lock, with generic refusals. Member-created blocks by member id were removed.
- Admin dashboard tabs: members (search, status filter, pagination), gatherings (create/edit/cancel, roster with check-in, waitlist positions, safety-conflict flags), announcements, partners (CRUD, https-only), reports.
- Member profile: my gatherings with confirmed/waitlist position/past/attended, validated profile fields, email-updates setting wired to announcement delivery.
- UX: accessible confirm dialog replaces alert()/confirm(); loading/empty/error/success states; Arabic for member screens; small-screen layout fixes.

## Known blockers — DO NOT deploy as production ready
- Resend is integrated but not configured: a Resend account, API key and a verified sending subdomain (DNS records, needs owner approval) are required before any email is delivered. Keep Titan MX/SPF/DMARC as they are.
- Admins must register, confirm their email, be promoted with the admin CLI (`HIVE_ADMIN_EMAILS`), and enrol TOTP. `MFA_ENCRYPTION_KEY` must be generated and backed up; losing it forces re-enrolment.
- Profile photos (Cloudflare R2) are not implemented.
- Blocks do not cancel bookings that already exist when the block is applied; the admin roster flags these conflicts for manual handling.
- Backups: the age key pair must be generated and stored offline, and an off-site rclone remote configured; no production backup/restore has been run.
- Privacy Policy and Terms are drafts with [bracketed] owner decisions (legal entity, retention of security records, governing law, liability).
- Security-record (audit_events) retention is not automated yet.
- Avatar uploads / Cloudflare R2 not implemented; prior photos do not automatically migrate.
- Antifraud, structured audit trail, retention/backups, privacy/legal review, accessibility and security testing are still needed.
- Existing Supabase accounts/data are NOT migrated. Do not delete prior database before an encrypted export and a validated migration. Passwords cannot simply be copied if hash formats are incompatible; require a safe reset/invite flow.
- This archive is a development checkpoint, not a production release.

## Secure deployment outline
1. Copy backend/.env.example to an actual untracked .env and set unrelated strong random secrets.
2. Set DNS for api.thehivesociety.ae to the VPS; **do not change Titan MX/SPF/DMARC**.
3. Start services via `docker compose --env-file backend/.env up -d --build`; the `migrate` service applies migrations before the API starts. Check `/ready`.
4. On Netlify, set HIVE_API_INTERNAL_URL=https://api.thehivesociety.ae, NEXT_PUBLIC_SITE_URL=https://thehivesociety.ae and HIVE_PROXY_SHARED_SECRET (same value as the API, server-only).
5. Create two administrator accounts, verify mailbox ownership and manually grant admin status inside a secure administrative maintenance workflow.
6. Verify cookies, redirects, login, approval, membership-only API responses, admin access, concurrent waitlist allocation and backup restoration before domain cutover.
