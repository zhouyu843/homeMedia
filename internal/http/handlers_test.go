package http

import (
  "bytes"
  "context"
  "crypto/sha256"
  "encoding/hex"
  "encoding/json"
  "errors"
  "io"
  "mime/multipart"
  "net/http"
  "net/http/httptest"
  "os"
  "strings"
  "sync"
  "testing"
  "time"

  "github.com/gin-gonic/gin"

  "homeMedia/internal/media"
  "homeMedia/internal/storage/local"
)

func TestAuthStatusAndLoginFlow(t *testing.T) {
  gin.SetMode(gin.TestMode)

  router := testRouter(t, &memoryRepository{}, mustLocalStore(t))

  statusResp := httptest.NewRecorder()
  router.ServeHTTP(statusResp, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
  if statusResp.Code != http.StatusOK {
    t.Fatalf("expected auth status 200, got %d", statusResp.Code)
  }

  var anonymous authStatusResponse
  if err := json.Unmarshal(statusResp.Body.Bytes(), &anonymous); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }
  if anonymous.Authenticated {
    t.Fatal("expected anonymous auth status")
  }
  if anonymous.CSRFToken == "" {
    t.Fatal("expected login csrf token")
  }

  loginCSRFCookie := findCookie(t, statusResp.Result().Cookies(), loginCSRFCookieName)
  sessionCookie, sessionStatus := loginAndGetSession(t, router, anonymous.CSRFToken, loginCSRFCookie)
  if !sessionStatus.Authenticated {
    t.Fatal("expected authenticated login response")
  }
  if sessionStatus.CSRFToken == "" {
    t.Fatal("expected session csrf token")
  }

  verifyResp := httptest.NewRecorder()
  verifyReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
  verifyReq.AddCookie(sessionCookie)
  router.ServeHTTP(verifyResp, verifyReq)
  if verifyResp.Code != http.StatusOK {
    t.Fatalf("expected authenticated status 200, got %d", verifyResp.Code)
  }

  var authenticated authStatusResponse
  if err := json.Unmarshal(verifyResp.Body.Bytes(), &authenticated); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }
  if !authenticated.Authenticated {
    t.Fatal("expected authenticated auth status")
  }
}

func TestProtectedRoutesHandleUnauthenticatedRequests(t *testing.T) {
  gin.SetMode(gin.TestMode)

  router := testRouter(t, &memoryRepository{}, mustLocalStore(t))

  pageResp := httptest.NewRecorder()
  router.ServeHTTP(pageResp, httptest.NewRequest(http.MethodGet, "/media", nil))
  if pageResp.Code != http.StatusSeeOther {
    t.Fatalf("expected media redirect 303, got %d", pageResp.Code)
  }
  if location := pageResp.Header().Get("Location"); location != "/login" {
    t.Fatalf("expected redirect to /login, got %q", location)
  }

  apiResp := httptest.NewRecorder()
  router.ServeHTTP(apiResp, httptest.NewRequest(http.MethodGet, "/api/media", nil))
  if apiResp.Code != http.StatusUnauthorized {
    t.Fatalf("expected api unauthorized 401, got %d", apiResp.Code)
  }
  if !strings.Contains(apiResp.Body.String(), "unauthorized") {
    t.Fatalf("expected unauthorized payload, got %q", apiResp.Body.String())
  }
}

func TestSPARoutesServeShellWhenAuthenticated(t *testing.T) {
  gin.SetMode(gin.TestMode)

  router := testRouter(t, &memoryRepository{assets: []media.Asset{{
    ID:               "asset-1",
    OriginalFilename: "photo.jpg",
    StoredFilename:   "photo.jpg",
    MediaType:        media.MediaTypeImage,
    MIMEType:         "image/jpeg",
    SizeBytes:        7,
    StoragePath:      "20260403/photo.jpg",
    CreatedAt:        time.Now().UTC(),
  }}}, mustLocalStore(t))
  sessionCookie, _ := loginSession(t, router)

  for _, path := range []string{"/media", "/trash", "/media/asset-1"} {
    resp := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, path, nil)
    req.AddCookie(sessionCookie)
    router.ServeHTTP(resp, req)

    if resp.Code != http.StatusOK {
      t.Fatalf("expected %s status 200, got %d", path, resp.Code)
    }
    if !strings.Contains(resp.Body.String(), "id=\"root\"") {
      t.Fatalf("expected %s to return spa shell, got %q", path, resp.Body.String())
    }
  }
}

