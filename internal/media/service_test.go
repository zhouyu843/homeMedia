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

func TestServiceDeleteRemovesAssetAndFile(t *testing.T) {
	repo := &fakeRepository{
		assetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg"},
		},
	}
	store := &fakeFileStore{}
	service := NewService(repo, store)

	err := service.Delete(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.softDeletedID != "asset-1" {
		t.Fatalf("expected soft deleted ID asset-1, got %q", repo.softDeletedID)
	}
	if store.deletedPath != "" {
		t.Fatalf("expected soft delete to skip physical delete, got %q", store.deletedPath)
	}
}

func TestServiceDeleteSkipsPhysicalDeleteForSharedFile(t *testing.T) {
	repo := &fakeRepository{
		assetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg"},
		},
	}
	store := &fakeFileStore{}
	service := NewService(repo, store)

	err := service.Delete(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if repo.softDeletedID != "asset-1" {
		t.Fatalf("expected soft deleted ID asset-1, got %q", repo.softDeletedID)
	}
	if store.deletedPath != "" {
		t.Fatalf("expected soft delete to skip physical delete, got %q", store.deletedPath)
	}
}

func TestServiceDeletePermanentlyMissingFileStillDeletesRecord(t *testing.T) {
	repo := &fakeRepository{
		deletedAssetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg", DeletedAt: timePointer(time.Now().UTC())},
		},
		storagePathCounts: map[string]int{"20260403/photo.jpg": 0},
	}
	store := &fakeFileStore{deleteErr: os.ErrNotExist}
	service := NewService(repo, store)

	err := service.DeletePermanently(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("expected permanent delete to tolerate missing file, got %v", err)
	}
	if repo.deletedID != "asset-1" {
		t.Fatalf("expected deleted ID asset-1, got %q", repo.deletedID)
	}
}

func TestServiceRestoreClearsDeletedAsset(t *testing.T) {
	repo := &fakeRepository{
		deletedAssetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg", DeletedAt: timePointer(time.Now().UTC())},
		},
	}
	service := NewService(repo, &fakeFileStore{})

	err := service.Restore(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("Restore returned error: %v", err)
	}
	if repo.restoredID != "asset-1" {
		t.Fatalf("expected restored ID asset-1, got %q", repo.restoredID)
	}
}

func TestServiceDeletePermanentlyRemovesAssetAndFile(t *testing.T) {
	repo := &fakeRepository{
		deletedAssetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg", DeletedAt: timePointer(time.Now().UTC())},
		},
		storagePathCounts: map[string]int{"20260403/photo.jpg": 0},
	}
	store := &fakeFileStore{}
	service := NewService(repo, store)

	err := service.DeletePermanently(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("DeletePermanently returned error: %v", err)
	}
	if repo.deletedID != "asset-1" {
		t.Fatalf("expected deleted ID asset-1, got %q", repo.deletedID)
	}
	if store.deletedPath != "20260403/photo.jpg" {
		t.Fatalf("expected deleted path 20260403/photo.jpg, got %q", store.deletedPath)
	}
}

func TestServiceDeletePermanentlySkipsPhysicalDeleteForActiveSharedFile(t *testing.T) {
	repo := &fakeRepository{
		deletedAssetByID: map[string]Asset{
			"asset-1": {ID: "asset-1", StoragePath: "20260403/photo.jpg", DeletedAt: timePointer(time.Now().UTC())},
		},
		storagePathCounts: map[string]int{"20260403/photo.jpg": 1},
	}
	store := &fakeFileStore{}
	service := NewService(repo, store)

	err := service.DeletePermanently(context.Background(), "asset-1")
	if err != nil {
		t.Fatalf("DeletePermanently returned error: %v", err)
	}
	if store.deletedPath != "" {
		t.Fatalf("expected shared file to skip physical delete, got %q", store.deletedPath)
	}
}

func TestServiceListTrashReturnsDeletedAssets(t *testing.T) {
	deletedAt := time.Now().UTC()
	repo := &fakeRepository{trashAssets: []Asset{{ID: "asset-1", DeletedAt: &deletedAt}}}
	service := NewService(repo, &fakeFileStore{})

	assets, err := service.ListTrash(context.Background())
	if err != nil {
		t.Fatalf("ListTrash returned error: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != "asset-1" {
		t.Fatalf("expected one trashed asset, got %+v", assets)
	}
}

func TestServiceUploadReturnsDecisionForDeletedDuplicate(t *testing.T) {
	deletedAt := time.Now().UTC()
	repo := &fakeRepository{deletedAssetByContentHash: map[string]Asset{
		"3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7": {
			ID: "asset-deleted", OriginalFilename: "deleted.jpg", ContentHash: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7", DeletedAt: &deletedAt,
		},
	}}
	service := NewService(repo, &fakeFileStore{})

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "different-name.jpg",
		MIMEType:         "image/jpeg",
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if !result.RequiresDecision {
		t.Fatalf("expected upload to require decision, got %+v", result)
	}
	if result.DecisionAsset.ID != "asset-deleted" {
		t.Fatalf("expected decision asset asset-deleted, got %q", result.DecisionAsset.ID)
	}
}

func TestServiceUploadRestoresDeletedDuplicateWhenRequested(t *testing.T) {
	deletedAt := time.Now().UTC()
	deletedAsset := Asset{ID: "asset-deleted", OriginalFilename: "deleted.jpg", ContentHash: "3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7", DeletedAt: &deletedAt}
	repo := &fakeRepository{
		deletedAssetByID: map[string]Asset{"asset-deleted": deletedAsset},
		deletedAssetByContentHash: map[string]Asset{
			"3a6eb0790f39ac87c94f3856b2dd2c5d110e6811602261a9a923d3bb23adc8b7": deletedAsset,
		},
	}
	service := NewService(repo, &fakeFileStore{})

	result, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "different-name.jpg",
		MIMEType:         "image/jpeg",
		Reader:           strings.NewReader("data"),
		DuplicateAction:  DuplicateActionRestore,
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}
	if !result.Restored {
		t.Fatalf("expected upload to restore deleted duplicate, got %+v", result)
	}
	if repo.restoredID != "asset-deleted" {
		t.Fatalf("expected restored ID asset-deleted, got %q", repo.restoredID)
	}
}

