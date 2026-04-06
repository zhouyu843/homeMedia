package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"sort"
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

func (s Service) Upload(ctx context.Context, input UploadInput) (UploadResult, error) {
	mediaType, err := mediaTypeFromMIME(input.MIMEType)
	if err != nil {
		return UploadResult{}, err
	}

	tempFile, contentHash, sizeBytes, err := createUploadTempFile(input.Reader)
	if err != nil {
		return UploadResult{}, err
	}
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
	}()

	existingAsset, err := s.repository.FindByContentHash(ctx, contentHash)
	if err == nil {
		return UploadResult{Asset: existingAsset, Existing: true}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}

	trashedAsset, err := s.repository.FindDeletedByContentHash(ctx, contentHash)
	if err == nil {
		switch input.DuplicateAction {
		case DuplicateActionRestore:
			if err := s.repository.Restore(ctx, trashedAsset.ID); err != nil {
				return UploadResult{}, err
			}
			restoredAsset := trashedAsset
			restoredAsset.DeletedAt = nil
			return UploadResult{Asset: restoredAsset, Restored: true}, nil
		case DuplicateActionPrompt:
			return UploadResult{RequiresDecision: true, DecisionAsset: trashedAsset}, nil
		}
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		return UploadResult{}, err
	}

	legacyAsset, err := s.findLegacyAssetByContentHash(ctx, contentHash, sizeBytes)
	if err != nil {
		return UploadResult{}, err
	}
	if legacyAsset.ID != "" {
		return UploadResult{Asset: legacyAsset, Existing: true}, nil
	}

	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return UploadResult{}, fmt.Errorf("rewind upload temp file: %w", err)
	}

	storedFile, err := s.fileStore.Save(ctx, input.OriginalFilename, tempFile)
	if err != nil {
		return UploadResult{}, err
	}

	asset := Asset{
		ID:               uuid.NewString(),
		OriginalFilename: input.OriginalFilename,
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        mediaType,
		MIMEType:         input.MIMEType,
		SizeBytes:        sizeBytes,
		ContentHash:      contentHash,
		StoragePath:      storedFile.StoragePath,
		CreatedAt:        s.now().UTC(),
	}

	savedAsset, err := s.repository.Save(ctx, asset)
	if err != nil {
		_ = s.fileStore.Delete(storedFile.StoragePath)
		if errors.Is(err, ErrDuplicateContentHash) {
			existingAsset, findErr := s.repository.FindByContentHash(ctx, contentHash)
			if findErr == nil {
				return UploadResult{Asset: existingAsset, Existing: true}, nil
			}
			if !errors.Is(findErr, ErrNotFound) {
				return UploadResult{}, findErr
			}
		}
		return UploadResult{}, err
	}

	return UploadResult{Asset: savedAsset, Created: true}, nil
}

func (s Service) List(ctx context.Context) ([]Asset, error) {
	return s.repository.ListRecent(ctx)
}

func (s Service) ListTrash(ctx context.Context) ([]Asset, error) {
	return s.repository.ListTrash(ctx)
}

func (s Service) Get(ctx context.Context, id string) (Asset, error) {
	return s.repository.FindByID(ctx, id)
}

func (s Service) Download(ctx context.Context, id string) (Asset, io.ReadSeekCloser, error) {
	asset, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return Asset{}, nil, err
	}

	return s.openAssetFile(asset)
}

func (s Service) Delete(ctx context.Context, id string) error {
	if _, err := s.repository.FindByID(ctx, id); err != nil {
		return err
	}

	deletedAt := s.now().UTC()
	return s.repository.SoftDelete(ctx, id, deletedAt)
}

func (s Service) Restore(ctx context.Context, id string) error {
	if _, err := s.repository.FindDeletedByID(ctx, id); err != nil {
		return err
	}

	return s.repository.Restore(ctx, id)
}

