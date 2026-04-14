package local

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"homeMedia/internal/media"
)

type Store struct {
	rootDir string
	now     func() time.Time
}

func New(rootDir string) (Store, error) {
	cleanRoot, err := filepath.Abs(rootDir)
	if err != nil {
		return Store{}, fmt.Errorf("resolve upload root: %w", err)
	}

	if err := os.MkdirAll(cleanRoot, 0o755); err != nil {
		return Store{}, fmt.Errorf("create upload root: %w", err)
	}

	return Store{
		rootDir: cleanRoot,
		now:     time.Now,
	}, nil
}

func (s Store) Save(_ context.Context, originalFilename string, source io.Reader) (media.StoredFile, error) {
	return s.saveFile("", strings.ToLower(filepath.Ext(originalFilename)), source)
}

func (s Store) SaveThumbnail(_ context.Context, assetID string, source io.Reader) (media.StoredFile, error) {
	return s.saveFile("thumbnails", assetID+".jpg", source)
}

func (s Store) saveFile(directoryPrefix string, filename string, source io.Reader) (media.StoredFile, error) {
	fileName := filename
	if directoryPrefix == "" {
		extension := strings.ToLower(filepath.Ext(filename))
		fileName = uuid.NewString() + extension
	}

	folder := s.now().UTC().Format("20060102")
	storedFilename := fileName
	storagePath := filepath.ToSlash(filepath.Join(directoryPrefix, folder, storedFilename))
	if directoryPrefix == "" {
		storagePath = filepath.ToSlash(filepath.Join(folder, storedFilename))
	}

	fullPath, err := s.resolvePath(storagePath)
	if err != nil {
		return media.StoredFile{}, err
	}

	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return media.StoredFile{}, fmt.Errorf("create destination directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return media.StoredFile{}, fmt.Errorf("create destination file: %w", err)
	}
	defer file.Close()

	sizeBytes, err := io.Copy(file, source)
	if err != nil {
		return media.StoredFile{}, fmt.Errorf("write file: %w", err)
	}

	return media.StoredFile{
		StoredFilename: storedFilename,
		StoragePath:    storagePath,
		SizeBytes:      sizeBytes,
	}, nil
}

func (s Store) Open(storagePath string) (io.ReadSeekCloser, error) {
	fullPath, err := s.resolvePath(storagePath)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}

	return file, nil
}

func (s Store) Delete(storagePath string) error {
	fullPath, err := s.resolvePath(storagePath)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (s Store) resolvePath(storagePath string) (string, error) {
	cleanPath := filepath.Clean(storagePath)
	if filepath.IsAbs(cleanPath) {
		return "", media.ErrInvalidStoragePath
	}

	fullPath := filepath.Join(s.rootDir, cleanPath)
	relativePath, err := filepath.Rel(s.rootDir, fullPath)
	if err != nil {
		return "", media.ErrInvalidStoragePath
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", media.ErrInvalidStoragePath
	}

	return fullPath, nil
}
