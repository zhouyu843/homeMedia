package media

import (
	"context"
	"io"
)

type Repository interface {
	Save(ctx context.Context, asset Asset) (Asset, error)
	FindByID(ctx context.Context, id string) (Asset, error)
	FindByContentHash(ctx context.Context, contentHash string) (Asset, error)
	FindWithoutContentHashBySize(ctx context.Context, sizeBytes int64) ([]Asset, error)
	UpdateContentHash(ctx context.Context, id string, contentHash string) error
	ListRecent(ctx context.Context) ([]Asset, error)
}

type FileStore interface {
	Save(ctx context.Context, originalFilename string, source io.Reader) (StoredFile, error)
	Open(storagePath string) (io.ReadSeekCloser, error)
	Delete(storagePath string) error
}
