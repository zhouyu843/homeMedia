package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
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
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	uploadReq := newUploadRequest(t, "file", "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}, map[string]string{"csrf_token": uploadCSRFToken})
	uploadReq.AddCookie(sessionCookie)
	uploadResp := httptest.NewRecorder()
	router.ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusSeeOther {
		t.Fatalf("expected upload redirect, got %d with body %q", uploadResp.Code, uploadResp.Body.String())
	}

	listResp := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/media", nil)
	listReq.AddCookie(sessionCookie)
	router.ServeHTTP(listResp, listReq)
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
	detailReq := httptest.NewRequest(http.MethodGet, "/media/"+assetID, nil)
	detailReq.AddCookie(sessionCookie)
	router.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail status 200, got %d", detailResp.Code)
	}
	if !strings.Contains(detailResp.Body.String(), "/media/"+assetID+"/view") {
		t.Fatalf("expected detail page to contain preview url, got %q", detailResp.Body.String())
	}

	viewResp := httptest.NewRecorder()
	viewReq := httptest.NewRequest(http.MethodGet, "/media/"+assetID+"/view", nil)
	viewReq.AddCookie(sessionCookie)
	router.ServeHTTP(viewResp, viewReq)
	if viewResp.Code != http.StatusOK {
		t.Fatalf("expected view status 200, got %d", viewResp.Code)
	}
	if got := viewResp.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("expected view content type image/jpeg, got %q", got)
	}
	if got := viewResp.Header().Get("Content-Disposition"); !strings.Contains(got, "inline") {
		t.Fatalf("expected inline content disposition, got %q", got)
	}
	if !bytes.Contains(viewResp.Body.Bytes(), []byte{0xff, 0xd8, 0xff}) {
		t.Fatalf("expected view body to contain stored bytes")
	}

	downloadResp := httptest.NewRecorder()
	downloadReq := httptest.NewRequest(http.MethodGet, "/media/"+assetID+"/download", nil)
	downloadReq.AddCookie(sessionCookie)
	router.ServeHTTP(downloadResp, downloadReq)
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
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	req := newUploadRequest(t, "file", "notes.txt", []byte("plain text"), map[string]string{"csrf_token": csrfToken})
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415, got %d with body %q", resp.Code, resp.Body.String())
	}
}

func TestAPIUploadReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	req := newUploadRequest(t, "file", "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}, map[string]string{"csrf_token": uploadCSRFToken})
	req.URL.Path = "/api/uploads"
	req.Header.Set("X-CSRF-Token", uploadCSRFToken)
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d with body %q", resp.Code, resp.Body.String())
	}

	var payload struct {
		Existing bool `json:"existing"`
		Asset    struct {
			ID               string `json:"id"`
			OriginalFilename string `json:"originalFilename"`
			DetailURL        string `json:"detailUrl"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}

	if payload.Asset.ID == "" {
		t.Fatal("expected response asset id to be set")
	}
	if payload.Existing {
		t.Fatal("expected first upload to create a new asset")
	}
	if payload.Asset.OriginalFilename != "photo.jpg" {
		t.Fatalf("expected originalFilename photo.jpg, got %q", payload.Asset.OriginalFilename)
	}
	if !strings.HasPrefix(payload.Asset.DetailURL, "/media/") {
		t.Fatalf("expected detailUrl to start with /media/, got %q", payload.Asset.DetailURL)
	}
}

func TestAPIUploadReturnsExistingAssetForDuplicateContent(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)
	body := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}

	firstReq := newUploadRequest(t, "file", "photo-original.jpg", body, map[string]string{"csrf_token": uploadCSRFToken})
	firstReq.URL.Path = "/api/uploads"
	firstReq.Header.Set("X-CSRF-Token", uploadCSRFToken)
	firstReq.AddCookie(sessionCookie)
	firstResp := httptest.NewRecorder()
	router.ServeHTTP(firstResp, firstReq)
	if firstResp.Code != http.StatusCreated {
		t.Fatalf("expected first upload status 201, got %d with body %q", firstResp.Code, firstResp.Body.String())
	}

	secondReq := newUploadRequest(t, "file", "photo-copy.jpg", body, map[string]string{"csrf_token": uploadCSRFToken})
	secondReq.URL.Path = "/api/uploads"
	secondReq.Header.Set("X-CSRF-Token", uploadCSRFToken)
	secondReq.AddCookie(sessionCookie)
	secondResp := httptest.NewRecorder()
	router.ServeHTTP(secondResp, secondReq)
	if secondResp.Code != http.StatusOK {
		t.Fatalf("expected duplicate upload status 200, got %d with body %q", secondResp.Code, secondResp.Body.String())
	}

	var payload struct {
		Existing bool `json:"existing"`
		Asset    struct {
			ID               string `json:"id"`
			OriginalFilename string `json:"originalFilename"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(secondResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if !payload.Existing {
		t.Fatal("expected duplicate upload to be marked as existing")
	}
	if len(repo.assets) != 1 {
		t.Fatalf("expected one stored asset after duplicate upload, got %d", len(repo.assets))
	}
	if payload.Asset.OriginalFilename != "photo-original.jpg" {
		t.Fatalf("expected duplicate upload to reuse existing asset metadata, got %q", payload.Asset.OriginalFilename)
	}
}

func TestAPIUploadMatchesLegacyAssetWithoutContentHash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{assets: []media.Asset{{
		ID:               "asset-legacy",
		OriginalFilename: "photo-original.jpg",
		StoredFilename:   "stored-legacy.jpg",
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		StoragePath:      "20260403/stored-legacy.jpg",
		CreatedAt:        time.Now().UTC(),
	}}}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}
	storedFile, err := store.Save(context.Background(), "photo-original.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	repo.assets[0].StoredFilename = storedFile.StoredFilename
	repo.assets[0].StoragePath = storedFile.StoragePath

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)
	body := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}

	req := newUploadRequest(t, "file", "photo-copy.jpg", body, map[string]string{"csrf_token": uploadCSRFToken})
	req.URL.Path = "/api/uploads"
	req.Header.Set("X-CSRF-Token", uploadCSRFToken)
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected duplicate upload status 200, got %d with body %q", resp.Code, resp.Body.String())
	}
	if len(repo.assets) != 1 {
		t.Fatalf("expected legacy asset to be reused, got %d assets", len(repo.assets))
	}
}

func TestAPIUploadRejectsInvalidFileType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	req := newUploadRequest(t, "file", "notes.txt", []byte("plain text"), map[string]string{"csrf_token": csrfToken})
	req.URL.Path = "/api/uploads"
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("expected status 415, got %d with body %q", resp.Code, resp.Body.String())
	}

	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if payload.Code != "unsupported_media_type" {
		t.Fatalf("expected code unsupported_media_type, got %q", payload.Code)
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
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/asset-1/download", nil)
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
}

