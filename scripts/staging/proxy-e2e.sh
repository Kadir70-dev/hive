#!/bin/bash
# End-to-end check of the browser -> Next.js proxy -> Go API path, run locally
# or in CI against a DISPOSABLE PostgreSQL database. It starts the real API
# binary and the production Next.js server, then verifies:
#   cookies survive the proxy, client-IP trust (x-nf-client-connection-ip +
#   shared secret), client-supplied IP headers are ignored, Retry-After is
#   passed through, CSRF origin checks, and security headers.
#
# Env: DATABASE_URL (migrated, disposable), API_BIN, NEXT_PORT (3917), API_PORT (18120)
set -euo pipefail
: "${DATABASE_URL:?}" "${API_BIN:?path to the built api binary}"
NEXT_PORT="${NEXT_PORT:-3917}"; API_PORT="${API_PORT:-18120}"
# The Hostinger test simulates Caddy's overwritten verified-client-ip header.
# This does not itself prove Caddy header sanitization; config validation does.
EDGE="${HIVE_SITE_EDGE:-netlify}"
case "$EDGE" in
  netlify) IP_HEADER="x-nf-client-connection-ip" ;;
  caddy) IP_HEADER="x-hive-verified-client-ip" ;;
  *) echo "unknown HIVE_SITE_EDGE"; exit 2 ;;
esac
SITE="http://127.0.0.1:$NEXT_PORT"
SECRET="e2e-proxy-secret-$(head -c 16 /dev/urandom | od -An -tx1 | tr -d ' \n')"
work="$(mktemp -d)"; jar="$work/cookies"
pids=()
cleanup() {
  for p in "${pids[@]}"; do kill "$p" 2>/dev/null || true; done
  # Fallback: anything still listening on our two ports was started by this script.
  for port in "$NEXT_PORT" "$API_PORT"; do
    for p in $(ss -ltnpH "sport = :$port" 2>/dev/null | grep -o 'pid=[0-9]*' | cut -d= -f2); do kill "$p" 2>/dev/null || true; done
  done
  rm -rf "$work"
}
trap cleanup EXIT
fail() { echo "FAIL: $*"; echo "--- api log"; tail -20 "$work/api.log" || true; exit 1; }

PORT="$API_PORT" DATABASE_URL="$DATABASE_URL" COOKIE_SECURE=false MAIL_PROVIDER=log MAIL_LOG_BODIES=false \
  ALLOWED_ORIGINS="$SITE" HIVE_PROXY_SHARED_SECRET="$SECRET" PUBLIC_APP_URL="$SITE" \
  "$API_BIN" > "$work/api.log" 2>&1 & pids+=($!)
HIVE_API_INTERNAL_URL="http://127.0.0.1:$API_PORT" HIVE_PROXY_SHARED_SECRET="$SECRET" HIVE_SITE_EDGE="$EDGE" \
  node node_modules/next/dist/bin/next start -p "$NEXT_PORT" > "$work/next.log" 2>&1 & pids+=($!)
for i in $(seq 1 60); do curl -fs -o /dev/null "http://127.0.0.1:$API_PORT/ready" && curl -fs -o /dev/null "$SITE/auth" && break; sleep 1; done

api() { # method path [json] [extra curl args...]
  local m="$1" p="$2" d="${3:-}"; shift 3 || shift $#
  curl -s -o "$work/body" -D "$work/headers" -w '%{http_code}' -X "$m" -b "$jar" -c "$jar" \
    -H 'Content-Type: application/json' ${d:+--data "$d"} "$@" "$SITE/api/backend$p"
}

