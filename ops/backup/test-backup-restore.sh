#!/bin/bash
# End-to-end backup test against a DISPOSABLE PostgreSQL server.
# Never point this at production: it creates and drops its own test databases.
#
#   PGHOST PGPORT PGUSER PGPASSWORD  — the disposable server (reachable from the host)
#   BACKUP_IMAGE (default hive-backup:test), API_IMAGE (default hive-api:ci)
set -euo pipefail
BACKUP_IMAGE="${BACKUP_IMAGE:-hive-backup:test}"
API_IMAGE="${API_IMAGE:-hive-api:ci}"
: "${PGHOST:?}" "${PGPORT:=5432}" "${PGUSER:?}" "${PGPASSWORD:?}"
export PGPORT
src="hive_bk_src_$$"; dst="hive_bk_dst_$$"; dst2="hive_bk_tamper_$$"
tmp="$(mktemp -d)"; chmod 777 "$tmp"
cleanup() {
  for d in "$src" "$dst" "$dst2"; do psql -d postgres -qc "DROP DATABASE IF EXISTS $d WITH (FORCE)" >/dev/null 2>&1 || true; done
  docker run --rm -v "$tmp:/t" alpine:3.21 sh -c 'rm -rf /t/*' >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT
run() { docker run --rm --network host -e PGHOST -e PGPORT -e PGUSER -e PGPASSWORD --tmpfs /work -v "$tmp:/backups" "$@"; }

echo "== seed a migrated source database"
psql -d postgres -qc "CREATE DATABASE $src" -c "CREATE DATABASE $dst" -c "CREATE DATABASE $dst2"
docker run --rm --network host -e DATABASE_URL="postgres://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/$src?sslmode=disable" --entrypoint migrate "$API_IMAGE" up
psql -d "$src" -v ON_ERROR_STOP=1 -q <<'SQL'
INSERT INTO users(email,password_hash,first_name,status,email_verified_at)
  SELECT 'm'||i||'@example.test','x','Member '||i,'approved',now() FROM generate_series(1,50) i;
INSERT INTO gatherings(title,starts_on,starts_at,place,capacity) VALUES ('Backup test','2031-01-01','10:00','Abu Dhabi',10);
INSERT INTO reservations(gathering_id,user_id,status) SELECT g.id,u.id,'confirmed' FROM gatherings g, (SELECT id FROM users LIMIT 10) u;
SQL

echo "== generate a throwaway age key pair"
run --entrypoint age-keygen "$BACKUP_IMAGE" -o /backups/test-identity.txt 2>/dev/null
recipient="$(run --entrypoint age-keygen "$BACKUP_IMAGE" -y /backups/test-identity.txt)"

echo "== old backup older than retention (must be deleted)"
run --entrypoint sh "$BACKUP_IMAGE" -c 'touch -d "20 days ago" /backups/hive-old-20000101T000000Z.dump.age'

echo "== backup"
run -e PGDATABASE="$src" -e BACKUP_AGE_RECIPIENT="$recipient" -e BACKUP_RETENTION_DAYS=14 "$BACKUP_IMAGE" backup
file="$(ls "$tmp" | grep -E '^hive-.*\.dump\.age$' | grep -v old- | head -1)"
[ -n "$file" ] || { echo "FAIL: no backup produced"; exit 1; }
[ ! -e "$tmp/hive-old-20000101T000000Z.dump.age" ] || { echo "FAIL: retention did not delete the old backup"; exit 1; }
run --entrypoint sh "$BACKUP_IMAGE" -c 'test -s /backups/last-success' || { echo "FAIL: last-success marker missing"; exit 1; }
run --entrypoint /usr/local/bin/entrypoint.sh "$BACKUP_IMAGE" healthy || { echo "FAIL: health check should pass after a backup"; exit 1; }
# Backups are root-owned and mode 0600, so inspect them from inside the container.
header="$(run --entrypoint sh "$BACKUP_IMAGE" -c "head -c 21 /backups/$file; echo; stat -c %a /backups/$file")"
case "$header" in *PGDMP*) echo "FAIL: backup is not encrypted"; exit 1 ;; esac
echo "$header" | grep -q 'age-encryption.org' || { echo "FAIL: not an age file"; exit 1; }
echo "$header" | tail -1 | grep -qx 600 || { echo "FAIL: backup file must be mode 600"; exit 1; }
if run --entrypoint sh "$BACKUP_IMAGE" -c "ls /work 2>/dev/null | grep -q dump"; then echo "FAIL: plaintext left behind"; exit 1; fi
echo "encrypted: $file"

