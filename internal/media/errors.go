package media

import "errors"

var (
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	ErrNotFound             = errors.New("media asset not found")
	ErrFileMissing          = errors.New("media file missing")
	ErrInvalidStoragePath   = errors.New("invalid storage path")
)