func TestUploadListDetailAndDownloadFlowOverAPI(t *testing.T) {
  gin.SetMode(gin.TestMode)

  repo := &memoryRepository{}
  store := mustLocalStore(t)
  router := testRouter(t, repo, store)
  sessionCookie, authStatus := loginSession(t, router)

  uploadReq := newUploadRequest(t, "photo.jpg", []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x43, 0x00}, map[string]string{"csrf_token": authStatus.CSRFToken})
  uploadReq.AddCookie(sessionCookie)
  uploadReq.Header.Set("X-CSRF-Token", authStatus.CSRFToken)
  uploadResp := httptest.NewRecorder()
  router.ServeHTTP(uploadResp, uploadReq)
  if uploadResp.Code != http.StatusCreated {
    t.Fatalf("expected upload status 201, got %d with body %q", uploadResp.Code, uploadResp.Body.String())
  }

  var uploadPayload struct {
    Asset    mediaAssetResponse `json:"asset"`
    Existing bool               `json:"existing"`
  }
  if err := json.Unmarshal(uploadResp.Body.Bytes(), &uploadPayload); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }
  if uploadPayload.Existing {
    t.Fatal("expected first upload not existing")
  }

  listResp := httptest.NewRecorder()
  listReq := httptest.NewRequest(http.MethodGet, "/api/media", nil)
  listReq.AddCookie(sessionCookie)
  router.ServeHTTP(listResp, listReq)
  if listResp.Code != http.StatusOK {
    t.Fatalf("expected list status 200, got %d", listResp.Code)
  }
  if !strings.Contains(listResp.Body.String(), "photo.jpg") {
    t.Fatalf("expected list payload to contain uploaded filename, got %q", listResp.Body.String())
  }
  if strings.Contains(listResp.Body.String(), "detailUrl") {
    t.Fatalf("expected list payload to omit detailUrl, got %q", listResp.Body.String())
  }

  detailResp := httptest.NewRecorder()
  detailReq := httptest.NewRequest(http.MethodGet, "/api/media/"+uploadPayload.Asset.ID, nil)
  detailReq.AddCookie(sessionCookie)
  router.ServeHTTP(detailResp, detailReq)
  if detailResp.Code != http.StatusOK {
    t.Fatalf("expected detail status 200, got %d", detailResp.Code)
  }
  if !strings.Contains(detailResp.Body.String(), "/media/"+uploadPayload.Asset.ID+"/view") {
    t.Fatalf("expected detail payload to contain view url, got %q", detailResp.Body.String())
  }
  if strings.Contains(detailResp.Body.String(), "detailUrl") {
    t.Fatalf("expected detail payload to omit detailUrl, got %q", detailResp.Body.String())
  }

  viewResp := httptest.NewRecorder()
  viewReq := httptest.NewRequest(http.MethodGet, "/media/"+uploadPayload.Asset.ID+"/view", nil)
  viewReq.AddCookie(sessionCookie)
  router.ServeHTTP(viewResp, viewReq)
  if viewResp.Code != http.StatusOK {
    t.Fatalf("expected view status 200, got %d", viewResp.Code)
  }
  if got := viewResp.Header().Get("Content-Type"); got != "image/jpeg" {
    t.Fatalf("expected view content type image/jpeg, got %q", got)
  }

  downloadResp := httptest.NewRecorder()
  downloadReq := httptest.NewRequest(http.MethodGet, "/media/"+uploadPayload.Asset.ID+"/download", nil)
  downloadReq.AddCookie(sessionCookie)
  router.ServeHTTP(downloadResp, downloadReq)
  if downloadResp.Code != http.StatusOK {
    t.Fatalf("expected download status 200, got %d", downloadResp.Code)
  }
  if !bytes.Contains(downloadResp.Body.Bytes(), []byte{0xff, 0xd8, 0xff}) {
    t.Fatalf("expected download body to contain stored bytes")
  }

  if len(repo.assets) != 1 {
    t.Fatalf("expected one stored asset, got %d", len(repo.assets))
  }
}

