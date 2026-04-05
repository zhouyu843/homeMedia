DROP INDEX IF EXISTS idx_media_assets_deleted_at;

DROP INDEX IF EXISTS uq_media_assets_content_hash;

ALTER TABLE media_assets
DROP COLUMN IF EXISTS deleted_at;

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_assets_content_hash
ON media_assets (content_hash)
WHERE content_hash IS NOT NULL;