CREATE TABLE IF NOT EXISTS media_assets (
    id UUID PRIMARY KEY,
    original_filename TEXT NOT NULL,
    stored_filename TEXT NOT NULL,
    media_type TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    size_bytes BIGINT NOT NULL,
    storage_path TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_assets_created_at ON media_assets (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_media_assets_media_type ON media_assets (media_type);
