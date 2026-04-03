package media

import "time"

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
	StoragePath      string
	CreatedAt        time.Time
}

type UploadInput struct {
	OriginalFilename string
	MIMEType         string
	SizeBytes        int64
	Reader           Reader
}

type StoredFile struct {
	StoredFilename string
	StoragePath    string
	SizeBytes      int64
}

type Reader interface {
	Read(p []byte) (n int, err error)
}
