package media

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	fileStore  FileStore
	now        func() time.Time
}

func NewService(repository Repository, fileStore FileStore) Service {
	return Service{
		repository: repository,
		fileStore:  fileStore,
		now:        time.Now,
	}
}

func (s Service) Upload(ctx context.Context, input UploadInput) (Asset, error) {
	mediaType, err := mediaTypeFromMIME(input.MIMEType)
	if err != nil {
		return Asset{}, err
	}

	storedFile, err := s.fileStore.Save(ctx, input.OriginalFilename, input.Reader)
	if err != nil {
		return Asset{}, err
	}

	asset := Asset{
		ID:               uuid.NewString(),
		OriginalFilename: input.OriginalFilename,
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        mediaType,
		MIMEType:         input.MIMEType,
		SizeBytes:        storedFile.SizeBytes,
		StoragePath:      storedFile.StoragePath,
		CreatedAt:        s.now().UTC(),
	}

	savedAsset, err := s.repository.Save(ctx, asset)
	if err != nil {
		_ = s.fileStore.Delete(storedFile.StoragePath)
		return Asset{}, err
	}

	return savedAsset, nil
}

func (s Service) List(ctx context.Context) ([]Asset, error) {
	return s.repository.ListRecent(ctx)
}

func (s Service) Get(ctx context.Context, id string) (Asset, error) {
	return s.repository.FindByID(ctx, id)
}

func (s Service) Download(ctx context.Context, id string) (Asset, io.ReadSeekCloser, error) {
	asset, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return Asset{}, nil, err
	}

	file, err := s.fileStore.Open(asset.StoragePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Asset{}, nil, ErrFileMissing
		}
		if errors.Is(err, ErrInvalidStoragePath) {
			return Asset{}, nil, err
		}
		return Asset{}, nil, err
	}

	return asset, file, nil
}

func mediaTypeFromMIME(mimeType string) (MediaType, error) {
	if mimeType == "" {
		return "", ErrUnsupportedMediaType
	}

	normalized := strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if _, ok := allowedImageMIMETypes[normalized]; ok {
		return MediaTypeImage, nil
	}
	if _, ok := allowedVideoMIMETypes[normalized]; ok {
		return MediaTypeVideo, nil
	}

	return "", ErrUnsupportedMediaType
}

var allowedImageMIMETypes = map[string]struct{}{
	"image/gif":  {},
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var allowedVideoMIMETypes = map[string]struct{}{
	"video/mp4":        {},
	"video/quicktime":  {},
	"video/webm":       {},
	"video/x-matroska": {},
}
