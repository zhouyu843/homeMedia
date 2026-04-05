ALTER TABLE media_assets
ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

DROP INDEX IF EXISTS uq_media_assets_content_hash;

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_assets_content_hash
ON media_assets (content_hash)
WHERE content_hash IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_media_assets_deleted_at
ON media_assets (deleted_at DESC)
WHERE deleted_at IS NOT NULL;