func TestDetailListAndTrashRenderProtectedForms(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deletedAt := time.Now().UTC()
	repo := &memoryRepository{assets: []media.Asset{
		{
			ID:               "asset-1",
			OriginalFilename: "photo.jpg",
			StoredFilename:   "photo.jpg",
			MediaType:        media.MediaTypeImage,
			MIMEType:         "image/jpeg",
			SizeBytes:        7,
			StoragePath:      "20260403/photo.jpg",
			CreatedAt:        time.Now().UTC(),
		},
		{
			ID:               "asset-2",
			OriginalFilename: "trashed-photo.jpg",
			StoredFilename:   "trashed-photo.jpg",
			MediaType:        media.MediaTypeImage,
			MIMEType:         "image/jpeg",
			SizeBytes:        9,
			StoragePath:      "20260403/trashed-photo.jpg",
			CreatedAt:        deletedAt.Add(-time.Hour),
			DeletedAt:        &deletedAt,
		},
	}}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	listResp := httptest.NewRecorder()
	listReq := httptest.NewRequest(http.MethodGet, "/media", nil)
	listReq.AddCookie(sessionCookie)
	router.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list status 200, got %d", listResp.Code)
	}
	if !strings.Contains(listResp.Body.String(), "/media/asset-1/delete") {
		t.Fatalf("expected list page delete form, got %q", listResp.Body.String())
	}
	if strings.Contains(listResp.Body.String(), "data-confirm-message=") {
		t.Fatalf("expected list page delete without confirm, got %q", listResp.Body.String())
	}

	detailResp := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/media/asset-1", nil)
	detailReq.AddCookie(sessionCookie)
	router.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected detail status 200, got %d", detailResp.Code)
	}
	if !strings.Contains(detailResp.Body.String(), "/media/asset-1/delete") {
		t.Fatalf("expected detail page delete form, got %q", detailResp.Body.String())
	}
	if strings.Contains(detailResp.Body.String(), "data-confirm-message=") {
		t.Fatalf("expected detail page delete without confirm, got %q", detailResp.Body.String())
	}

	trashResp := httptest.NewRecorder()
	trashReq := httptest.NewRequest(http.MethodGet, "/trash", nil)
	trashReq.AddCookie(sessionCookie)
	router.ServeHTTP(trashResp, trashReq)
	if trashResp.Code != http.StatusOK {
		t.Fatalf("expected trash status 200, got %d", trashResp.Code)
	}
	trashBody := trashResp.Body.String()
	if !strings.Contains(trashBody, "/media/asset-2/restore") {
		t.Fatalf("expected trash page restore form, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "data-confirm-message=\"确定恢复这个媒体吗？恢复后会重新出现在媒体库中。\"") {
		t.Fatalf("expected trash page restore confirm, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "/media/asset-2/permanent-delete") {
		t.Fatalf("expected trash page permanent delete form, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "data-confirm-message=\"确定彻底删除这个媒体吗？如果没有其他活跃记录引用同一文件，物理文件也会一起删除。\"") {
		t.Fatalf("expected trash page permanent delete confirm, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "/trash/empty") {
		t.Fatalf("expected trash page empty form, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "data-confirm-message=\"确定清空回收站吗？其中的媒体记录会被彻底删除。\"") {
		t.Fatalf("expected trash page empty confirm, got %q", trashBody)
	}
	if !strings.Contains(trashBody, "/static/js/confirm-dialog.js") {
		t.Fatalf("expected trash page confirm script, got %q", trashBody)
	}
}

func TestDeleteMediaRequiresCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{assets: []media.Asset{{
		ID:               "asset-1",
		OriginalFilename: "photo.jpg",
		StoredFilename:   "photo.jpg",
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		StoragePath:      "20260403/photo.jpg",
		CreatedAt:        time.Now().UTC(),
	}}}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/asset-1/delete", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
}

func TestDeleteMediaRemovesAssetAndRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	storedFile, err := store.Save(context.Background(), "photo.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	repo.assets = []media.Asset{{
		ID:               "asset-1",
		OriginalFilename: "photo.jpg",
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		StoragePath:      storedFile.StoragePath,
		CreatedAt:        time.Now().UTC(),
	}}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/media/asset-1", sessionCookie)

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/asset-1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d with body %q", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Location"); got != "/media" {
		t.Fatalf("expected redirect to /media, got %q", got)
	}
	if _, err := store.Open(storedFile.StoragePath); err != nil {
		t.Fatalf("expected soft-deleted file to remain on disk, got %v", err)
	}

	detailResp := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/media/asset-1", nil)
	detailReq.AddCookie(sessionCookie)
	router.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusNotFound {
		t.Fatalf("expected deleted detail status 404, got %d", detailResp.Code)
	}

	trashResp := httptest.NewRecorder()
	trashReq := httptest.NewRequest(http.MethodGet, "/trash", nil)
	trashReq.AddCookie(sessionCookie)
	router.ServeHTTP(trashResp, trashReq)
	if trashResp.Code != http.StatusOK {
		t.Fatalf("expected trash status 200, got %d", trashResp.Code)
	}
	if !strings.Contains(trashResp.Body.String(), "photo.jpg") {
		t.Fatalf("expected trashed asset to appear in trash page, got %q", trashResp.Body.String())
	}
}