echo "== signup, verify and login through the proxy"
email="e2e-$RANDOM@example.test"
[ "$(api POST /signup "{\"email\":\"$email\",\"password\":\"a-long-password-123\",\"firstName\":\"E2E\"}" -H "Origin: $SITE")" = 202 ] || fail "signup"
token="$(psql "$DATABASE_URL" -tAc "SELECT substring(text_body from '#token=([A-Za-z0-9_-]+)') FROM email_outbox WHERE to_addr='$email' AND template='verify_email' ORDER BY id DESC LIMIT 1")"
[ -n "$token" ] || fail "no verification email queued"
[ "$(api POST /verify-email "{\"token\":\"$token\"}" -H "Origin: $SITE")" = 200 ] || fail "verify"
[ "$(api POST /login "{\"email\":\"$email\",\"password\":\"a-long-password-123\"}" -H "Origin: $SITE" -H "$IP_HEADER: 198.51.100.10")" = 200 ] || fail "login"
grep -i '^set-cookie: hive_session=' "$work/headers" | grep -qi 'httponly' || fail "session cookie missing or not HttpOnly"
grep -i '^set-cookie: hive_session=' "$work/headers" | grep -qi 'samesite=lax' || fail "session cookie not SameSite=Lax"
[ "$(api GET /me)" = 200 ] && grep -q "\"$email\"" "$work/body" || fail "cookie not accepted on /me"
grep -qi "^content-security-policy: default-src 'none'" "$work/headers" || fail "API responses lack CSP"
echo "ok: cookie round-trip and API security headers"

echo "== CSRF: cookie-authenticated request from a foreign origin"
[ "$(api PATCH /me '{"firstName":"X"}' -H 'Origin: https://evil.example')" = 403 ] || fail "foreign origin accepted"
[ "$(api PATCH /me '{"firstName":"X"}' -H "Origin: $SITE")" = 200 ] || fail "same-origin update refused"
echo "ok: CSRF origin check"

echo "== client-IP trust: per-IP login limit (30/10 min) keyed on the Netlify client IP"
rm -f "$jar"
for i in $(seq 1 30); do
  c="$(api POST /login "{\"email\":\"nobody$i-$RANDOM@example.test\",\"password\":\"wrong-password-123\"}" -H "Origin: $SITE" -H "$IP_HEADER: 198.51.100.7")"
  [ "$c" = 401 ] || fail "attempt $i returned $c"
done
c="$(api POST /login '{"email":"x@example.test","password":"wrong-password-123"}' -H "Origin: $SITE" -H "$IP_HEADER: 198.51.100.7")"
[ "$c" = 429 ] || fail "31st attempt from the same client IP returned $c"
grep -qi '^retry-after: [0-9]' "$work/headers" || fail "Retry-After not passed through"
# A client cannot escape its bucket by sending the internal headers itself.
c="$(api POST /login '{"email":"y@example.test","password":"wrong-password-123"}' -H "Origin: $SITE" -H "$IP_HEADER: 198.51.100.7" -H 'x-hive-client-ip: 203.0.113.99' -H "x-hive-proxy-secret: guess" -H 'x-forwarded-for: 203.0.113.99')"
[ "$c" = 429 ] || fail "spoofed client headers escaped the limit ($c)"
c="$(api POST /login '{"email":"z@example.test","password":"wrong-password-123"}' -H "Origin: $SITE" -H "$IP_HEADER: 198.51.100.8")"
[ "$c" = 401 ] || fail "a different client IP should have its own bucket ($c)"
echo "ok: client IP is trusted only via the proxy, spoofing ignored, Retry-After forwarded"

echo "== page security headers through the production server"
curl -s -D "$work/page" -o /dev/null "$SITE/auth"
grep -qi "^content-security-policy:.*'nonce-" "$work/page" || fail "page CSP nonce missing"
curl -s -D "$work/home" -o /dev/null "$SITE/hive.html"
grep -qi "^content-security-policy:.*'sha256-" "$work/home" || fail "homepage CSP hash missing"
[ "$(curl -s --path-as-is -o /dev/null -w "%{http_code}" "$SITE/api/backend/../health")" != 200 ] || fail "proxy reached a non-/v1 route"
echo "PASS: $EDGE-style proxy preserves cookies, client-IP trust and security headers"
