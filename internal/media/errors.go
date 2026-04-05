package media

import "errors"

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrNotFound             = errors.New("media asset not found")
	ErrFileMissing          = errors.New("media file missing")
	ErrInvalidStoragePath   = errors.New("invalid storage path")
	ErrDuplicateContentHash = errors.New("media asset with same content already exists")
	ErrThumbnailGeneration  = errors.New("thumbnail generation failed")
)
