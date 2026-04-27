package media

import (
	"io"
	"time"
)

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
	MediaTypePDF   MediaType = "pdf"
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
	ThumbnailStoragePath string
	PreviewStoragePath   string
	CreatedAt        time.Time
	DeletedAt        *time.Time
}

type PlaybackWarning struct {
	Code    string
	Message string
}

type UploadResult struct {
	Asset            Asset
	Created          bool
	Existing         bool
	Restored         bool
	RequiresDecision bool
	DecisionAsset    Asset
}

type UploadInput struct {
	OriginalFilename string
	MIMEType         string
	SizeBytes        int64
	Reader           io.Reader
	DuplicateAction  DuplicateAction
}

type DuplicateAction string

const (
	DuplicateActionPrompt  DuplicateAction = ""
	DuplicateActionRestore DuplicateAction = "restore"
	DuplicateActionNew     DuplicateAction = "new"
)

type StoredFile struct {
	StoredFilename string
	StoragePath    string
	SizeBytes      int64
}