func TestUploadPDFListViewDownloadAndTrashFlowOverAPI(t *testing.T) {
  gin.SetMode(gin.TestMode)

  repo := &memoryRepository{}
  store := mustLocalStore(t)
  router := testRouter(t, repo, store)
  sessionCookie, authStatus := loginSession(t, router)

  pdfBody := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\ntrailer\n<< /Root 1 0 R >>\n%%EOF")
  uploadReq := newUploadRequest(t, "manual.pdf", pdfBody, map[string]string{"csrf_token": authStatus.CSRFToken})
  uploadReq.AddCookie(sessionCookie)
  uploadReq.Header.Set("X-CSRF-Token", authStatus.CSRFToken)
  uploadResp := httptest.NewRecorder()
  router.ServeHTTP(uploadResp, uploadReq)
  if uploadResp.Code != http.StatusCreated {
    t.Fatalf("expected upload status 201, got %d with body %q", uploadResp.Code, uploadResp.Body.String())
  }

  var uploadPayload struct {
    Asset    mediaAssetResponse `json:"asset"`
    Existing bool               `json:"existing"`
  }
  if err := json.Unmarshal(uploadResp.Body.Bytes(), &uploadPayload); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }
  if uploadPayload.Existing {
    t.Fatal("expected first upload not existing")
  }
  if uploadPayload.Asset.MediaType != string(media.MediaTypePDF) {
    t.Fatalf("expected mediaType pdf, got %q", uploadPayload.Asset.MediaType)
  }

  listResp := httptest.NewRecorder()
  listReq := httptest.NewRequest(http.MethodGet, "/api/media", nil)
  listReq.AddCookie(sessionCookie)
  router.ServeHTTP(listResp, listReq)
  if listResp.Code != http.StatusOK {
    t.Fatalf("expected list status 200, got %d", listResp.Code)
  }
  if !strings.Contains(listResp.Body.String(), "manual.pdf") {
    t.Fatalf("expected list payload to contain uploaded filename, got %q", listResp.Body.String())
  }
  if !strings.Contains(listResp.Body.String(), `"mediaType":"pdf"`) {
    t.Fatalf("expected list payload to contain pdf media type, got %q", listResp.Body.String())
  }
  if strings.Contains(listResp.Body.String(), "detailUrl") {
    t.Fatalf("expected list payload to omit detailUrl, got %q", listResp.Body.String())
  }

  detailResp := httptest.NewRecorder()
  detailReq := httptest.NewRequest(http.MethodGet, "/api/media/"+uploadPayload.Asset.ID, nil)
  detailReq.AddCookie(sessionCookie)
  router.ServeHTTP(detailResp, detailReq)
  if detailResp.Code != http.StatusOK {
    t.Fatalf("expected detail status 200, got %d", detailResp.Code)
  }
  if !strings.Contains(detailResp.Body.String(), "manual.pdf") {
    t.Fatalf("expected detail payload to contain pdf filename, got %q", detailResp.Body.String())
  }
  if strings.Contains(detailResp.Body.String(), "detailUrl") {
    t.Fatalf("expected detail payload to omit detailUrl, got %q", detailResp.Body.String())
  }

  showResp := httptest.NewRecorder()
  showReq := httptest.NewRequest(http.MethodGet, "/media/"+uploadPayload.Asset.ID, nil)
  showReq.AddCookie(sessionCookie)
  router.ServeHTTP(showResp, showReq)
  if showResp.Code != http.StatusNotFound {
    t.Fatalf("expected pdf detail page status 404, got %d", showResp.Code)
  }
  if !strings.Contains(showResp.Body.String(), "detail page not available for pdf") {
    t.Fatalf("expected pdf detail page to be unavailable, got %q", showResp.Body.String())
  }

  viewResp := httptest.NewRecorder()
  viewReq := httptest.NewRequest(http.MethodGet, "/media/"+uploadPayload.Asset.ID+"/view", nil)
  viewReq.AddCookie(sessionCookie)
  router.ServeHTTP(viewResp, viewReq)
  if viewResp.Code != http.StatusOK {
    t.Fatalf("expected view status 200, got %d", viewResp.Code)
  }
  if got := viewResp.Header().Get("Content-Type"); got != "application/pdf" {
    t.Fatalf("expected view content type application/pdf, got %q", got)
  }
  if !bytes.Contains(viewResp.Body.Bytes(), []byte("%PDF-1.4")) {
    t.Fatalf("expected view body to contain PDF bytes")
  }

  downloadResp := httptest.NewRecorder()
  downloadReq := httptest.NewRequest(http.MethodGet, "/media/"+uploadPayload.Asset.ID+"/download", nil)
  downloadReq.AddCookie(sessionCookie)
  router.ServeHTTP(downloadResp, downloadReq)
  if downloadResp.Code != http.StatusOK {
    t.Fatalf("expected download status 200, got %d", downloadResp.Code)
  }
  if got := downloadResp.Header().Get("Content-Type"); got != "application/pdf" {
    t.Fatalf("expected download content type application/pdf, got %q", got)
  }
  if !bytes.Contains(downloadResp.Body.Bytes(), []byte("%PDF-1.4")) {
    t.Fatalf("expected download body to contain PDF bytes")
  }

  deleteResp := performCSRFPost(t, router, "/api/media/"+uploadPayload.Asset.ID+"/delete", sessionCookie, authStatus.CSRFToken)
  if deleteResp.Code != http.StatusOK {
    t.Fatalf("expected delete status 200, got %d with body %q", deleteResp.Code, deleteResp.Body.String())
  }

  trashResp := httptest.NewRecorder()
  trashReq := httptest.NewRequest(http.MethodGet, "/api/trash", nil)
  trashReq.AddCookie(sessionCookie)
  router.ServeHTTP(trashResp, trashReq)
  if trashResp.Code != http.StatusOK {
    t.Fatalf("expected trash list status 200, got %d", trashResp.Code)
  }
  if !strings.Contains(trashResp.Body.String(), "manual.pdf") {
    t.Fatalf("expected pdf to appear in trash list, got %q", trashResp.Body.String())
  }
}