func TestDeleteMediaKeepsSharedFileForRemainingAsset(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	storedFile, err := store.Save(context.Background(), "photo.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	now := time.Now().UTC()
	repo.assets = []media.Asset{
		{
			ID:               "asset-1",
			OriginalFilename: "photo-a.jpg",
			StoredFilename:   storedFile.StoredFilename,
			MediaType:        media.MediaTypeImage,
			MIMEType:         "image/jpeg",
			SizeBytes:        7,
			StoragePath:      storedFile.StoragePath,
			CreatedAt:        now,
		},
		{
			ID:               "asset-2",
			OriginalFilename: "photo-b.jpg",
			StoredFilename:   storedFile.StoredFilename,
			MediaType:        media.MediaTypeImage,
			MIMEType:         "image/jpeg",
			SizeBytes:        7,
			StoragePath:      storedFile.StoragePath,
			CreatedAt:        now.Add(-time.Minute),
		},
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/media/asset-1", sessionCookie)

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/asset-1/delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", resp.Code)
	}
	if _, err := store.Open(storedFile.StoragePath); err != nil {
		t.Fatalf("expected shared file to remain, got %v", err)
	}

	downloadResp := httptest.NewRecorder()
	downloadReq := httptest.NewRequest(http.MethodGet, "/media/asset-2/download", nil)
	downloadReq.AddCookie(sessionCookie)
	router.ServeHTTP(downloadResp, downloadReq)
	if downloadResp.Code != http.StatusOK {
		t.Fatalf("expected remaining asset download status 200, got %d", downloadResp.Code)
	}
}

func TestRestoreMediaRestoresAssetAndRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	storedFile, err := store.Save(context.Background(), "photo.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	now := time.Now().UTC().Add(-time.Minute)
	deletedAt := time.Now().UTC()
	repo.assets = []media.Asset{{
		ID:               "asset-1",
		OriginalFilename: "photo.jpg",
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		StoragePath:      storedFile.StoragePath,
		CreatedAt:        now,
		DeletedAt:        &deletedAt,
	}}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/trash", sessionCookie)

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/asset-1/restore", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d with body %q", resp.Code, resp.Body.String())
	}
	if got := resp.Header().Get("Location"); got != "/trash" {
		t.Fatalf("expected redirect to /trash, got %q", got)
	}

	detailResp := httptest.NewRecorder()
	detailReq := httptest.NewRequest(http.MethodGet, "/media/asset-1", nil)
	detailReq.AddCookie(sessionCookie)
	router.ServeHTTP(detailResp, detailReq)
	if detailResp.Code != http.StatusOK {
		t.Fatalf("expected restored detail status 200, got %d", detailResp.Code)
	}
}

func TestPermanentDeleteRemovesAssetAndFile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	storedFile, err := store.Save(context.Background(), "photo.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}))
	if err != nil {
		t.Fatalf("Save returned error: %v", err)
	}
	deletedAt := time.Now().UTC()
	repo.assets = []media.Asset{{
		ID:               "asset-1",
		OriginalFilename: "photo.jpg",
		StoredFilename:   storedFile.StoredFilename,
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		StoragePath:      storedFile.StoragePath,
		CreatedAt:        time.Now().UTC().Add(-time.Minute),
		DeletedAt:        &deletedAt,
	}}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	csrfToken := getPageCSRFToken(t, router, "/trash", sessionCookie)

	form := url.Values{}
	form.Set("csrf_token", csrfToken)
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/media/asset-1/permanent-delete", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d with body %q", resp.Code, resp.Body.String())
	}
	if _, err := store.Open(storedFile.StoragePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected physical file to be deleted, got %v", err)
	}
}

