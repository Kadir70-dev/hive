#!/bin/bash
# Proves a backup is restorable: restores it into a brand-new scratch database
# on the given server, runs sanity checks, then drops only that scratch DB.
#
#   verify.sh BACKUP_FILE
#
# Run on an operator machine or a separate host with AGE_IDENTITY_FILE and
# PG* pointing at a NON-production server (e.g. a throwaway container).
set -euo pipefail
file="${1:?usage: verify.sh BACKUP_FILE}"
scratch="hive_restore_check_$(date -u +%Y%m%d%H%M%S)_$$"
psql --no-password -d postgres -qc "CREATE DATABASE $scratch"
cleanup() { psql --no-password -d postgres -qc "DROP DATABASE IF EXISTS $scratch WITH (FORCE)"; }
trap cleanup EXIT
/usr/local/bin/restore.sh "$file" "$scratch"
for t in users gatherings reservations sessions goose_db_version; do
  psql --no-password -d "$scratch" -tAc "SELECT 1 FROM $t LIMIT 0" >/dev/null || { echo "verify: table $t missing"; exit 1; }
done
echo "verify: OK — $file restores cleanly"
