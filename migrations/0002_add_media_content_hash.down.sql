DROP INDEX IF EXISTS uq_media_assets_content_hash;

ALTER TABLE media_assets
DROP COLUMN IF EXISTS content_hash;