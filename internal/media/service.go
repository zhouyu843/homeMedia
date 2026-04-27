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
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	fileStore  FileStore
	now        func() time.Time
	thumbnailGenerator func(ctx context.Context, file io.Reader, mediaType MediaType) (string, []byte, error)
	videoCodecProbe    func(ctx context.Context, source io.Reader) (string, error)
}

func NewService(repository Repository, fileStore FileStore) Service {
	return Service{
		repository: repository,
		fileStore:  fileStore,
		now:        time.Now,
		thumbnailGenerator: generateThumbnail,
		videoCodecProbe:    probeVideoCodec,
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

	assetID := uuid.NewString()
	storedFile, err := s.fileStore.Save(ctx, input.OriginalFilename, tempFile)
	if err != nil {
		return UploadResult{}, err
	}

	thumbnailStoragePath, thumbnailErr := s.storeThumbnailFromTempFile(ctx, assetID, tempFile, mediaType)
	previewStoragePath, _ := s.storePreviewFromTempFile(ctx, assetID, tempFile, mediaType)

	asset := Asset{
		ID:               assetID,
		OriginalFilename: input.OriginalFilename,
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        mediaType,
		MIMEType:         input.MIMEType,
		SizeBytes:        sizeBytes,
		ContentHash:      contentHash,
		StoragePath:      storedFile.StoragePath,
		ThumbnailStoragePath: thumbnailStoragePath,
		PreviewStoragePath:   previewStoragePath,
		CreatedAt:        s.now().UTC(),
	}

	savedAsset, err := s.repository.Save(ctx, asset)
	if err != nil {
		_ = s.fileStore.Delete(storedFile.StoragePath)
		if thumbnailStoragePath != "" {
			_ = s.fileStore.Delete(thumbnailStoragePath)
		}
		if previewStoragePath != "" {
			_ = s.fileStore.Delete(previewStoragePath)
		}
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
	if thumbnailErr != nil {
		savedAsset.ThumbnailStoragePath = ""
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

func (s Service) PlaybackWarning(ctx context.Context, asset Asset) *PlaybackWarning {
	if asset.MediaType != MediaTypeVideo {
		return nil
	}

	file, err := s.fileStore.Open(asset.StoragePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	codecName, err := s.videoCodecProbe(ctx, file)
	if err != nil {
		return nil
	}

	if !isPotentiallyUnsupportedBrowserCodec(codecName) {
		return nil
	}

	return &PlaybackWarning{
		Code:    "hevc_browser_compatibility",
		Message: "检测到 HEVC/H.265 视频编码。部分 Linux Chrome 浏览器可能只有声音没有画面；若播放异常，请改用 Firefox、Safari 或下载后本地播放。",
	}
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

	if asset.ThumbnailStoragePath != "" {
		if err := s.fileStore.Delete(asset.ThumbnailStoragePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}

	if asset.PreviewStoragePath != "" {
		if err := s.fileStore.Delete(asset.PreviewStoragePath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
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
	asset, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return "", nil, err
	}

	return s.thumbnailForAsset(ctx, asset)
}

func (s Service) TrashThumbnail(ctx context.Context, id string) (string, []byte, error) {
	asset, err := s.repository.FindDeletedByID(ctx, id)
	if err != nil {
		return "", nil, err
	}

	return s.thumbnailForAsset(ctx, asset)
}

func (s Service) thumbnailForAsset(ctx context.Context, asset Asset) (string, []byte, error) {
	if asset.ThumbnailStoragePath != "" {
		file, err := s.fileStore.Open(asset.ThumbnailStoragePath)
		if err == nil {
			defer file.Close()
			thumbnail, readErr := io.ReadAll(file)
			if readErr == nil && len(thumbnail) > 0 {
				return "image/jpeg", thumbnail, nil
			}
		}
	}

	_, file, err := s.openAssetFile(asset)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	contentType, thumbnail, err := s.thumbnailGenerator(ctx, file, asset.MediaType)
	if err != nil {
		return "", nil, err
	}

	thumbnailStoragePath, persistErr := s.persistThumbnail(ctx, asset.ID, thumbnail)
	if persistErr == nil && thumbnailStoragePath != "" {
		asset.ThumbnailStoragePath = thumbnailStoragePath
		_ = s.repository.UpdateThumbnailStoragePath(ctx, asset.ID, thumbnailStoragePath)
	}

	return contentType, thumbnail, nil
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

func generateThumbnail(ctx context.Context, file io.Reader, mediaType MediaType) (string, []byte, error) {
	var (
		thumb []byte
		err   error
	)
	if mediaType == MediaTypePDF {
		thumb, err = generateThumbnailWithPdftoppm(ctx, file)
	} else {
		thumb, err = generateThumbnailWithFFmpeg(ctx, file, mediaType)
	}
	if err != nil {
		return "", nil, err
	}

	return "image/jpeg", thumb, nil
}

func (s Service) storeThumbnailFromTempFile(ctx context.Context, assetID string, tempFile *os.File, mediaType MediaType) (string, error) {
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind upload temp file for thumbnail: %w", err)
	}

	_, thumbnail, err := s.thumbnailGenerator(ctx, tempFile, mediaType)
	if err != nil {
		return "", err
	}

	return s.persistThumbnail(ctx, assetID, thumbnail)
}

func (s Service) persistThumbnail(ctx context.Context, assetID string, thumbnail []byte) (string, error) {
	if len(thumbnail) == 0 {
		return "", ErrThumbnailGeneration
	}

	storedThumbnail, err := s.fileStore.SaveThumbnail(ctx, assetID, bytes.NewReader(thumbnail))
	if err != nil {
		return "", err
	}

	return storedThumbnail.StoragePath, nil
}

// Preview opens the compressed preview file for the given asset.
// Returns ErrPreviewNotAvailable if no preview has been generated yet.
func (s Service) Preview(ctx context.Context, id string) (contentType string, file io.ReadSeekCloser, createdAt time.Time, err error) {
	asset, findErr := s.repository.FindByID(ctx, id)
	if findErr != nil {
		return "", nil, time.Time{}, findErr
	}

	if asset.PreviewStoragePath == "" {
		return "", nil, time.Time{}, ErrPreviewNotAvailable
	}

	f, openErr := s.fileStore.Open(asset.PreviewStoragePath)
	if openErr != nil {
		return "", nil, time.Time{}, ErrPreviewNotAvailable
	}

	ct := previewContentType(asset.MediaType)
	return ct, f, asset.CreatedAt, nil
}

func previewContentType(mediaType MediaType) string {
	if mediaType == MediaTypeVideo {
		return "video/mp4"
	}
	return "image/jpeg"
}

func previewExtension(mediaType MediaType) string {
	if mediaType == MediaTypeVideo {
		return ".mp4"
	}
	return ".jpg"
}

func generatePreview(ctx context.Context, inputPath string, mediaType MediaType) ([]byte, error) {
	if mediaType == MediaTypeImage {
		return generateImagePreview(ctx, inputPath)
	}
	if mediaType == MediaTypeVideo {
		return generateVideoPreview(ctx, inputPath)
	}
	return nil, ErrPreviewGeneration
}

func generateImagePreview(ctx context.Context, inputPath string) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		tctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-vf", "scale=1280:-1",
		"-frames:v", "1",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)

	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(errOut.String()) != "" {
			return nil, fmt.Errorf("%w: %s", ErrPreviewGeneration, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: %v", ErrPreviewGeneration, err)
	}

	if out.Len() == 0 {
		return nil, ErrPreviewGeneration
	}

	return out.Bytes(), nil
}

func generateVideoPreview(ctx context.Context, inputPath string) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	outputFile, err := os.CreateTemp("", "home-media-preview-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("%w: create preview temp file: %v", ErrPreviewGeneration, err)
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer func() { _ = os.Remove(outputPath) }()

	cmd := exec.CommandContext(
		tctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-y",
		outputPath,
	)

	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(errOut.String()) != "" {
			return nil, fmt.Errorf("%w: %s", ErrPreviewGeneration, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: %v", ErrPreviewGeneration, err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read preview file: %v", ErrPreviewGeneration, err)
	}
	if len(data) == 0 {
		return nil, ErrPreviewGeneration
	}

	return data, nil
}

func (s Service) storePreviewFromTempFile(ctx context.Context, assetID string, tempFile *os.File, mediaType MediaType) (string, error) {
	if mediaType == MediaTypePDF {
		return "", ErrPreviewGeneration
	}

	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("rewind upload temp file for preview: %w", err)
	}

	inputPath, cleanup, err := writeThumbnailTempFile(tempFile, "home-media-preview-src-*")
	if err != nil {
		return "", err
	}
	defer cleanup()

	data, err := generatePreview(ctx, inputPath, mediaType)
	if err != nil {
		return "", err
	}

	ext := previewExtension(mediaType)
	stored, err := s.fileStore.SavePreview(ctx, assetID, ext, bytes.NewReader(data))
	if err != nil {
		return "", err
	}

	return stored.StoragePath, nil
}

func (s Service) ListWithoutPreview(ctx context.Context) ([]Asset, error) {
	return s.repository.ListWithoutPreview(ctx)
}

// BackfillPreview generates and persists a preview for an existing asset.
// It reads the original file from storage to generate the preview.
func (s Service) BackfillPreview(ctx context.Context, id string) error {
	asset, err := s.repository.FindByID(ctx, id)
	if err != nil {
		return err
	}

	if asset.MediaType == MediaTypePDF {
		return ErrPreviewGeneration
	}

	_, originalFile, err := s.openAssetFile(asset)
	if err != nil {
		return err
	}
	defer originalFile.Close()

	inputPath, cleanup, err := writeThumbnailTempFile(originalFile, "home-media-preview-backfill-*")
	if err != nil {
		return err
	}
	defer cleanup()

	data, err := generatePreview(ctx, inputPath, asset.MediaType)
	if err != nil {
		return err
	}

	ext := previewExtension(asset.MediaType)
	stored, err := s.fileStore.SavePreview(ctx, asset.ID, ext, bytes.NewReader(data))
	if err != nil {
		return err
	}

	return s.repository.UpdatePreviewStoragePath(ctx, asset.ID, stored.StoragePath)
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
	if _, ok := allowedPDFMIMETypes[normalized]; ok {
		return MediaTypePDF, nil
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

var allowedPDFMIMETypes = map[string]struct{}{
	"application/pdf": {},
}

func AllowedUploadMIMETypes() []string {
	mimeTypes := make([]string, 0, len(allowedImageMIMETypes)+len(allowedVideoMIMETypes)+len(allowedPDFMIMETypes))
	for mimeType := range allowedImageMIMETypes {
		mimeTypes = append(mimeTypes, mimeType)
	}
	for mimeType := range allowedVideoMIMETypes {
		mimeTypes = append(mimeTypes, mimeType)
	}
	for mimeType := range allowedPDFMIMETypes {
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
		inputPath, cleanup, err := writeThumbnailTempFile(source, "home-media-video-thumb-*")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		filter = "thumbnail,scale=360:-1"
		cmd := exec.CommandContext(
			tctx,
			"ffmpeg",
			"-hide_banner",
			"-loglevel", "error",
			"-i", inputPath,
			"-vf", filter,
			"-frames:v", "1",
			"-f", "image2pipe",
			"-vcodec", "mjpeg",
			"pipe:1",
		)

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

func writeThumbnailTempFile(source io.Reader, pattern string) (string, func(), error) {
	inputFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", nil, fmt.Errorf("%w: create temp input file: %v", ErrThumbnailGeneration, err)
	}

	if _, err := io.Copy(inputFile, source); err != nil {
		inputPath := inputFile.Name()
		_ = inputFile.Close()
		_ = os.Remove(inputPath)
		return "", nil, fmt.Errorf("%w: buffer temp input file: %v", ErrThumbnailGeneration, err)
	}

	if err := inputFile.Close(); err != nil {
		inputPath := inputFile.Name()
		_ = os.Remove(inputPath)
		return "", nil, fmt.Errorf("%w: close temp input file: %v", ErrThumbnailGeneration, err)
	}

	inputPath := inputFile.Name()
	cleanup := func() {
		_ = os.Remove(inputPath)
	}

	return inputPath, cleanup, nil
}

func generateThumbnailWithPdftoppm(ctx context.Context, source io.Reader) ([]byte, error) {
	tctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	inputFile, err := os.CreateTemp("", "home-media-pdf-thumb-*.pdf")
	if err != nil {
		return nil, fmt.Errorf("%w: create pdf temp file: %v", ErrThumbnailGeneration, err)
	}
	inputPath := inputFile.Name()
	defer func() {
		_ = inputFile.Close()
		_ = os.Remove(inputPath)
	}()

	if _, err := io.Copy(inputFile, source); err != nil {
		return nil, fmt.Errorf("%w: buffer pdf source: %v", ErrThumbnailGeneration, err)
	}
	if err := inputFile.Close(); err != nil {
		return nil, fmt.Errorf("%w: close pdf temp file: %v", ErrThumbnailGeneration, err)
	}

	outputDir, err := os.MkdirTemp("", "home-media-pdf-thumb-out-*")
	if err != nil {
		return nil, fmt.Errorf("%w: create pdf output dir: %v", ErrThumbnailGeneration, err)
	}
	defer func() {
		_ = os.RemoveAll(outputDir)
	}()

	outputPrefix := filepath.Join(outputDir, "page")
	cmd := exec.CommandContext(
		tctx,
		"pdftoppm",
		"-jpeg",
		"-f", "1",
		"-l", "1",
		"-singlefile",
		inputPath,
		outputPrefix,
	)

	var errOut bytes.Buffer
	cmd.Stderr = &errOut

	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(errOut.String()) != "" {
			return nil, fmt.Errorf("%w: %s", ErrThumbnailGeneration, strings.TrimSpace(errOut.String()))
		}
		return nil, fmt.Errorf("%w: %v", ErrThumbnailGeneration, err)
	}

	thumbnailPath := outputPrefix + ".jpg"
	thumbnail, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return nil, fmt.Errorf("%w: read pdf thumbnail: %v", ErrThumbnailGeneration, err)
	}
	if len(thumbnail) == 0 {
		return nil, ErrThumbnailGeneration
	}

	return thumbnail, nil
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

func probeVideoCodec(ctx context.Context, source io.Reader) (string, error) {
	tempFile, err := os.CreateTemp("", "home-media-video-probe-*")
	if err != nil {
		return "", fmt.Errorf("create probe temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}()

	if _, err := io.Copy(tempFile, source); err != nil {
		return "", fmt.Errorf("buffer video for probe: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return "", fmt.Errorf("close probe temp file: %w", err)
	}

	tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		tctx,
		"ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		tempPath,
	)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if strings.TrimSpace(stderr.String()) != "" {
			return "", fmt.Errorf("probe video codec: %s", strings.TrimSpace(stderr.String()))
		}
		return "", fmt.Errorf("probe video codec: %w", err)
	}

	codecName := strings.ToLower(strings.TrimSpace(stdout.String()))
	if codecName == "" {
		return "", errors.New("probe video codec: empty codec name")
	}

	return codecName, nil
}

func isPotentiallyUnsupportedBrowserCodec(codecName string) bool {
	normalized := strings.ToLower(strings.TrimSpace(codecName))
	return normalized == "hevc" || normalized == "h265" || normalized == "h.265"
}