func TestDeleteRestoreAndPermanentDeleteFlowOverAPI(t *testing.T) {
  gin.SetMode(gin.TestMode)

  repo := &memoryRepository{}
  store := mustLocalStore(t)
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

  router := testRouter(t, repo, store)
  sessionCookie, authStatus := loginSession(t, router)

  deleteResp := performCSRFPost(t, router, "/api/media/asset-1/delete", sessionCookie, authStatus.CSRFToken)
  if deleteResp.Code != http.StatusOK {
    t.Fatalf("expected delete status 200, got %d with body %q", deleteResp.Code, deleteResp.Body.String())
  }

  trashResp := httptest.NewRecorder()
  trashReq := httptest.NewRequest(http.MethodGet, "/api/trash", nil)
  trashReq.AddCookie(sessionCookie)
  router.ServeHTTP(trashResp, trashReq)
  if trashResp.Code != http.StatusOK {
    t.Fatalf("expected trash list status 200, got %d", trashResp.Code)
  }
  if !strings.Contains(trashResp.Body.String(), "photo.jpg") {
    t.Fatalf("expected deleted asset in trash list, got %q", trashResp.Body.String())
  }

  restoreResp := performCSRFPost(t, router, "/api/media/asset-1/restore", sessionCookie, authStatus.CSRFToken)
  if restoreResp.Code != http.StatusOK {
    t.Fatalf("expected restore status 200, got %d", restoreResp.Code)
  }

  deleteAgainResp := performCSRFPost(t, router, "/api/media/asset-1/delete", sessionCookie, authStatus.CSRFToken)
  if deleteAgainResp.Code != http.StatusOK {
    t.Fatalf("expected second delete status 200, got %d", deleteAgainResp.Code)
  }

  permanentResp := performCSRFPost(t, router, "/api/media/asset-1/permanent-delete", sessionCookie, authStatus.CSRFToken)
  if permanentResp.Code != http.StatusOK {
    t.Fatalf("expected permanent delete status 200, got %d", permanentResp.Code)
  }
  if _, err := store.Open(storedFile.StoragePath); !errors.Is(err, os.ErrNotExist) {
    t.Fatalf("expected physical file to be deleted, got %v", err)
  }
}

