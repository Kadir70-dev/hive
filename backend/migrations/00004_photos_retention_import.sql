-- +goose Up
-- Profile photo cleanup queue, audit retention support, legacy member import
-- mapping and safety-conflict decisions. Additive and idempotent.

-- Objects to delete from R2. Rows are written in the same transaction that
-- replaces or removes a photo (or deletes an account), then a background job
-- deletes the object and the row, retrying on failure. No foreign key to
-- users: the row must outlive the account it belonged to.
CREATE TABLE IF NOT EXISTS storage_deletions (
  object_key text PRIMARY KEY,
  attempts int NOT NULL DEFAULT 0,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS audit_events_created_idx ON audit_events(created_at);

-- Legacy (Supabase) import bookkeeping: re-running an import never creates
-- duplicates. Only opaque legacy ids are stored, no personal data.
CREATE TABLE IF NOT EXISTS legacy_member_map (
  legacy_id text PRIMARY KEY,
  user_id uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
  imported_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS legacy_gathering_map (
  legacy_id text PRIMARY KEY,
  gathering_id uuid NOT NULL UNIQUE REFERENCES gatherings(id) ON DELETE CASCADE,
  imported_at timestamptz NOT NULL DEFAULT now()
);
-- Imported accounts have no working password until the member completes the
-- invitation (a password-reset link).
ALTER TABLE users ADD COLUMN IF NOT EXISTS imported_at timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS invited_at timestamptz;

-- Admin decisions on safety conflicts (a block applied after both members had
-- already booked the same gathering).
CREATE TABLE IF NOT EXISTS safety_conflict_decisions (
  gathering_id uuid NOT NULL REFERENCES gatherings(id) ON DELETE CASCADE,
  report_id uuid NOT NULL REFERENCES member_reports(id) ON DELETE CASCADE,
  decision text NOT NULL CHECK (decision IN ('acknowledged','released_reporter','released_reported')),
  note text NOT NULL DEFAULT '',
  decided_by uuid REFERENCES users(id) ON DELETE SET NULL,
  decided_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (gathering_id, report_id)
);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '00004_photos_retention_import is irreversible: restore from backup instead'; END $$;
-- +goose StatementEnd
