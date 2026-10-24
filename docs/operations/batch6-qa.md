# Batch 6 — final code hardening and QA checklist

This document records **code-level checks**. A passing CI run does **not**
substitute for real staging, legal review or a penetration test.

## Changes in Batch 6

- Photo status now uses `has_photo` and `photos_enabled` flags on `GET /v1/me`;
  no R2 object keys or signed/public storage URLs are exposed to the browser.
- Member upload controls are disabled when private storage is not configured.
  Upload/download failures show an error rather than an invented success.
- Photo updates invalidate the member query, and saved photo URLs use an
  in-page revision parameter to refresh after replacement.
- Admin roster returns `has_photo`, not an object storage key, and the
  private image remains behind admin MFA.
- Admin safety-conflict decisions always require explicit confirmation.
  The admin tabs support keyboard arrows, Home and End, with roving tab focus.
- PostgreSQL upgrade test builds a populated v3 database with member,
  avatar reference and reservation, migrates to v4, verifies retention of
  records and new schema, then asserts a second run is a no-op.
- UI tests exercise multipart photo upload, error handling, disabled state,
  private photo removal, conflict confirmation and keyboard navigation.

## Automated acceptance gates

- Go formatting, `go vet`, builds and full `go test -race` suite on
  disposable PostgreSQL 17
- Authenticated SeaweedFS S3 integration tests actually run (not skipped)
- Frontend lint, TypeScript typechecking, Vitest, production build
- Docker Compose and Caddy validation, images, migrations and encrypted
  backup restore/tamper tests
- Next.js-to-Go end-to-end proxy tests for cookies, origin and headers

Record the CI run URL and status before treating the coding phase as done.

## Outstanding staging and legal validation

- Real R2 account/bucket credentials and private access/EXIF/deletion checks
- Real transactional email delivery on an owner-approved sending subdomain
- Encrypted off-site backup configuration and a verified restore drill
- Live Netlify static headers, mobile browser accessibility (including RTL),
  full manual member/admin flows and production-rate security review
- Approved privacy/terms language and retention settings
- Offline member export reconciliation, controlled invitations and an approved
  domain cutover

No changes to production DNS, live data, Titan email or deployed services
are authorized by this coding-only checklist.