func TestUploadReturnsConflictForDeletedDuplicate(t *testing.T) {
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

  router := testRouter(t, repo, mustLocalStore(t))
  sessionCookie, authStatus := loginSession(t, router)

  req := newUploadRequest(t, "photo-copy.jpg", body, map[string]string{"csrf_token": authStatus.CSRFToken})
  req.AddCookie(sessionCookie)
  req.Header.Set("X-CSRF-Token", authStatus.CSRFToken)
  resp := httptest.NewRecorder()
  router.ServeHTTP(resp, req)

  if resp.Code != http.StatusConflict {
    t.Fatalf("expected conflict 409, got %d with body %q", resp.Code, resp.Body.String())
  }
  if !strings.Contains(resp.Body.String(), "trashed_duplicate") {
    t.Fatalf("expected trashed_duplicate payload, got %q", resp.Body.String())
  }
}

func TestUploadRejectsInvalidFileType(t *testing.T) {
  gin.SetMode(gin.TestMode)

  router := testRouter(t, &memoryRepository{}, mustLocalStore(t))
  sessionCookie, authStatus := loginSession(t, router)

  req := newUploadRequest(t, "notes.txt", []byte("plain text"), map[string]string{"csrf_token": authStatus.CSRFToken})
  req.AddCookie(sessionCookie)
  req.Header.Set("X-CSRF-Token", authStatus.CSRFToken)
  resp := httptest.NewRecorder()
  router.ServeHTTP(resp, req)

  if resp.Code != http.StatusUnsupportedMediaType {
    t.Fatalf("expected 415, got %d with body %q", resp.Code, resp.Body.String())
  }
}

func TestThumbnailMissingFileReturnsPlaceholder(t *testing.T) {
  gin.SetMode(gin.TestMode)

  repo := &memoryRepository{assets: []media.Asset{{
    ID:               "asset-1",
    OriginalFilename: "missing.jpg",
    StoredFilename:   "missing.jpg",
    MediaType:        media.MediaTypeImage,
    MIMEType:         "image/jpeg",
    SizeBytes:        10,
    StoragePath:      "20260403/missing.jpg",
    CreatedAt:        time.Now().UTC(),
  }}}
  router := testRouter(t, repo, brokenStore{})
  sessionCookie, _ := loginSession(t, router)

  resp := httptest.NewRecorder()
  req := httptest.NewRequest(http.MethodGet, "/media/asset-1/thumbnail", nil)
  req.AddCookie(sessionCookie)
  router.ServeHTTP(resp, req)

  if resp.Code != http.StatusOK {
    t.Fatalf("expected status 200, got %d", resp.Code)
  }
  if got := resp.Header().Get("Content-Type"); got != "image/svg+xml" {
    t.Fatalf("expected image/svg+xml, got %q", got)
  }
}

func TestDownloadMissingFileReturnsNotFound(t *testing.T) {
  gin.SetMode(gin.TestMode)

  repo := &memoryRepository{assets: []media.Asset{{
    ID:               "asset-1",
    OriginalFilename: "missing.jpg",
    StoredFilename:   "missing.jpg",
    MediaType:        media.MediaTypeImage,
    MIMEType:         "image/jpeg",
    SizeBytes:        10,
    StoragePath:      "20260403/missing.jpg",
    CreatedAt:        time.Now().UTC(),
  }}}
  router := testRouter(t, repo, brokenStore{})
  sessionCookie, _ := loginSession(t, router)

  resp := httptest.NewRecorder()
  req := httptest.NewRequest(http.MethodGet, "/media/asset-1/download", nil)
  req.AddCookie(sessionCookie)
  router.ServeHTTP(resp, req)

  if resp.Code != http.StatusNotFound {
    t.Fatalf("expected status 404, got %d", resp.Code)
  }
}

