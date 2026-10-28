# Legacy member migration — offline only

**Not a production cutover instruction.** The previous live membership database remains
untouched until the operator has completed an encrypted export, reconciled all
records, and obtained the business owner's approval. No live database credentials
or password hashes belong in the repository or in CI.

## Supported input format

Use an offline directory available only to the operator (mode `0700`).
The importer accepts three CSVs with header rows. Never commit the export.

- `members.csv` (required): `legacy_id,email,first_name`; optional
  `display_name,phone,area,interests,status,email_confirmed,email_updates,created_at`.
  `interests` is semicolon-separated. Map the source's real email confirmation
  status; do not mark unverified accounts as verified.
- `gatherings.csv` (optional): `legacy_id,title,starts_on,starts_at,place,capacity`;
  optional `type,description`. Date uses YYYY-MM-DD, time HH:MM in
  Asia/Dubai.
- `bookings.csv` (optional):
  `legacy_member_id,legacy_gathering_id,status,created_at`.
  Preserve original chronological booking timestamps for FIFO order.

`backend/internal/importer/testdata/export/` contains **synthetic** fixtures to
verify the format. Inspect validation rules in `backend/internal/importer/input.go`.
Unknown source fields are ignored; CSV values are not SQL identifiers.

## Exporting from the Lovable Cloud site (current thehivesociety.ae)

What the live site's public code shows (2026-10-28; no data was accessed):
Supabase Auth (`auth.users`) for sign-up/sign-in, `public.profiles` (read
with `select *`, keyed by the user id), `public.user_roles` (`role = 'admin'`)
and gatherings via the `list_gatherings` RPC. Writes happen in server
functions, so the exact profile, gathering and booking columns are only
visible to someone with access to the Lovable project.

**Who:** the Lovable project owner, in the project's Cloud/database view (SQL
editor or table CSV export). Nobody else connects to that database, and its
data is never modified.

### Step 1 — schema and counts only (no personal data; safe to share)

```sql
select table_name, column_name, data_type
from information_schema.columns
where table_schema = 'public'
order by table_name, ordinal_position;

select 'auth.users' as source, count(*) from auth.users
union all select 'confirmed', count(*) from auth.users where email_confirmed_at is not null
union all select 'profiles', count(*) from public.profiles
union all select 'admins', count(*) from public.user_roles where role = 'admin';
```

The operator turns the column list into the final mapping below; the counts
become the reconciliation target.

### Step 2 — members.csv (personal data: never via chat, email or Git)

Template; replace each `p.<…>` with the real column name from step 1
(drop a line if the column does not exist):

```sql
select u.id as legacy_id,
       lower(u.email) as email,
       coalesce(nullif(p.first_name, ''), split_part(coalesce(p.full_name, ''), ' ', 1),
                u.raw_user_meta_data->>'first_name', 'Member') as first_name,
       p.display_name, p.phone, p.area,
       array_to_string(p.interests, ';') as interests,
       coalesce(p.status, 'pending') as status,          -- pending/approved/declined (aliases accepted)
       (u.email_confirmed_at is not null) as email_confirmed,
       coalesce(p.email_updates, false) as email_updates,
       to_char(u.created_at at time zone 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"') as created_at
from auth.users u
left join public.profiles p on p.id = u.id
where u.email is not null
order by u.created_at;
```

Export `gatherings.csv` / `bookings.csv` the same way if the site has real
bookings (keep original booking timestamps for waitlist order).

### Step 3 — hand-over and rehearsal

1. Save the CSVs on the owner's machine, encrypt them with the import age
   public key (`age -r age1… -o hive-export.tar.age hive-export.tar`) and
   copy only the encrypted file to the VPS (`/root/hive-imports`, mode 0700).
2. `plan` against **staging**; resolve every reported row issue with the
   owner (the report has row numbers and counts only).
3. `apply` to staging, compare counts with step 1, test a member invitation
   end to end with a disposable address.
4. Repeat `plan` and `apply` on the private production stack. At cutover take
   a final export and re-apply (idempotent; only new rows are added).
5. Passwords never move: each member receives a single-use set-password
   invitation (`invite`) after cutover. Lovable admins are **not** promoted
   automatically; real admins use the allowlist, CLI promotion and TOTP.
6. Delete decrypted exports after import; keep the encrypted file only as long
   as the approved retention policy says.

## Safe procedure

1. Make and verify an encrypted backup of the **target** PostgreSQL database
   and a separate encrypted export of the previous live system.
2. Restore the target into an isolated staging database with the latest migrations.
   Avoid importing into an active member database.
3. Generate a redacted dry-run report. From the repository root (with a trusted
   local Go toolchain), for example:

   ```bash
   cd backend
   DATABASE_URL="$STAGING_DATABASE_URL" go run ./cmd/import-members plan --dir /secure/exports/hive
   ```

4. Review row-numbered errors, duplicate emails, approval states, waitlist
   order, capacity limits and relationships. Do not move ahead if unexplained
   issues remain.
5. With explicit approval, apply to the *intended staging database only*.
   The `--confirm-database` argument must equal the actual PostgreSQL database
   name; it is not an environment-label string.

   ```bash
   DATABASE_URL="$STAGING_DATABASE_URL" go run ./cmd/import-members apply \
     --dir /secure/exports/hive --confirm-database hive_staging
   ```

6. Compare member, gathering and reservation counts. Re-run the dry run to
   verify that the legacy-to-new mappings are idempotent. Inspect approvals and
   waitlist order manually.
7. Preview invitation batches; after configuring real transactional email in a
   *controlled staging environment*, queue invitations only with approval:

   ```bash
   DATABASE_URL="$STAGING_DATABASE_URL" go run ./cmd/import-members invite --limit 50
   DATABASE_URL="$STAGING_DATABASE_URL" go run ./cmd/import-members invite \
     --apply --limit 50 --confirm-database hive_staging --app-url https://YOUR-STAGING-URL
   ```

Imported password hashes are **not** reused. Invited members must set a new
password using a time-limited, single-use reset link. Imports are idempotent and
an existing email is not silently merged into a different account. Import reports
contain only row numbers and counts, never identifying personal information.

## Go-live decision gate

Requires owner approval, verified staging test invitations, backup/restore drill,
data-integrity reconciliation, legal/privacy review and a deliberate domain cutover
plan. Do **not** shut down or delete the previous live database just because the
new code has no dependency on it.
