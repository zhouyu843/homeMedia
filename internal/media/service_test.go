package media

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestServiceUploadStoresMetadata(t *testing.T) {
	repo := &fakeRepository{}
	store := &fakeFileStore{
		storedFile: StoredFile{
			StoredFilename: "stored.jpg",
			StoragePath:    "20260403/stored.jpg",
			SizeBytes:      4,
		},
	}
	service := NewService(repo, store)
	service.now = func() time.Time {
		return time.Date(2026, 4, 3, 12, 0, 0, 0, time.UTC)
	}

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "photo.jpg",
		MIMEType:         "image/jpeg",
		SizeBytes:        4,
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if !result.Created || result.Existing {
		t.Fatalf("expected created upload result, got %+v", result)
	}
	asset := result.Asset

	if asset.OriginalFilename != "photo.jpg" {
		t.Fatalf("expected original filename to be preserved, got %q", asset.OriginalFilename)
	}
	if asset.MediaType != MediaTypeImage {
		t.Fatalf("expected image media type, got %q", asset.MediaType)
	}
	if repo.savedAsset.StoragePath != "20260403/stored.jpg" {
		t.Fatalf("expected storage path to be saved, got %q", repo.savedAsset.StoragePath)
	}
	if store.deletedPath != "" {
		t.Fatalf("did not expect stored file cleanup, got %q", store.deletedPath)
	}
}

func TestServiceUploadRejectsUnsupportedType(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeFileStore{})

	_, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "notes.txt",
		MIMEType:         "text/plain",
		Reader:           strings.NewReader("hello"),
	})
	if !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("expected ErrUnsupportedMediaType, got %v", err)
	}
}

func TestServiceUploadCleansUpStoredFileWhenSaveFails(t *testing.T) {
	repo := &fakeRepository{saveErr: errors.New("db down")}
	store := &fakeFileStore{
		storedFile: StoredFile{StoredFilename: "stored.mp4", StoragePath: "20260403/stored.mp4", SizeBytes: 8},
	}
	service := NewService(repo, store)

	_, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "clip.mp4",
		MIMEType:         "video/mp4",
		Reader:           strings.NewReader("12345678"),
	})
	if err == nil {
		t.Fatal("expected upload to fail when repository save fails")
	}
	if store.deletedPath != "20260403/stored.mp4" {
		t.Fatalf("expected cleanup to delete stored file, got %q", store.deletedPath)
	}
}

func TestServiceUploadReusesExistingAssetWithSameContentHash(t *testing.T) {
	existingAsset := Asset{
		ID:               "asset-existing",
		OriginalFilename: "photo-original.jpg",
		StoredFilename:   "stored-existing.jpg",
		MediaType:        MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        4,
		ContentHash:      "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
		StoragePath:      "20260403/stored-existing.jpg",
		CreatedAt:        time.Now().UTC(),
	}
	repo := &fakeRepository{assetByContentHash: map[string]Asset{existingAsset.ContentHash: existingAsset}}
	store := &fakeFileStore{}
	service := NewService(repo, store)

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "different-name.jpg",
		MIMEType:         "image/jpeg",
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.Created || !result.Existing {
		t.Fatalf("expected existing upload result, got %+v", result)
	}
	if result.Asset.ID != existingAsset.ID {
		t.Fatalf("expected existing asset ID %q, got %q", existingAsset.ID, result.Asset.ID)
	}
	if store.saveCalls != 0 {
		t.Fatalf("expected file store to be skipped for duplicate content, got %d save calls", store.saveCalls)
	}
}

func TestServiceUploadDeletesStoredFileWhenRepositoryReportsDuplicateContentHash(t *testing.T) {
	existingAsset := Asset{
		ID:               "asset-existing",
		OriginalFilename: "photo-original.jpg",
		StoredFilename:   "stored-existing.jpg",
		MediaType:        MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        4,
		ContentHash:      "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7",
		StoragePath:      "20260403/stored-existing.jpg",
		CreatedAt:        time.Now().UTC(),
	}
	repo := &duplicateOnSaveRepository{existingAsset: existingAsset}
	store := &fakeFileStore{
		storedFile: StoredFile{StoredFilename: "stored-new.jpg", StoragePath: "20260403/stored-new.jpg", SizeBytes: 4},
	}
	service := NewService(repo, store)

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "photo-copy.jpg",
		MIMEType:         "image/jpeg",
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.Created || !result.Existing {
		t.Fatalf("expected existing upload result, got %+v", result)
	}
	if store.deletedPath != "20260403/stored-new.jpg" {
		t.Fatalf("expected duplicate save cleanup, got %q", store.deletedPath)
	}
}

