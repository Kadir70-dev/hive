#!/usr/bin/env bash
# Read-only public staging smoke plus disposable oversized-request probes.
set -euo pipefail
: "${BASE_URL:?Set BASE_URL to the approved HTTPS staging origin}"
base="${BASE_URL%/}"
if [[ "$base" =~ ^https://[a-zA-Z0-9.-]+$ ]]; then
  :
elif [[ "${ALLOW_INSECURE_LOCALHOST:-}" == 1 && "$base" =~ ^http://127\.0\.0\.1(:[0-9]+)?$ ]]; then
  :
else
  echo "Expected an HTTPS origin (HTTP allowed only for explicit localhost CI)"; exit 2
fi
check() {
  local path="$1" expected="$2" actual
  actual="$(curl --silent --show-error --max-time 30 --output /dev/null --write-out '%{http_code}' "$base$path")"
  [[ "$actual" = "$expected" ]] || { echo "FAIL: $path returned $actual (expected $expected)"; exit 1; }
  echo "ok: $path ($actual)"
}
check / 200
check /auth 200
check /hive.html 200
check /manifest.webmanifest 200
check /internal/metrics 404
# Anonymous session must be denied; 404 means the proxy is missing and
# 503 means the Go API/database is unavailable.
check /api/backend/me 401
headers="$(curl --silent --show-error --max-time 15 -D - -o /dev/null "$base/auth")"
grep -qi '^strict-transport-security:' <<< "$headers" || { echo "FAIL: HSTS missing"; exit 1; }
grep -qi "^content-security-policy:.*'nonce-" <<< "$headers" || { echo "FAIL: page CSP missing"; exit 1; }
hive_headers="$(curl --silent --show-error --max-time 15 -D - -o /dev/null "$base/hive.html")"
grep -qi '^x-frame-options: *SAMEORIGIN' <<< "$hive_headers" || { echo "FAIL: homepage iframe policy missing"; exit 1; }
if [[ "${EXPECT_STAGING_NOINDEX:-1}" == 1 ]]; then
  grep -qi '^x-robots-tag:.*noindex' <<< "$headers" || { echo "FAIL: staging noindex header missing"; exit 1; }
  curl --silent --show-error --max-time 15 "$base/robots.txt" | grep -q '^Disallow: /' || { echo "FAIL: staging robots.txt must disallow all"; exit 1; }
  curl --silent --show-error --max-time 15 "$base/sitemap.xml" | grep -q 'https://staging.example.test' || { echo "FAIL: sitemap lacks staging hostname"; exit 1; }
fi
# Production-safe probes: send disposable data only; no member operations.
if [[ "${TEST_BODY_LIMITS:-0}" == 1 ]]; then
  status="$(head -c 2097152 /dev/zero | curl -sS --max-time 30 -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/json' --data-binary @- "$base/api/backend/login")"
  [[ "$status" == 413 ]] || { echo "FAIL: 2MB login body returned $status (expected 413)"; exit 1; }
  # The unauthenticated photo is expected to be rejected by auth, NOT by
  # the 6MB Caddy size cap. This is NOT a successful upload test.
  status="$(head -c 5242880 /dev/zero | curl -sS --max-time 30 -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/octet-stream' --data-binary @- "$base/api/backend/me/photo")"
  [[ "$status" != 413 && "$status" != 000 && "$status" -ge 400 ]] || { echo "FAIL: 5MB photo returned $status"; exit 1; }
  status="$(head -c 7340032 /dev/zero | curl -sS --max-time 30 -o /dev/null -w '%{http_code}' -X POST -H 'Content-Type: application/octet-stream' --data-binary @- "$base/api/backend/me/photo")"
  [[ "$status" == 413 ]] || { echo "FAIL: 7MB photo returned $status (expected 413)"; exit 1; }
fi
echo "PASS: Hostinger staging smoke (real delivery, authenticated R2 and backup restore not tested)."
