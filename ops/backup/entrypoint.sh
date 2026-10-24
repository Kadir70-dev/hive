#!/bin/bash
# cron   : run backup.sh on BACKUP_SCHEDULE (UTC cron syntax) until stopped
# backup : one backup now
# restore/verify : see restore.sh / verify.sh
set -euo pipefail
cmd="${1:-cron}"; shift || true
case "$cmd" in
  cron)
    : "${BACKUP_SCHEDULE:=15 22 * * *}"   # 22:15 UTC = 02:15 Asia/Dubai
    # Fail fast on missing configuration instead of discovering it at 2 am.
    : "${BACKUP_AGE_RECIPIENT:?BACKUP_AGE_RECIPIENT (age public key) is required}"
    # busybox crond starts jobs with an empty environment: hand over only the
    # variables backup.sh needs, in a root-only file.
    ( umask 077; export -p | grep -E '^(declare -x|export) (PG|BACKUP_|RCLONE_)[A-Z0-9_]*=' > /etc/backup.env )
    # The env file uses bash syntax (declare -x), so the job runs under bash.
    printf '%s /bin/bash -c ". /etc/backup.env && exec /usr/local/bin/backup.sh" >> /proc/1/fd/1 2>&1\n' "$BACKUP_SCHEDULE" > /etc/crontabs/root
    echo "backup: scheduled '$BACKUP_SCHEDULE' (UTC)"
    exec crond -f -l 8 ;;
  backup)  exec /usr/local/bin/backup.sh "$@" ;;
  restore) exec /usr/local/bin/restore.sh "$@" ;;
  verify)  exec /usr/local/bin/verify.sh "$@" ;;
  healthy)
    # Healthy when the last successful backup is recent enough.
    # Unhealthy if the last success is too old, or a later run failed.
    max="${BACKUP_MAX_AGE_HOURS:-26}"
    d="${BACKUP_DIR:-/backups}"
    [ -f "$d/last-success" ] || exit 1
    ok="$(cat "$d/last-success")"
    [ $(( $(date +%s) - ok )) -lt $(( max * 3600 )) ] || exit 1
    if [ -f "$d/last-failure" ] && [ "$(cat "$d/last-failure")" -ge "$ok" ]; then exit 1; fi ;;
  *) exec "$cmd" "$@" ;;
esac
