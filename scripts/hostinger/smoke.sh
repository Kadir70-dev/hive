#!/usr/bin/env bash
# Safe read-only smoke check against an OWNER-APPROVED isolated Hostinger staging URL.
# Usage: BASE_URL=https://staging.example.test bash scripts/hostinger/smoke.sh
set -euo pipefail
: "${BASE_URL:?Set BASE_URL to the approved HTTPS staging origin}"
base="${BASE_URL%/}"
[[ "$base" =~ ^https://[a-zA-Z0-9.-]+$ ]] || { echo "Expected a bare HTTPS origin"; exit 2; }
check() {
  local path="$1" expected="$2" actual
  actual="$(curl --silent --show-error --max-time 15 --output /dev/null --write-out '%{http_code}' "$base$path")"
  [[ "$actual" = "$expected" ]] || { echo "FAIL: $path returned $actual (expected $expected)"; exit 1; }
  echo "ok: $path ($actual)"
}
check / 200
check /auth 200
check /hive.html 200
check /manifest.webmanifest 200
check /internal/metrics 404
# Anonymous session must be denied; 404 means the Next->Go proxy is missing,
# 503 means the Go backend/database is unavailable.
check /api/backend/me 401
# Check actual browser-facing response security, not just Caddy validation.
headers="$(curl --silent --show-error --max-time 15 -D - -o /dev/null "$base/auth")"
grep -qi '^strict-transport-security:' <<< "$headers" || { echo "FAIL: HSTS missing"; exit 1; }
grep -qi "^content-security-policy:.*'nonce-" <<< "$headers" || { echo "FAIL: page CSP nonce missing"; exit 1; }
hive_headers="$(curl --silent --show-error --max-time 15 -D - -o /dev/null "$base/hive.html")"
grep -qi '^x-frame-options: *SAMEORIGIN' <<< "$hive_headers" || { echo "FAIL: homepage iframe policy missing"; exit 1; }
echo "PASS: Hostinger staging HTTP/security smoke. This does NOT test real email, admin 2FA, backups or member workflows."