func TestServiceDeleteReturnsNotFound(t *testing.T) {
	service := NewService(&fakeRepository{}, &fakeFileStore{})

	err := service.Delete(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type fakeRepository struct {
	savedAsset                     Asset
	saveErr                        error
	assetByID                      map[string]Asset
	deletedAssetByID               map[string]Asset
	assetByContentHash             map[string]Asset
	deletedAssetByContentHash      map[string]Asset
	assetsWithoutContentHashBySize map[int64][]Asset
	updatedContentHashes           map[string]string
	listAssets                     []Asset
	trashAssets                    []Asset
	storagePathCounts              map[string]int
	softDeletedID                  string
	restoredID                     string
	deletedID                      string
	deleteErr                      error
	restoreErr                     error
	softDeleteErr                  error
	countErr                       error
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

func (f *fakeRepository) FindDeletedByID(_ context.Context, id string) (Asset, error) {
	asset, ok := f.deletedAssetByID[id]
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

func (f *fakeRepository) FindDeletedByContentHash(_ context.Context, contentHash string) (Asset, error) {
	asset, ok := f.deletedAssetByContentHash[contentHash]
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

func (f *fakeRepository) ListTrash(_ context.Context) ([]Asset, error) {
	return f.trashAssets, nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, id string, deletedAt time.Time) error {
	if f.softDeleteErr != nil {
		return f.softDeleteErr
	}
	asset, ok := f.assetByID[id]
	if !ok {
		return ErrNotFound
	}
	f.softDeletedID = id
	asset.DeletedAt = &deletedAt
	delete(f.assetByID, id)
	if f.deletedAssetByID == nil {
		f.deletedAssetByID = map[string]Asset{}
	}
	f.deletedAssetByID[id] = asset
	if asset.ContentHash != "" {
		if f.deletedAssetByContentHash == nil {
			f.deletedAssetByContentHash = map[string]Asset{}
		}
		f.deletedAssetByContentHash[asset.ContentHash] = asset
	}
	return nil
}

func (f *fakeRepository) Restore(_ context.Context, id string) error {
	if f.restoreErr != nil {
		return f.restoreErr
	}
	asset, ok := f.deletedAssetByID[id]
	if !ok {
		return ErrNotFound
	}
	f.restoredID = id
	asset.DeletedAt = nil
	delete(f.deletedAssetByID, id)
	if asset.ContentHash != "" {
		delete(f.deletedAssetByContentHash, asset.ContentHash)
	}
	if f.assetByID == nil {
		f.assetByID = map[string]Asset{}
	}
	f.assetByID[id] = asset
	return nil
}

func (f *fakeRepository) Delete(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if _, ok := f.deletedAssetByID[id]; !ok {
		return ErrNotFound
	}
	f.deletedID = id
	asset := f.deletedAssetByID[id]
	delete(f.deletedAssetByID, id)
	if asset.ContentHash != "" {
		delete(f.deletedAssetByContentHash, asset.ContentHash)
	}
	return nil
}

func (f *fakeRepository) CountByStoragePath(_ context.Context, storagePath string) (int, error) {
	if f.countErr != nil {
		return 0, f.countErr
	}
	return f.storagePathCounts[storagePath], nil
}

type fakeFileStore struct {
	storedFile  StoredFile
	saveErr     error
	openFile    io.ReadSeekCloser
	openErr     error
	deletedPath string
	deleteErr   error
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
	return f.deleteErr
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

func (d *duplicateOnSaveRepository) FindDeletedByID(_ context.Context, _ string) (Asset, error) {
	return Asset{}, ErrNotFound
}

func (d *duplicateOnSaveRepository) FindByContentHash(_ context.Context, _ string) (Asset, error) {
	d.findCalls++
	if d.findCalls == 1 {
		return Asset{}, ErrNotFound
	}
	return d.existingAsset, nil
}

func (d *duplicateOnSaveRepository) FindDeletedByContentHash(_ context.Context, _ string) (Asset, error) {
	return Asset{}, ErrNotFound
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

func (d *duplicateOnSaveRepository) ListTrash(_ context.Context) ([]Asset, error) {
	return nil, nil
}

func (d *duplicateOnSaveRepository) SoftDelete(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (d *duplicateOnSaveRepository) Restore(_ context.Context, _ string) error {
	return nil
}

func (d *duplicateOnSaveRepository) Delete(_ context.Context, _ string) error {
	return nil
}

func (d *duplicateOnSaveRepository) CountByStoragePath(_ context.Context, _ string) (int, error) {
	return 0, nil
}

type readSeekNopCloser struct {
	*strings.Reader
}

func (r readSeekNopCloser) Close() error {
	return nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}
