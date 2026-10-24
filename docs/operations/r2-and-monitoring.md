# Private avatar storage, metrics and audit retention

This is configuration guidance, **not evidence that any external service is live**.

## Cloudflare R2 profile photos

1. Create a **private** R2 bucket. Do not enable public bucket access or a public
   custom domain. Use R2's S3-compatible endpoint for the appropriate account.
2. Create an API token scoped to this one bucket with only the necessary object
   read/write/delete and bucket-lookup rights; store it as a server secret.
3. Configure only the Go API via an untracked `backend/.env` file:
   ```dotenv
   R2_ENDPOINT=https://ACCOUNT_ID.r2.cloudflarestorage.com
   R2_BUCKET=PRIVATE_BUCKET
   R2_ACCESS_KEY_ID=SET_IN_SECRET_STORE
   R2_SECRET_ACCESS_KEY=SET_IN_SECRET_STORE
   R2_REGION=auto
   ```
   Do not put R2 credentials in Netlify public variables.
4. Restart staging **only after explicit owner approval**. Check `GET /ready`
   reports `storage: ok`; a missing bucket or wrong credentials makes photo
   operations unavailable until fixed.
5. With disposable staging accounts, upload JPEG/PNG/WebP files, replace and
   delete photos, verify original EXIF location metadata is absent, test
   cross-member read is denied, and confirm deletion queue retries. File size
   is limited to 5 MiB before decoding; Caddy allows 6 MB only for
   `POST /v1/me/photo`; the Go backend applies the same per-route bound.
6. The member image is served only through `GET /api/backend/me/photo`.
   Admin access is limited to MFA-gated
   `GET /api/backend/admin/members/{id}/photo`.
   The bucket must remain private.

In CI a disposable SeaweedFS server uses
`scripts/ci/seaweedfs-s3.json` with **test-only credentials** to exercise real
S3 operations and rejection of incorrect credentials. Never copy these values to
R2 or staging.

## Metrics and alerting

- Set `METRICS_TOKEN` to a random 32+ character secret if private Prometheus
  scraping is required. Otherwise `/internal/metrics` returns 404.
- The route is restricted by the Go bearer-token check and Caddy denies any
  public `/internal/*` request. Connect an internal scraper via a private
  network, not through the public API hostname.
- Metric values contain only service counters and ages, not member data.
- Monitor public `/ready` and `backup` container health separately.
  The API's readiness may return 200 with `storage: degraded` while photos
  cannot load; monitoring must inspect the JSON, not only the HTTP status.
- `BACKUP_ALERT_WEBHOOK` (optional) should be a private HTTPS endpoint
  controlled by the operator. Failed backup attempts post a minimal alert
  with service/status/step/timestamp and cause backup health to fail.
  Protect the destination URL as a secret, allow only required receivers, and
  validate a deliberately induced failure in staging.
- Configure uptime monitors and off-site encrypted backup storage
  **before production**. Schedule a periodic restore drill.

## Audit retention

`AUDIT_RETENTION_DAYS=0` (the default) means audit records are **not
automatically deleted**. A nonzero value must be at least 30 days. Do not enable
retention cleanup until the owner and legal reviewer approve the period;
publish that decision in the Privacy Policy. Database logs may have separate
retention and should be managed independently. Never rely on audit records
as the only disaster-recovery backup.
