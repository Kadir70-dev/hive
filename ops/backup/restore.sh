#!/bin/bash
# Restores an encrypted backup into a database that must be EMPTY.
#
#   restore.sh BACKUP_FILE TARGET_DATABASE
#
# Needs the age private key: AGE_IDENTITY_FILE (path) — keep it offline and
# mount it only for a restore. Connection: standard PG* variables (the target
# server). Safety: verifies the sha256 sidecar, refuses non-empty targets and
# never drops or overwrites anything.
set -euo pipefail
umask 077
file="${1:?usage: restore.sh BACKUP_FILE TARGET_DATABASE}"
target="${2:?usage: restore.sh BACKUP_FILE TARGET_DATABASE}"
: "${AGE_IDENTITY_FILE:?AGE_IDENTITY_FILE (age private key file) is required}"
work="${BACKUP_WORK_DIR:-/work}"
mkdir -p "$work"
plain="$work/restore-$$.dump"
trap 'rm -f "$plain"' EXIT

if [ -f "$file.sha256" ]; then
  ( cd "$(dirname "$file")" && sha256sum --check --status "$(basename "$file").sha256" ) \
    || { echo "restore: checksum mismatch for $file; refusing"; exit 1; }
  echo "restore: checksum ok"
else
  echo "restore: WARNING no .sha256 sidecar found; integrity not pre-checked"
fi

tables="$(psql --no-password -d "$target" -tAc "SELECT count(*) FROM pg_tables WHERE schemaname='public'")"
if [ "$tables" != "0" ]; then
  echo "restore: target database '$target' is not empty ($tables tables); refusing to overwrite"; exit 1
fi

age --decrypt --identity "$AGE_IDENTITY_FILE" --output "$plain" "$file"
pg_restore --no-password --no-owner --no-privileges --exit-on-error --single-transaction -d "$target" "$plain"
echo "restore: restored into '$target'"
psql --no-password -d "$target" -tAc "SELECT 'goose version: ' || COALESCE(max(version_id)::text,'none') FROM goose_db_version WHERE is_applied" || true
psql --no-password -d "$target" -tAc "SELECT 'users: ' || count(*) FROM users" || true
