-- +goose Up
-- Announcements management + email delivery, admin-mediated safety reports,
-- partner editing. Additive and idempotent.

ALTER TABLE announcements ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE announcements ADD COLUMN IF NOT EXISTS published_at timestamptz;
UPDATE announcements SET published_at = created_at WHERE published AND published_at IS NULL;
CREATE INDEX IF NOT EXISTS announcements_feed_idx ON announcements(published, published_at DESC);

-- One row per (announcement, member) email: the primary key makes repeated
-- "send" requests and retries idempotent.
CREATE TABLE IF NOT EXISTS announcement_deliveries (
  announcement_id uuid NOT NULL REFERENCES announcements(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  queued_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (announcement_id, user_id)
);

ALTER TABLE partners ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();

-- Safety reports are admin-mediated: members describe who was involved
-- instead of selecting another member, so no member identities are exposed.
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT 'other';
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS gathering_id uuid REFERENCES gatherings(id) ON DELETE SET NULL;
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS subject_description text NOT NULL DEFAULT '';
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS block_requested boolean NOT NULL DEFAULT false;
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS resolution_note text NOT NULL DEFAULT '';
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS resolved_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS resolved_at timestamptz;
ALTER TABLE member_reports ADD COLUMN IF NOT EXISTS updated_at timestamptz NOT NULL DEFAULT now();
-- +goose StatementBegin
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'member_reports_status_check') THEN
    UPDATE member_reports SET status = 'open' WHERE status NOT IN ('open','in_review','resolved','dismissed');
    ALTER TABLE member_reports ADD CONSTRAINT member_reports_status_check CHECK (status IN ('open','in_review','resolved','dismissed'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'member_reports_category_check') THEN
    ALTER TABLE member_reports ADD CONSTRAINT member_reports_category_check CHECK (category IN ('harassment','safety','inappropriate','other'));
  END IF;
END $$;
-- +goose StatementEnd
CREATE INDEX IF NOT EXISTS member_reports_status_idx ON member_reports(status, created_at DESC);
CREATE INDEX IF NOT EXISTS member_reports_reporter_idx ON member_reports(reporter_id, created_at DESC);

-- Blocks are created by admins on a member's behalf; record who and why.
ALTER TABLE member_blocks ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE member_blocks ADD COLUMN IF NOT EXISTS report_id uuid REFERENCES member_reports(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS member_blocks_blocked_idx ON member_blocks(blocked_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '00003_community is irreversible: restore from backup instead'; END $$;
-- +goose StatementEnd
