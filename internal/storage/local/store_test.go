package local

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"homeMedia/internal/media"
)

func TestStoreSaveAndOpenRoundTrip(t *testing.T) {
	rootDir := t.TempDir()
	store, err := New(rootDir)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	storedFile, err := store.Save(context.Background(), "photo.jpg", strings.NewReader("image-data"))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	file, err := store.Open(storedFile.StoragePath)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll returned error: %v", err)
	}
	if string(content) != "image-data" {
		t.Fatalf("expected saved content to match, got %q", string(content))
	}

	fullPath := filepath.Join(rootDir, filepath.FromSlash(storedFile.StoragePath))
	if _, err := os.Stat(fullPath); err != nil {
		t.Fatalf("expected stored file on disk, got %v", err)
	}
}

func TestStoreRejectsPathTraversal(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = store.Open("../../etc/passwd")
	if !errors.Is(err, media.ErrInvalidStoragePath) {
		t.Fatalf("expected ErrInvalidStoragePath, got %v", err)
	}
}