func (s Service) DeletePermanently(ctx context.Context, id string) error {
	asset, err := s.repository.FindDeletedByID(ctx, id)
	if err != nil {
		return err
	}

	storagePathRefCount, err := s.repository.CountByStoragePath(ctx, asset.StoragePath)
	if err != nil {
		return err
	}

	if err := s.repository.Delete(ctx, id); err != nil {
		return err
	}

	if storagePathRefCount > 0 {
		return nil
	}

	if err := s.fileStore.Delete(asset.StoragePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func (s Service) EmptyTrash(ctx context.Context) error {
	assets, err := s.repository.ListTrash(ctx)
	if err != nil {
		return err
	}

	for _, asset := range assets {
		if err := s.DeletePermanently(ctx, asset.ID); err != nil {
			return err
		}
	}

	return nil
}

func (s Service) Thumbnail(ctx context.Context, id string) (string, []byte, error) {
	asset, file, err := s.Download(ctx, id)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	return s.generateThumbnail(ctx, file, asset.MediaType)
}

func (s Service) TrashThumbnail(ctx context.Context, id string) (string, []byte, error) {
	asset, err := s.repository.FindDeletedByID(ctx, id)
	if err != nil {
		return "", nil, err
	}

	_, file, err := s.openAssetFile(asset)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	return s.generateThumbnail(ctx, file, asset.MediaType)
}

func (s Service) openAssetFile(asset Asset) (Asset, io.ReadSeekCloser, error) {
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

func (s Service) generateThumbnail(ctx context.Context, file io.Reader, mediaType MediaType) (string, []byte, error) {
	thumb, err := generateThumbnailWithFFmpeg(ctx, file, mediaType)
	if err != nil {
		return "", nil, err
	}

	return "image/jpeg", thumb, nil
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

func AllowedUploadMIMETypes() []string {
	mimeTypes := make([]string, 0, len(allowedImageMIMETypes)+len(allowedVideoMIMETypes))
	for mimeType := range allowedImageMIMETypes {
		mimeTypes = append(mimeTypes, mimeType)
	}
	for mimeType := range allowedVideoMIMETypes {
		mimeTypes = append(mimeTypes, mimeType)
	}
	sort.Strings(mimeTypes)
	return mimeTypes
}

func generateThumbnailWithFFmpeg(ctx context.Context, source io.Reader, mediaType MediaType) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	filter := "scale=360:-1"
	if mediaType == MediaTypeVideo {
		filter = "thumbnail,scale=360:-1"
	}

	cmd := exec.CommandContext(
		tctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-vf", filter,
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"pipe:1",
	)

	cmd.Stdin = source
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(errOut.String()) != "" {
			return nil, fmt.Errorf("%w: %s", ErrThumbnailGeneration, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: %v", ErrThumbnailGeneration, err)
	}

	if out.Len() == 0 {
		return nil, ErrThumbnailGeneration
	}

	return out.Bytes(), nil
}

func createUploadTempFile(source io.Reader) (*os.File, string, int64, error) {
	tempFile, err := os.CreateTemp("", "home-media-upload-*")
	if err != nil {
		return nil, "", 0, fmt.Errorf("create upload temp file: %w", err)
	}

	hasher := sha256.New()
	sizeBytes, err := io.Copy(io.MultiWriter(tempFile, hasher), source)
	if err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return nil, "", 0, fmt.Errorf("buffer upload file: %w", err)
	}

	return tempFile, hex.EncodeToString(hasher.Sum(nil)), sizeBytes, nil
}

func (s Service) findLegacyAssetByContentHash(ctx context.Context, contentHash string, sizeBytes int64) (Asset, error) {
	candidates, err := s.repository.FindWithoutContentHashBySize(ctx, sizeBytes)
	if err != nil {
		return Asset{}, err
	}

	for _, candidate := range candidates {
		file, err := s.fileStore.Open(candidate.StoragePath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || errors.Is(err, ErrInvalidStoragePath) {
				continue
			}
			return Asset{}, err
		}

		candidateHash, hashErr := hashReadSeeker(file)
		_ = file.Close()
		if hashErr != nil {
			return Asset{}, hashErr
		}

		if updateErr := s.repository.UpdateContentHash(ctx, candidate.ID, candidateHash); updateErr != nil && !errors.Is(updateErr, ErrDuplicateContentHash) {
			return Asset{}, updateErr
		}

		if candidateHash == contentHash {
			candidate.ContentHash = candidateHash
			return candidate, nil
		}
	}

	return Asset{}, nil
}

func hashReadSeeker(reader io.Reader) (string, error) {
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return "", fmt.Errorf("hash stored file: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