func TestServiceUploadMatchesLegacyAssetWithoutContentHash(t *testing.T) {
	legacyAsset := Asset{
		ID:               "asset-legacy",
		OriginalFilename: "old-photo.jpg",
		StoredFilename:   "stored-legacy.jpg",
		MediaType:        MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        4,
		StoragePath:      "20260403/stored-legacy.jpg",
		CreatedAt:        time.Now().UTC(),
	}
	repo := &fakeRepository{assetsWithoutContentHashBySize: map[int64][]Asset{4: {legacyAsset}}}
	store := &fakeFileStore{openFile: readSeekNopCloser{Reader: strings.NewReader("data")}}
	service := NewService(repo, store)

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "photo-copy.jpg",
		MIMEType:         "image/jpeg",
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if result.Created || !result.Existing {
		t.Fatalf("expected existing upload result, got %+v", result)
	}
	if result.Asset.ID != legacyAsset.ID {
		t.Fatalf("expected legacy asset ID %q, got %q", legacyAsset.ID, result.Asset.ID)
	}
	if store.saveCalls != 0 {
		t.Fatalf("expected file store to be skipped for matched legacy content, got %d save calls", store.saveCalls)
	}
	if got := repo.updatedContentHashes[legacyAsset.ID]; got == "" {
		t.Fatal("expected legacy asset content hash to be backfilled")
	}
}

func TestServiceDownloadReturnsFileMissing(t *testing.T) {
	repo := &fakeRepository{assetByID: map[string]Asset{
		"asset-1": {ID: "asset-1", StoragePath: "20260403/missing.jpg"},
	}}
	store := &fakeFileStore{openErr: os.ErrNotExist}
	service := NewService(repo, store)

	_, _, err := service.Download(context.Background(), "asset-1")
	if !errors.Is(err, ErrFileMissing) {
		t.Fatalf("expected ErrFileMissing, got %v", err)
	}
}

type fakeRepository struct {
	savedAsset Asset
	saveErr    error
	assetByID  map[string]Asset
	assetByContentHash map[string]Asset
	assetsWithoutContentHashBySize map[int64][]Asset
	updatedContentHashes map[string]string
	listAssets []Asset
}

func (f *fakeRepository) Save(_ context.Context, asset Asset) (Asset, error) {
	if f.saveErr != nil {
		return Asset{}, f.saveErr
	}
	f.savedAsset = asset
	if f.assetByContentHash == nil {
		f.assetByContentHash = map[string]Asset{}
	}
	f.assetByContentHash[asset.ContentHash] = asset
	return asset, nil
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (Asset, error) {
	asset, ok := f.assetByID[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

func (f *fakeRepository) FindByContentHash(_ context.Context, contentHash string) (Asset, error) {
	asset, ok := f.assetByContentHash[contentHash]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return asset, nil
}

func (f *fakeRepository) FindWithoutContentHashBySize(_ context.Context, sizeBytes int64) ([]Asset, error) {
	return f.assetsWithoutContentHashBySize[sizeBytes], nil
}

func (f *fakeRepository) UpdateContentHash(_ context.Context, id string, contentHash string) error {
	if f.updatedContentHashes == nil {
		f.updatedContentHashes = map[string]string{}
	}
	f.updatedContentHashes[id] = contentHash
	return nil
}

func (f *fakeRepository) ListRecent(_ context.Context) ([]Asset, error) {
	return f.listAssets, nil
}

type fakeFileStore struct {
	storedFile  StoredFile
	saveErr     error
	openFile    io.ReadSeekCloser
	openErr     error
	deletedPath string
	saveCalls   int
}

func (f *fakeFileStore) Save(_ context.Context, _ string, _ io.Reader) (StoredFile, error) {
	f.saveCalls++
	if f.saveErr != nil {
		return StoredFile{}, f.saveErr
	}
	return f.storedFile, nil
}

func (f *fakeFileStore) Open(_ string) (io.ReadSeekCloser, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}
	return f.openFile, nil
}

func (f *fakeFileStore) Delete(storagePath string) error {
	f.deletedPath = storagePath
	return nil
}

type duplicateOnSaveRepository struct {
	existingAsset Asset
	findCalls     int
}

func (d *duplicateOnSaveRepository) Save(_ context.Context, _ Asset) (Asset, error) {
	return Asset{}, ErrDuplicateContentHash
}

func (d *duplicateOnSaveRepository) FindByID(_ context.Context, _ string) (Asset, error) {
	return Asset{}, ErrNotFound
}

func (d *duplicateOnSaveRepository) FindByContentHash(_ context.Context, _ string) (Asset, error) {
	d.findCalls++
	if d.findCalls == 1 {
		return Asset{}, ErrNotFound
	}
	return d.existingAsset, nil
}

func (d *duplicateOnSaveRepository) FindWithoutContentHashBySize(_ context.Context, _ int64) ([]Asset, error) {
	return nil, nil
}

func (d *duplicateOnSaveRepository) UpdateContentHash(_ context.Context, _ string, _ string) error {
	return nil
}

func (d *duplicateOnSaveRepository) ListRecent(_ context.Context) ([]Asset, error) {
	return nil, nil
}

type readSeekNopCloser struct {
	*strings.Reader
}

func (r readSeekNopCloser) Close() error {
	return nil
}
