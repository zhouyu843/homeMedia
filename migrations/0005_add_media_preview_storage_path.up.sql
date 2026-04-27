ALTER TABLE media_assets
ADD COLUMN IF NOT EXISTS preview_storage_path TEXT;