echo "== restore into an empty database and compare"
run -e AGE_IDENTITY_FILE=/backups/test-identity.txt "$BACKUP_IMAGE" restore "/backups/$file" "$dst"
fingerprint() { psql -d "$1" -tAc "SELECT (SELECT count(*) FROM users)||'/'||(SELECT count(*) FROM reservations)||'/'||(SELECT md5(string_agg(email,',' ORDER BY email)) FROM users)||'/'||(SELECT max(version_id) FROM goose_db_version)"; }
a="$(fingerprint "$src")"; b="$(fingerprint "$dst")"
[ "$a" = "$b" ] || { echo "FAIL: restored data differs: $a vs $b"; exit 1; }
echo "restored data matches: $a"

echo "== restore refuses a non-empty target"
if run -e AGE_IDENTITY_FILE=/backups/test-identity.txt "$BACKUP_IMAGE" restore "/backups/$file" "$dst" >/dev/null 2>&1; then
  echo "FAIL: restore overwrote a non-empty database"; exit 1; fi

echo "== verify (scratch database is created and dropped)"
run -e AGE_IDENTITY_FILE=/backups/test-identity.txt "$BACKUP_IMAGE" verify "/backups/$file"

echo "== tampered backup is rejected"
run --entrypoint sh "$BACKUP_IMAGE" -c "cp /backups/$file /backups/tampered.dump.age && cp /backups/$file.sha256 /backups/tampered.sha && sed 's/$file/tampered.dump.age/' /backups/tampered.sha > /backups/tampered.dump.age.sha256 && printf 'X' | dd of=/backups/tampered.dump.age bs=1 seek=200 conv=notrunc 2>/dev/null"
if run -e AGE_IDENTITY_FILE=/backups/test-identity.txt "$BACKUP_IMAGE" restore /backups/tampered.dump.age "$dst2" >/dev/null 2>&1; then
  echo "FAIL: tampered backup restored"; exit 1; fi
echo "== failure alert: unreachable database -> webhook alert + unhealthy"
alert_port=$(( 20000 + RANDOM % 20000 ))
docker run -d --name "hive-alert-$$" --network host alpine:3.21 \
  sh -c "printf 'HTTP/1.1 200 OK\r\nContent-Length: 0\r\nConnection: close\r\n\r\n' | nc -l -p $alert_port" >/dev/null
sleep 1
if run -e PGDATABASE=database_that_does_not_exist -e BACKUP_AGE_RECIPIENT="$recipient" \
     -e BACKUP_ALERT_WEBHOOK="http://127.0.0.1:$alert_port/hook" "$BACKUP_IMAGE" backup >/dev/null 2>&1; then
  echo "FAIL: backup of a missing database reported success"; exit 1
fi
sleep 1
alert="$(docker logs "hive-alert-$$" 2>&1 || true)"
docker rm -f "hive-alert-$$" >/dev/null 2>&1 || true
echo "$alert" | grep -q '"status":"failed"' || { echo "FAIL: no failure alert received"; echo "$alert"; exit 1; }
echo "$alert" | grep -q '"step":"dump"' || { echo "FAIL: alert does not name the failing step"; exit 1; }
if echo "$alert" | grep -qiE "$PGPASSWORD|age1|AGE-SECRET"; then echo "FAIL: alert leaks secrets"; exit 1; fi
if run --entrypoint /usr/local/bin/entrypoint.sh "$BACKUP_IMAGE" healthy; then
  echo "FAIL: health check should fail after a failed backup"; exit 1; fi
echo "alert received and health check unhealthy"

echo "PASS: backup, encryption, retention, restore, verify, tamper detection and failure alerts"