func TestUploadMediaJSONReturnsConflictForDeletedDuplicate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}
	hash := sha256.Sum256(body)

	repo := &memoryRepository{assets: []media.Asset{{
		ID:               "asset-deleted",
		OriginalFilename: "photo.jpg",
		StoredFilename:   "photo.jpg",
		MediaType:        media.MediaTypeImage,
		MIMEType:         "image/jpeg",
		SizeBytes:        7,
		ContentHash:      hex.EncodeToString(hash[:]),
		StoragePath:      "20260403/photo.jpg",
		CreatedAt:        time.Now().UTC().Add(-time.Minute),
		DeletedAt:        timePointer(time.Now().UTC()),
	}}}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	req := newUploadRequest(t, "file", "photo-copy.jpg", body, map[string]string{"csrf_token": uploadCSRFToken})
	req.URL.Path = "/api/uploads"
	req.Header.Set("X-CSRF-Token", uploadCSRFToken)
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d with body %q", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "trashed_duplicate") {
		t.Fatalf("expected conflict response code, got %q", resp.Body.String())
	}
}

func TestViewMissingFileReturnsNotFound(t *testing.T) {
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
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/asset-1/view", nil)
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
}

func TestThumbnailMediaReturnsJPEG(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)
	uploadCSRFToken := getPageCSRFToken(t, router, "/media", sessionCookie)

	validTinyPNG := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0x99, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x03, 0x01, 0x01, 0x00, 0xc9, 0xfe, 0x92,
		0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	uploadReq := newUploadRequest(t, "file", "photo.png", validTinyPNG, map[string]string{"csrf_token": uploadCSRFToken})
	uploadReq.AddCookie(sessionCookie)
	uploadResp := httptest.NewRecorder()
	router.ServeHTTP(uploadResp, uploadReq)
	if uploadResp.Code != http.StatusSeeOther {
		t.Fatalf("expected upload redirect, got %d with body %q", uploadResp.Code, uploadResp.Body.String())
	}

	assetID := repo.assets[0].ID

	thumbResp := httptest.NewRecorder()
	thumbReq := httptest.NewRequest(http.MethodGet, "/media/"+assetID+"/thumbnail", nil)
	thumbReq.AddCookie(sessionCookie)
	router.ServeHTTP(thumbResp, thumbReq)

	if thumbResp.Code != http.StatusOK {
		t.Fatalf("expected thumbnail status 200, got %d with body %q", thumbResp.Code, thumbResp.Body.String())
	}
	if got := thumbResp.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("expected thumbnail content type image/jpeg, got %q", got)
	}
	if len(thumbResp.Body.Bytes()) == 0 {
		t.Fatal("expected thumbnail body not empty")
	}
}

func TestThumbnailMissingFileReturnsPlaceholder(t *testing.T) {
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
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/media/asset-1/thumbnail", nil)
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	if got := resp.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("expected content type image/svg+xml, got %q", got)
	}
	if !strings.Contains(resp.Body.String(), "PREVIEW") {
		t.Fatalf("expected placeholder thumbnail body, got %q", resp.Body.String())
	}
}

func TestProtectedRoutesRedirectWhenUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/media", nil))

	if resp.Code != http.StatusSeeOther {
		t.Fatalf("expected status 303, got %d", resp.Code)
	}
	if got := resp.Header().Get("Location"); got != "/login" {
		t.Fatalf("expected redirect to /login, got %q", got)
	}
}

func TestUploadRejectsMissingCSRF(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &memoryRepository{}
	store, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New returned error: %v", err)
	}

	service := media.NewService(repo, store)
	handler := NewHandler(service, testTemplates(t), 10*1024*1024, testAuth())
	router := NewRouter(handler)
	sessionCookie := loginAndGetSessionCookie(t, router)

	req := newUploadRequest(t, "file", "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}, nil)
	req.AddCookie(sessionCookie)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
}

