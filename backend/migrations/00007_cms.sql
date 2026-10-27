-- +goose Up
-- Website Studio: per-page draft and published copy, a history of published
-- versions, and a small image library. Media bytes are re-encoded (metadata
-- stripped) before they are stored here, so they need no object storage.
CREATE TABLE IF NOT EXISTS cms_pages(
 slug text PRIMARY KEY,
 draft jsonb NOT NULL DEFAULT '{}',
 published jsonb NOT NULL DEFAULT '{}',
 draft_revision bigint NOT NULL DEFAULT 0,
 published_revision bigint NOT NULL DEFAULT 0,
 updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 published_at timestamptz);
CREATE TABLE IF NOT EXISTS cms_versions(
 page_slug text NOT NULL REFERENCES cms_pages(slug) ON DELETE CASCADE,
 version bigint NOT NULL,
 snapshot jsonb NOT NULL,
 published_by uuid REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(page_slug, version));
CREATE TABLE IF NOT EXISTS cms_media(
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 filename text NOT NULL,
 alt_text text NOT NULL DEFAULT '',
 content_type text NOT NULL CHECK (content_type = 'image/jpeg'),
 data bytea NOT NULL,
 byte_size int NOT NULL,
 width int NOT NULL,
 height int NOT NULL,
 uploaded_by uuid REFERENCES users(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now());

-- +goose Down
DROP TABLE IF EXISTS cms_media;
DROP TABLE IF EXISTS cms_versions;
DROP TABLE IF EXISTS cms_pages;
