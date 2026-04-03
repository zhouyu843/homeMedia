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

	asset, err := service.Upload(context.Background(), UploadInput{
		OriginalFilename: "photo.jpg",
		MIMEType:         "image/jpeg",
		SizeBytes:        4,
		Reader:           strings.NewReader("data"),
	})
	if err != nil {
		t.Fatalf("Upload returned error: %v", err)
	}

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
	listAssets []Asset
}

func (f *fakeRepository) Save(_ context.Context, asset Asset) (Asset, error) {
	if f.saveErr != nil {
		return Asset{}, f.saveErr
	}
	f.savedAsset = asset
	return asset, nil
}

func (f *fakeRepository) FindByID(_ context.Context, id string) (Asset, error) {
	asset, ok := f.assetByID[id]
	if !ok {
		return Asset{}, ErrNotFound
	}
	return asset, nil
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
}

func (f *fakeFileStore) Save(_ context.Context, _ string, _ io.Reader) (StoredFile, error) {
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