func testTemplates(t *testing.T) *template.Template {
	t.Helper()
	return template.Must(template.New("base").Parse(`
{{define "list.html"}}
<form action="/uploads" method="post" enctype="multipart/form-data">
<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
<input id="file" name="file" type="file" required>
</form>
<a href="/trash">回收站</a>
{{range .Assets}}
<a href="/media/{{.ID}}">{{.OriginalFilename}}</a>
<form action="/media/{{.ID}}/delete" method="post"><input type="hidden" name="csrf_token" value="{{$.CSRFToken}}"></form>
{{end}}
{{end}}
{{define "detail.html"}}
{{if eq .Asset.MediaType "image"}}<img src="/media/{{.Asset.ID}}/view" alt="{{.Asset.OriginalFilename}}">{{end}}
{{if eq .Asset.MediaType "video"}}<video controls><source src="/media/{{.Asset.ID}}/view" type="{{.Asset.MIMEType}}"></video>{{end}}
<img src="/media/{{.Asset.ID}}/thumbnail" alt="thumb">
<a href="/media/{{.Asset.ID}}/download">download</a>
<form action="/media/{{.Asset.ID}}/delete" method="post"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"></form>
<form action="/logout" method="post"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"></form>
{{end}}
{{define "trash.html"}}
{{range .Assets}}
<span>{{.OriginalFilename}}</span>
<form action="/media/{{.ID}}/restore" method="post" data-confirm-message="确定恢复这个媒体吗？恢复后会重新出现在媒体库中。"><input type="hidden" name="csrf_token" value="{{$.CSRFToken}}"></form>
<form action="/media/{{.ID}}/permanent-delete" method="post" data-confirm-message="确定彻底删除这个媒体吗？如果没有其他活跃记录引用同一文件，物理文件也会一起删除。"><input type="hidden" name="csrf_token" value="{{$.CSRFToken}}"></form>
{{end}}
<form action="/trash/empty" method="post" data-confirm-message="确定清空回收站吗？其中的媒体记录会被彻底删除。"><input type="hidden" name="csrf_token" value="{{.CSRFToken}}"></form>
<script src="/static/js/confirm-dialog.js?v=20260406-2" defer></script>
{{end}}
{{define "login.html"}}
{{if .Error}}<p>{{.Error}}</p>{{end}}
<form action="/login" method="post">
<input type="hidden" name="csrf_token" value="{{.CSRFToken}}">
<input name="username" type="text">
<input name="password" type="password">
</form>
{{end}}
`))
}

func newUploadRequest(t *testing.T, fieldName string, filename string, content []byte, fields map[string]string) *http.Request {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField returned error: %v", err)
		}
	}
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

func testAuth() *AuthService {
	return NewAuthService("admin", "pass123", "test-secret", 24*time.Hour)
}

func loginAndGetSessionCookie(t *testing.T, router *gin.Engine) *http.Cookie {
	t.Helper()

	loginPageResp := httptest.NewRecorder()
	router.ServeHTTP(loginPageResp, httptest.NewRequest(http.MethodGet, "/login", nil))
	if loginPageResp.Code != http.StatusOK {
		t.Fatalf("expected login page status 200, got %d", loginPageResp.Code)
	}

	loginCSRF := extractCSRFToken(t, loginPageResp.Body.String())
	loginCSRFCookie := findCookie(t, loginPageResp.Result().Cookies(), loginCSRFCookieName)

	form := url.Values{}
	form.Set("username", "admin")
	form.Set("password", "pass123")
	form.Set("csrf_token", loginCSRF)
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.AddCookie(loginCSRFCookie)

	loginResp := httptest.NewRecorder()
	router.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusSeeOther {
		t.Fatalf("expected login status 303, got %d with body %q", loginResp.Code, loginResp.Body.String())
	}

	return findCookie(t, loginResp.Result().Cookies(), sessionCookieName)
}

func getPageCSRFToken(t *testing.T, router *gin.Engine, path string, sessionCookie *http.Cookie) string {
	t.Helper()

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.AddCookie(sessionCookie)
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected %s status 200, got %d", path, resp.Code)
	}

	return extractCSRFToken(t, resp.Body.String())
}

func extractCSRFToken(t *testing.T, body string) string {
	t.Helper()

	re := regexp.MustCompile(`name="csrf_token"\s+value="([^"]+)"`)
	matches := re.FindStringSubmatch(body)
	if len(matches) != 2 {
		t.Fatalf("csrf token not found in body: %q", body)
	}

	return matches[1]
}

