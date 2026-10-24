# The Hive Society — no-Supabase migration checkpoint

## Completed in source
- Removed Supabase SDK import/dependency, Supabase integration source, and old Supabase SQL migrations.
- Built a separate Go + Chi backend using PostgreSQL, Argon2id password hashing, server-side hashed session tokens and secure HttpOnly cookies.
- Added relational schema for accounts, approval status, gatherings, FIFO waitlist, attendance, announcements, partners, reports, blocks and founding signups.
- Converted Next.js member login/signup, authorization gate, profile, gatherings, admin basics, safety, announcements and partners to same-origin Next.js proxy calls to the Go backend.
- Added Docker Compose: Go API, PostgreSQL, Redis, Caddy. Redis exists in infrastructure but Asynq and cache integration are NOT implemented.
- Public signup proxied to Go; public recommendation and chat features return clear temporary-unavailable errors instead of consulting Supabase.

## Known blockers — DO NOT deploy as production ready
- Go compilation not verified because external module downloads failed in this environment (DNS blocked). Next.js dependency install/typecheck/build also not verified.
- PostgreSQL migration must be applied manually and tested against an isolated database.
- Email verification, password reset, signup alert email, notifications and email opt-in delivery need transactional email integration. Keep Titan MX/SPF/DMARC as they are.
- Admin bootstrap requires manual verified elevation after each of the two intended addresses registers. Never elevate accounts solely from matching email text.
- Avatar uploads / Cloudflare R2 not implemented; prior photos do not automatically migrate.
- Gathering edits (create and delete work), announcement/partner edit actions and moderation resolution remain incomplete.
- Waitlist promotion when an account is deleted needs a transactional release/promotion operation. This edge case is unresolved.
- Rate limits, antifraud, structured audit trail, retention/backups, privacy/legal review, accessibility and security testing are still needed.
- Existing Supabase accounts/data are NOT migrated. Do not delete prior database before an encrypted export and a validated migration. Passwords cannot simply be copied if hash formats are incompatible; require a safe reset/invite flow.
- This archive is a development checkpoint, not a production release.

## Secure deployment outline
1. Copy backend/.env.example to an actual untracked .env and set unrelated strong random secrets.
2. Set DNS for api.thehivesociety.ae to the VPS; **do not change Titan MX/SPF/DMARC**.
3. Create PostgreSQL database and run backend/migrations/001_init.sql with psql before starting API.
4. Start services via docker compose up -d --build. Check API health.
5. On Netlify, set HIVE_API_INTERNAL_URL=https://api.thehivesociety.ae and NEXT_PUBLIC_SITE_URL=https://thehivesociety.ae.
6. Create two administrator accounts, verify mailbox ownership and manually grant admin status inside a secure administrative maintenance workflow.
7. Verify cookies, redirects, login, approval, membership-only API responses, admin access, concurrent waitlist allocation and backup restoration before domain cutover.
