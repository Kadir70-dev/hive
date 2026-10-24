# Backup and recovery — The Hive Society

Status: implemented and tested against disposable databases only. **No production
backup or restore has been performed.** Review with the owner before launch.

## What runs

| Item | Detail |
|---|---|
| Service | `backup` in `docker-compose.yml` (image `ops/backup`) |
| Schedule | `BACKUP_SCHEDULE`, cron syntax in **UTC**; default `15 22 * * *` = 02:15 Asia/Dubai |
| Format | `pg_dump --format=custom`, checked with `pg_restore --list` before encryption |
| Encryption | [age](https://age-encryption.org) to the **public** key `BACKUP_AGE_RECIPIENT`. The private key never lives on the server, so a compromised server cannot read old backups |
| Plaintext | exists only in the `/work` tmpfs (RAM) for the few seconds between dump and encryption |
| Integrity | `.sha256` sidecar per backup; restore refuses on mismatch; age's authenticated encryption rejects tampering |
| Local copy | Docker volume `backups` (`hive-<db>-<UTC timestamp>.dump.age`, mode 600) |
| Off-site copy | `BACKUP_RCLONE_REMOTE` (e.g. Hetzner Storage Box or Cloudflare R2) — **strongly recommended; not configured yet** |
| Retention | `BACKUP_RETENTION_DAYS` (default 14) locally and off-site; only files matching `hive-*.dump.age*` are ever deleted |
| Monitoring | container health check fails if the last success is older than `BACKUP_MAX_AGE_HOURS` (26) |

## One-time setup (owner/operator)

1. On a trusted computer (not the server): `age-keygen -o hive-backup-identity.txt`.
   The file contains the private key; the `# public key: age1…` line is the recipient.
2. Store the identity file in **two** safe places (e.g. a password manager vault and an
   offline USB key). Losing it makes every backup unreadable.
3. Put only the public key in `backend/.env`: `BACKUP_AGE_RECIPIENT=age1…`.
4. Recommended: configure an off-site remote, e.g. for R2:
   ```
   BACKUP_RCLONE_REMOTE=offsite:hive-backups
   RCLONE_CONFIG_OFFSITE_TYPE=s3
   RCLONE_CONFIG_OFFSITE_PROVIDER=Cloudflare
   RCLONE_CONFIG_OFFSITE_ENDPOINT=https://<account>.r2.cloudflarestorage.com
   RCLONE_CONFIG_OFFSITE_ACCESS_KEY_ID=…      # write-only token if possible
   RCLONE_CONFIG_OFFSITE_SECRET_ACCESS_KEY=…
   ```
   Prefer a bucket with object lock/versioning so a compromised server cannot delete history.

## Routine checks

- `docker compose ps backup` shows `healthy`.
- Monthly: run a **verified restore** (below) on a non-production machine and record the result.

## Verified restore test (safe — never touches production)

On an operator machine with Docker and the identity file:

```bash
# 1. A throwaway PostgreSQL 17 server in RAM
docker run -d --name hive-restore-check --tmpfs /var/lib/postgresql/data -e POSTGRES_PASSWORD=check -p 127.0.0.1:55499:5432 postgres:17-alpine
# 2. Copy the latest backup (+ .sha256) from the server or off-site storage into ./restore/
# 3. Restore into a scratch database, run sanity checks, drop the scratch database
docker build -t hive-backup ops/backup
docker run --rm --network host -e PGHOST=127.0.0.1 -e PGPORT=55499 -e PGUSER=postgres -e PGPASSWORD=check \
  -e AGE_IDENTITY_FILE=/keys/identity.txt -v "$PWD/restore:/backups:ro" -v "$HOME/keys:/keys:ro" --tmpfs /work \
  hive-backup verify /backups/hive-hive-<timestamp>.dump.age
docker rm -f hive-restore-check
```

`ops/backup/test-backup-restore.sh` runs the whole cycle automatically (backup →
encryption check → retention → restore → data comparison → verify → tamper
rejection); CI runs it on every pull request.

## Disaster recovery (production)

Use only with the owner's explicit go-ahead. Nothing here drops or overwrites data:
`restore.sh` refuses any target database that already has tables.

1. **Stop writes**: `docker compose stop api backup` (the site shows "API unavailable").
2. **Keep the damaged database**: do not delete the `postgres_data` volume; rename or
   snapshot it (Hetzner volume/server snapshot) for investigation.
3. **Create an empty database** to restore into, e.g. `hive_restored`:
   `docker compose exec db createdb -U hive hive_restored`
4. **Restore** the chosen backup (identity file mounted read-only, only for this step):
   ```bash
   docker compose run --rm -e PGHOST=db -e PGUSER=hive -e AGE_IDENTITY_FILE=/keys/identity.txt \
     -v /secure/keys:/keys:ro backup restore /backups/hive-hive-<timestamp>.dump.age hive_restored
   ```
5. **Check** user/gathering counts and the goose version printed by the restore.
6. **Switch**: point `DATABASE_URL` at `hive_restored` (or rename databases while the API
   is stopped), run `migrate up` (no-op unless the backup predates a migration), start `api`.
7. **Remove the identity file** from the server; record what happened.

Recovery point: up to 24 h of data loss with the default schedule. Recovery time:
minutes for a small database. If a lower RPO is needed, add WAL archiving
(e.g. pgBackRest) — not implemented.

## Limitations

- Off-site copies are not configured; a server loss today would also lose local backups.
- Backups contain personal data of deleted members until they age out (privacy draft says so).
- No point-in-time recovery between nightly backups.