func findCookie(t *testing.T, cookies []*http.Cookie, name string) *http.Cookie {
	t.Helper()

	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}

	t.Fatalf("cookie %q not found", name)
	return nil
}

type memoryRepository struct {
	mu     sync.Mutex
	assets []media.Asset
}

func (m *memoryRepository) Save(_ context.Context, asset media.Asset) (media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.assets = append([]media.Asset{asset}, m.assets...)
	return asset, nil
}

func (m *memoryRepository) FindByID(_ context.Context, id string) (media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, asset := range m.assets {
		if asset.ID == id && asset.DeletedAt == nil {
			return asset, nil
		}
	}
	return media.Asset{}, media.ErrNotFound
}

func (m *memoryRepository) FindDeletedByID(_ context.Context, id string) (media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, asset := range m.assets {
		if asset.ID == id && asset.DeletedAt != nil {
			return asset, nil
		}
	}
	return media.Asset{}, media.ErrNotFound
}

func (m *memoryRepository) FindByContentHash(_ context.Context, contentHash string) (media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, asset := range m.assets {
		if asset.ContentHash == contentHash && asset.DeletedAt == nil {
			return asset, nil
		}
	}
	return media.Asset{}, media.ErrNotFound
}

func (m *memoryRepository) FindDeletedByContentHash(_ context.Context, contentHash string) (media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, asset := range m.assets {
		if asset.ContentHash == contentHash && asset.DeletedAt != nil {
			return asset, nil
		}
	}
	return media.Asset{}, media.ErrNotFound
}

func (m *memoryRepository) FindWithoutContentHashBySize(_ context.Context, sizeBytes int64) ([]media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var assets []media.Asset
	for _, asset := range m.assets {
		if asset.ContentHash == "" && asset.SizeBytes == sizeBytes && asset.DeletedAt == nil {
			assets = append(assets, asset)
		}
	}
	return assets, nil
}

func (m *memoryRepository) UpdateContentHash(_ context.Context, id string, contentHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, asset := range m.assets {
		if asset.ID == id {
			asset.ContentHash = contentHash
			m.assets[index] = asset
			return nil
		}
	}
	return media.ErrNotFound
}

func (m *memoryRepository) ListRecent(_ context.Context) ([]media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	assets := make([]media.Asset, 0, len(m.assets))
	for _, asset := range m.assets {
		if asset.DeletedAt == nil {
			assets = append(assets, asset)
		}
	}
	return assets, nil
}

func (m *memoryRepository) ListTrash(_ context.Context) ([]media.Asset, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	assets := make([]media.Asset, 0, len(m.assets))
	for _, asset := range m.assets {
		if asset.DeletedAt != nil {
			assets = append(assets, asset)
		}
	}
	return assets, nil
}

func (m *memoryRepository) SoftDelete(_ context.Context, id string, deletedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, asset := range m.assets {
		if asset.ID == id && asset.DeletedAt == nil {
			asset.DeletedAt = &deletedAt
			m.assets[index] = asset
			return nil
		}
	}
	return media.ErrNotFound
}

func (m *memoryRepository) Restore(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, asset := range m.assets {
		if asset.ID == id && asset.DeletedAt != nil {
			for _, active := range m.assets {
				if active.DeletedAt == nil && active.ContentHash != "" && active.ContentHash == asset.ContentHash {
					return media.ErrDuplicateContentHash
				}
			}
			asset.DeletedAt = nil
			m.assets[index] = asset
			return nil
		}
	}
	return media.ErrNotFound
}

func (m *memoryRepository) Delete(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index, asset := range m.assets {
		if asset.ID == id && asset.DeletedAt != nil {
			m.assets = append(m.assets[:index], m.assets[index+1:]...)
			return nil
		}
	}
	return media.ErrNotFound
}

func (m *memoryRepository) CountByStoragePath(_ context.Context, storagePath string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, asset := range m.assets {
		if asset.StoragePath == storagePath && asset.DeletedAt == nil {
			count++
		}
	}
	return count, nil
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

func timePointer(value time.Time) *time.Time {
	return &value
}