func testRouter(t *testing.T, repo media.Repository, store media.FileStore) *gin.Engine {
  t.Helper()

  service := media.NewService(repo, store)
  handler := NewHandler(service, 10*1024*1024, testAuth(), testSPAEntry(t))
  return NewRouter(handler)
}

func testSPAEntry(t *testing.T) string {
  t.Helper()

  path := t.TempDir() + "/index.html"
  if err := os.WriteFile(path, []byte(`<!doctype html><html><body><div id="root"></div></body></html>`), 0o644); err != nil {
    t.Fatalf("WriteFile returned error: %v", err)
  }
  return path
}

func mustLocalStore(t *testing.T) local.Store {
  t.Helper()

  store, err := local.New(t.TempDir())
  if err != nil {
    t.Fatalf("local.New returned error: %v", err)
  }
  return store
}

func newUploadRequest(t *testing.T, filename string, content []byte, fields map[string]string) *http.Request {
  t.Helper()

  body := &bytes.Buffer{}
  writer := multipart.NewWriter(body)
  for key, value := range fields {
    if err := writer.WriteField(key, value); err != nil {
      t.Fatalf("WriteField returned error: %v", err)
    }
  }
  part, err := writer.CreateFormFile("file", filename)
  if err != nil {
    t.Fatalf("CreateFormFile returned error: %v", err)
  }
  if _, err := part.Write(content); err != nil {
    t.Fatalf("Write returned error: %v", err)
  }
  if err := writer.Close(); err != nil {
    t.Fatalf("Close returned error: %v", err)
  }

  req := httptest.NewRequest(http.MethodPost, "/api/uploads", body)
  req.Header.Set("Content-Type", writer.FormDataContentType())
  return req
}

func testAuth() *AuthService {
  return NewAuthService("admin", "pass123", "test-secret", 24*time.Hour)
}

func loginSession(t *testing.T, router *gin.Engine) (*http.Cookie, authStatusResponse) {
  t.Helper()

  anonymousResp := httptest.NewRecorder()
  router.ServeHTTP(anonymousResp, httptest.NewRequest(http.MethodGet, "/api/auth/status", nil))
  if anonymousResp.Code != http.StatusOK {
    t.Fatalf("expected anonymous auth status 200, got %d", anonymousResp.Code)
  }

  var anonymous authStatusResponse
  if err := json.Unmarshal(anonymousResp.Body.Bytes(), &anonymous); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }

  loginCSRFCookie := findCookie(t, anonymousResp.Result().Cookies(), loginCSRFCookieName)
  return loginAndGetSession(t, router, anonymous.CSRFToken, loginCSRFCookie)
}

func loginAndGetSession(t *testing.T, router *gin.Engine, csrfToken string, loginCSRFCookie *http.Cookie) (*http.Cookie, authStatusResponse) {
  t.Helper()

  payload, err := json.Marshal(loginRequest{Username: "admin", Password: "pass123", CSRFToken: csrfToken})
  if err != nil {
    t.Fatalf("json.Marshal returned error: %v", err)
  }

  loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewReader(payload))
  loginReq.Header.Set("Content-Type", "application/json")
  loginReq.AddCookie(loginCSRFCookie)

  loginResp := httptest.NewRecorder()
  router.ServeHTTP(loginResp, loginReq)
  if loginResp.Code != http.StatusOK {
    t.Fatalf("expected login status 200, got %d with body %q", loginResp.Code, loginResp.Body.String())
  }

  var status authStatusResponse
  if err := json.Unmarshal(loginResp.Body.Bytes(), &status); err != nil {
    t.Fatalf("json.Unmarshal returned error: %v", err)
  }

  return findCookie(t, loginResp.Result().Cookies(), sessionCookieName), status
}

func performCSRFPost(t *testing.T, router *gin.Engine, path string, sessionCookie *http.Cookie, csrfToken string) *httptest.ResponseRecorder {
  t.Helper()

  req := httptest.NewRequest(http.MethodPost, path, nil)
  req.AddCookie(sessionCookie)
  req.Header.Set("X-CSRF-Token", csrfToken)
  resp := httptest.NewRecorder()
  router.ServeHTTP(resp, req)
  return resp
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
