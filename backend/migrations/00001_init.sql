-- +goose Up
-- Baseline schema. Every statement is idempotent so databases that were
-- initialised manually from the legacy 001_init.sql upgrade cleanly.
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), email text NOT NULL UNIQUE, password_hash text NOT NULL,
 first_name text NOT NULL, display_name text NOT NULL DEFAULT '', phone text, area text NOT NULL DEFAULT '', interests text[] NOT NULL DEFAULT '{}',
 avatar_path text, email_updates boolean NOT NULL DEFAULT false, status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','approved','declined')),
 is_admin boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS sessions(id_hash text PRIMARY KEY, user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE, expires_at timestamptz NOT NULL);
CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS gatherings(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),title text NOT NULL,type text NOT NULL DEFAULT 'community',description text NOT NULL DEFAULT '',starts_on date NOT NULL,starts_at time NOT NULL,place text NOT NULL,capacity int NOT NULL CHECK(capacity>0),created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS reservations(id uuid PRIMARY KEY DEFAULT gen_random_uuid(), gathering_id uuid NOT NULL REFERENCES gatherings(id) ON DELETE CASCADE,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,status text NOT NULL CHECK(status IN ('confirmed','waitlist')),created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(gathering_id,user_id));
CREATE INDEX IF NOT EXISTS reservations_queue_idx ON reservations(gathering_id,status,created_at,id);
CREATE TABLE IF NOT EXISTS attendance(gathering_id uuid NOT NULL REFERENCES gatherings(id) ON DELETE CASCADE,user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,attended boolean NOT NULL DEFAULT false,checked_at timestamptz,PRIMARY KEY(gathering_id,user_id));
CREATE TABLE IF NOT EXISTS announcements(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),title text NOT NULL,body text NOT NULL,published boolean NOT NULL DEFAULT true,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS partners(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),name text NOT NULL,description text NOT NULL DEFAULT '',logo_url text NOT NULL DEFAULT '',website_url text NOT NULL DEFAULT '',published boolean NOT NULL DEFAULT true,created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS member_reports(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),reporter_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,reported_member_id uuid REFERENCES users(id) ON DELETE SET NULL,reason text NOT NULL,status text NOT NULL DEFAULT 'open',created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS member_blocks(blocker_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,blocked_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,created_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(blocker_id,blocked_id),CHECK(blocker_id<>blocked_id));
CREATE TABLE IF NOT EXISTS founding_signups(id uuid PRIMARY KEY DEFAULT gen_random_uuid(),name text NOT NULL,email text NOT NULL,phone text,why text,lang text NOT NULL DEFAULT 'en',created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS audit_events(id bigserial PRIMARY KEY,actor_id uuid,action text NOT NULL,created_at timestamptz NOT NULL DEFAULT now());

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN RAISE EXCEPTION '00001_init is irreversible: restore from backup instead'; END $$;
-- +goose StatementEnd
