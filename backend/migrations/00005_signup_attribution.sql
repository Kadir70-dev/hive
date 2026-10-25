-- +goose Up
-- First-party campaign attribution linked only to actual new accounts.
-- A conversion is counted only after the email address is verified.
-- No IP address, raw referrer, email address, or third-party analytics identifier is stored here.
CREATE TABLE IF NOT EXISTS signup_attribution (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  utm_source text NOT NULL DEFAULT '',
  utm_medium text NOT NULL DEFAULT '',
  utm_campaign text NOT NULL DEFAULT '',
  utm_content text NOT NULL DEFAULT '',
  utm_term text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  verified_at timestamptz
);
CREATE INDEX IF NOT EXISTS signup_attribution_verified_idx ON signup_attribution(verified_at) WHERE verified_at IS NOT NULL;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '00005_signup_attribution is irreversible: restore from backup instead'; END $$;
-- +goose StatementEnd
