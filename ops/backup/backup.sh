#!/bin/bash
# Creates one encrypted PostgreSQL backup.
#
#   pg_dump (custom format) -> integrity check (pg_restore --list)
#   -> age encryption to BACKUP_AGE_RECIPIENT -> sha256 -> optional rclone copy
#   -> retention cleanup -> last-success marker
#
# The plaintext dump only ever exists in BACKUP_WORK_DIR, which Compose mounts
# as tmpfs (RAM). Only the public key is on the server, so the server cannot
# decrypt its own backups. Exits non-zero on any failure.
#
# Env: PGHOST PGPORT PGUSER PGPASSWORD PGDATABASE (standard libpq),
#      BACKUP_AGE_RECIPIENT (age1…), BACKUP_DIR (/backups),
#      BACKUP_WORK_DIR (/work), BACKUP_RETENTION_DAYS (14),
#      BACKUP_RCLONE_REMOTE (optional, e.g. "offsite:hive-backups"; configure
#      the remote with RCLONE_CONFIG_<NAME>_* environment variables),
#      BACKUP_REMOTE_PRUNE (1; set 0 when a bucket lock/lifecycle rule owns
#      off-site retention, so the server never needs delete rights there).
set -euo pipefail
umask 077
: "${BACKUP_AGE_RECIPIENT:?BACKUP_AGE_RECIPIENT is required}"
: "${PGDATABASE:?PGDATABASE is required}"
dir="${BACKUP_DIR:-/backups}"
work="${BACKUP_WORK_DIR:-/work}"
retention="${BACKUP_RETENTION_DAYS:-14}"
mkdir -p "$dir" "$work"

ts="$(date -u +%Y%m%dT%H%M%SZ)"
name="hive-${PGDATABASE}-${ts}.dump.age"
plain="$work/${name%.age}"
trap 'rm -f "$plain" "$dir/$name.partial"' EXIT

log() { echo "backup: $(date -u +%FT%TZ) $*"; }

# Failure alerting: record the failure (the health check turns unhealthy) and,
# if BACKUP_ALERT_WEBHOOK is set, POST a small JSON alert. The payload holds no
# secrets or data: service, status, failing step and time only.
step="start"
on_failure() {
  date +%s > "$dir/last-failure"
  log "ERROR: backup failed during step '$step'"
  if [ -n "${BACKUP_ALERT_WEBHOOK:-}" ]; then
    payload="{\"service\":\"hive-backup\",\"status\":\"failed\",\"step\":\"$step\",\"time\":\"$(date -u +%FT%TZ)\"}"
    wget -q -T 10 -O /dev/null --header 'Content-Type: application/json' --post-data "$payload" "$BACKUP_ALERT_WEBHOOK" \
      || log "WARNING: alert webhook could not be reached"
  fi
}
trap on_failure ERR

step="dump"
log "dumping database ${PGDATABASE}"
pg_dump --format=custom --compress=6 --no-password --file="$plain"

step="verify"
# Integrity: the archive's table of contents must be readable and non-empty.
entries="$(pg_restore --list "$plain" | grep -vc '^;' || true)"
if [ "${entries:-0}" -lt 1 ]; then
  log "ERROR: dump has no restorable entries"; false
fi
log "dump ok: $(stat -c %s "$plain") bytes, ${entries} archive entries"

step="encrypt"
age --encrypt --recipient "$BACKUP_AGE_RECIPIENT" --output "$dir/$name.partial" "$plain"
rm -f "$plain"
mv "$dir/$name.partial" "$dir/$name"
( cd "$dir" && sha256sum "$name" > "$name.sha256" )
log "encrypted backup written: $name"

if [ -n "${BACKUP_RCLONE_REMOTE:-}" ]; then
  step="offsite"
  rclone copyto --immutable "$dir/$name" "$BACKUP_RCLONE_REMOTE/$name"
  rclone copyto --immutable "$dir/$name.sha256" "$BACKUP_RCLONE_REMOTE/$name.sha256"
  # Anchored patterns: only top-level files, never nested environment paths.
  rclone check --one-way "$dir" "$BACKUP_RCLONE_REMOTE" --include "/$name" --include "/$name.sha256"
  if [ "${BACKUP_REMOTE_PRUNE:-1}" = 1 ]; then
    rclone delete --min-age "${retention}d" --include '/hive-*.dump.age*' "$BACKUP_RCLONE_REMOTE"
  else
    log "off-site retention left to the bucket's lock/lifecycle rules"
  fi
  log "copied off-site to $BACKUP_RCLONE_REMOTE"
else
  log "WARNING: BACKUP_RCLONE_REMOTE not set; backups are only on this server"
fi

step="retention"
# Retention: only files this script creates are ever deleted.
find "$dir" -maxdepth 1 -type f -name 'hive-*.dump.age*' -mtime +"$retention" -print -delete | sed 's/^/backup: expired /'
date +%s > "$dir/last-success"
rm -f "$dir/last-failure"
log "done"
