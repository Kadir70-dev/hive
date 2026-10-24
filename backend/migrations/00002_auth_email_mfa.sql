-- +goose Up
-- Email verification, password reset, admin TOTP, session metadata and the
-- transactional email outbox. Additive and idempotent.

ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed_at timestamptz;
-- TOTP secrets are AES-256-GCM encrypted with MFA_ENCRYPTION_KEY; never stored in clear.
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_secret_enc bytea;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_pending_enc bytea;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_enabled_at timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS totp_last_step bigint;

ALTER TABLE sessions ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS mfa_verified_at timestamptz;
CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);

-- Single-use, hashed, expiring tokens for email verification and password reset.
CREATE TABLE IF NOT EXISTS email_tokens (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  purpose text NOT NULL CHECK (purpose IN ('verify_email','reset_password')),
  expires_at timestamptz NOT NULL,
  used_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS email_tokens_user_idx ON email_tokens(user_id, purpose);
CREATE INDEX IF NOT EXISTS email_tokens_expiry_idx ON email_tokens(expires_at);

-- Admin MFA recovery codes (SHA-256 of high-entropy codes), single use.
CREATE TABLE IF NOT EXISTS mfa_recovery_codes (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  code_hash text NOT NULL,
  used_at timestamptz,
  PRIMARY KEY (user_id, code_hash)
);

-- Transactional outbox: rows are written in the same transaction as the
-- token they carry, then delivered by the API's worker with retries.
-- Bodies are erased once delivered or abandoned.
CREATE TABLE IF NOT EXISTS email_outbox (
  id bigserial PRIMARY KEY,
  user_id uuid REFERENCES users(id) ON DELETE CASCADE,
  template text NOT NULL,
  to_addr text NOT NULL,
  subject text NOT NULL,
  text_body text,
  html_body text,
  attempts int NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  locked_until timestamptz,
  sent_at timestamptz,
  failed_at timestamptz,
  provider_id text,
  last_error text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS email_outbox_due_idx ON email_outbox(next_attempt_at) WHERE sent_at IS NULL AND failed_at IS NULL;

ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS target_id uuid;
ALTER TABLE audit_events ADD COLUMN IF NOT EXISTS detail text;

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '00002_auth_email_mfa is irreversible: restore from backup instead'; END $$;
-- +goose StatementEnd
