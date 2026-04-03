package http

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"homeMedia/internal/media"
	"homeMedia/internal/storage/local"
)

func TestUploadListDetailAndDownloadFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024)
	router := NewRouter(handler)

	uploadReq := newUploadRequest(t, "file", "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00})
	uploadResp := httptest.NewRecorder()
	router.ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusSeeOther {
		t.Fatalf("expected upload redirect, got %d with body %q", uploadResp.Code, uploadResp.Body.String())
	}

	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/media", nil))
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.Code)
	}
	if !strings.Contains(listResp.Body.String(), "photo.jpg") {
		t.Fatalf("expected list page to contain uploaded filename, got %q", listResp.Body.String())
	}

	if len(repo.assets) != 1 {
		t.Fatalf("expected one stored asset, got %d", len(repo.assets))
	}
	assetID := repo.assets[0].ID

	detailResp := httptest.NewRecorder()
	router.ServeHTTP(detailResp, httptest.NewRequest(http.MethodGet, "/media/"+assetID, nil))
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail status 200, got %d", detailResp.Code)
	}
	if !strings.Contains(detailResp.Body.String(), assetID) {
		t.Fatalf("expected detail page to contain asset id, got %q", detailResp.Body.String())
	}

	downloadResp := httptest.NewRecorder()
	router.ServeHTTP(downloadResp, httptest.NewRequest(http.MethodGet, "/media/"+assetID+"/download", nil))
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected download status 200, got %d", downloadResp.Code)
	}
	if got := downloadResp.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("expected download content type image/jpeg, got %q", got)
	}
	if !bytes.Contains(downloadResp.Body.Bytes(), []byte{0xff, 0xd8, 0xff}) {
		t.Fatalf("expected download body to contain stored bytes")
	}
}

func TestUploadRejectsInvalidFileType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024)
	router := NewRouter(handler)

	req := newUploadRequest(t, "file", "notes.txt", []byte("plain text"))
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415, got %d with body %q", resp.Code, resp.Body.String())
	}
}

func TestDownloadMissingFileReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{
		assets: []media.Asset{{
			ID:               "asset-1",
			OriginalFilename: "missing.jpg",
			StoredFilename:   "missing.jpg",
			MediaType:        media.MediaTypeImage,
			MIMEType:         "image/jpeg",
			SizeBytes:        10,
			StoragePath:      "20260403/missing.jpg",
			CreatedAt:        time.Now().UTC(),
		}},
	}
	service := media.NewService(repo, brokenStore{})
	handler := NewHandler(service, testTemplates(t), 10*1024*1024)
	router := NewRouter(handler)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/media/asset-1/download", nil))

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("list.html").Parse(`{{define "list.html"}}{{range .Assets}}{{.OriginalFilename}}{{end}}{{end}}{{define "detail.html"}}{{.Asset.ID}}{{end}}`))
}

func newUploadRequest(t *testing.T, fieldName string, filename string, content []byte) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/uploads", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type memoryRepository struct {
	assets []media.Asset
}

func (m *memoryRepository) Save(_ context.Context, asset media.Asset) (media.Asset, error) {
	m.assets = append([]media.Asset{asset}, m.assets...)
	return asset, nil
}

func (m *memoryRepository) FindByID(_ context.Context, id string) (media.Asset, error) {
	for _, asset := range m.assets {
		if asset.ID == id {
			return asset, nil
		}
	}
	return media.Asset{}, media.ErrNotFound
}

func (m *memoryRepository) ListRecent(_ context.Context) ([]media.Asset, error) {
	return m.assets, nil
}

type brokenStore struct{}

func (brokenStore) Save(_ context.Context, _ string, _ io.Reader) (media.StoredFile, error) {
	return media.StoredFile{}, nil
}

func (brokenStore) Open(_ string) (io.ReadSeekCloser, error) {
	return nil, os.ErrNotExist
}

func (brokenStore) Delete(_ string) error {
	return nil
}
