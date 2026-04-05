package media

import (
	"io"
	"time"
)

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

type Asset struct {
	ID               string
	OriginalFilename string
	StoredFilename   string
	MediaType        MediaType
	MIMEType         string
	SizeBytes        int64
	ContentHash      string
	StoragePath      string
	CreatedAt        time.Time
}

type UploadResult struct {
	Asset    Asset
	Created  bool
	Existing bool
}

type UploadInput struct {
	OriginalFilename string
	MIMEType         string
	SizeBytes        int64
	Reader           io.Reader
}

type StoredFile struct {
	StoredFilename string
	StoragePath    string
	SizeBytes      int64
}
