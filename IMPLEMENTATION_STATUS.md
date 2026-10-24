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

## Known blockers — DO NOT deploy as production ready
- Email verification, password reset, signup alert email, notifications and email opt-in delivery need transactional email integration. Keep Titan MX/SPF/DMARC as they are.
- Admin bootstrap requires manual verified elevation after each of the two intended addresses registers. Never elevate accounts solely from matching email text.
- Avatar uploads / Cloudflare R2 not implemented; prior photos do not automatically migrate.
- Gathering edits (create and delete work), announcement/partner edit actions and moderation resolution remain incomplete. **Batch 3:** a gathering edit endpoint must run inside a transaction and call `seats.Fill` after changing capacity; reducing capacity below confirmed seats needs an explicit policy (no automatic demotion exists).
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
