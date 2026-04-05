ALTER TABLE media_assets
ADD COLUMN IF NOT EXISTS content_hash TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_media_assets_content_hash
ON media_assets (content_hash)
WHERE content_hash IS NOT NULL;