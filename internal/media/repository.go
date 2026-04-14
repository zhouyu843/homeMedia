package media

import (
	"context"
	"io"
	"time"
)

type Repository interface {
	Save(ctx context.Context, asset Asset) (Asset, error)
	FindByID(ctx context.Context, id string) (Asset, error)
	FindDeletedByID(ctx context.Context, id string) (Asset, error)
	FindByContentHash(ctx context.Context, contentHash string) (Asset, error)
	FindDeletedByContentHash(ctx context.Context, contentHash string) (Asset, error)
	FindWithoutContentHashBySize(ctx context.Context, sizeBytes int64) ([]Asset, error)
	UpdateContentHash(ctx context.Context, id string, contentHash string) error
	UpdateThumbnailStoragePath(ctx context.Context, id string, thumbnailStoragePath string) error
	ListRecent(ctx context.Context) ([]Asset, error)
	ListTrash(ctx context.Context) ([]Asset, error)
	SoftDelete(ctx context.Context, id string, deletedAt time.Time) error
	Restore(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
	CountByStoragePath(ctx context.Context, storagePath string) (int, error)
}

type FileStore interface {
	Save(ctx context.Context, originalFilename string, source io.Reader) (StoredFile, error)
	SaveThumbnail(ctx context.Context, assetID string, source io.Reader) (StoredFile, error)
	Open(storagePath string) (io.ReadSeekCloser, error)
	Delete(storagePath string) error
}
