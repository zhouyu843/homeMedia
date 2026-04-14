package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"homeMedia/internal/media"
)

type MediaService interface {
	Upload(ctx context.Context, input media.UploadInput) (media.UploadResult, error)
	List(ctx context.Context) ([]media.Asset, error)
	ListTrash(ctx context.Context) ([]media.Asset, error)
	Get(ctx context.Context, id string) (media.Asset, error)
	PlaybackWarning(ctx context.Context, asset media.Asset) *media.PlaybackWarning
	Download(ctx context.Context, id string) (media.Asset, io.ReadSeekCloser, error)
	Thumbnail(ctx context.Context, id string) (string, []byte, error)
	TrashThumbnail(ctx context.Context, id string) (string, []byte, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	DeletePermanently(ctx context.Context, id string) error
	EmptyTrash(ctx context.Context) error
}

type Handler struct {
	service        MediaService
	maxUploadBytes int64
	auth           *AuthService
	spaEntryPath   string
}

func NewHandler(service MediaService, maxUploadBytes int64, auth *AuthService, spaEntryPath string) Handler {
	return Handler{
		service:        service,
		maxUploadBytes: maxUploadBytes,
		auth:           auth,
		spaEntryPath:   spaEntryPath,
	}
}

func (h Handler) Home(c *gin.Context) {
	if h.auth.IsAuthenticated(c.Request) {
		c.Redirect(http.StatusSeeOther, "/media")
		return
	}

	c.Redirect(http.StatusSeeOther, "/login")
}

func (h Handler) LoginPage(c *gin.Context) {
	if h.auth.IsAuthenticated(c.Request) {
		c.Redirect(http.StatusSeeOther, "/media")
		return
	}

	h.serveSPA(c)
}

func (h Handler) AuthStatus(c *gin.Context) {
	username, authenticated := h.auth.CurrentUser(c.Request)
	csrfToken := ""

	if authenticated {
		var ok bool
		csrfToken, ok = h.auth.SessionCSRFToken(c.Request)
		if !ok {
			c.JSON(http.StatusUnauthorized, apiErrorResponse{Code: "unauthorized", Message: "authentication required"})
			return
		}
	} else {
		var err error
		csrfToken, err = h.auth.IssueLoginCSRF(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, apiErrorResponse{Code: "internal_error", Message: "issue csrf failed"})
			return
		}
	}

	c.JSON(http.StatusOK, authStatusResponse{
		Authenticated:    authenticated,
		Username:         username,
		CSRFToken:        csrfToken,
		MaxUploadBytes:   h.maxUploadBytes,
		AllowedMimeTypes: media.AllowedUploadMIMETypes(),
	})
}

func (h Handler) LoginJSON(c *gin.Context) {
	if h.auth.IsAuthenticated(c.Request) {
		username, _ := h.auth.CurrentUser(c.Request)
		csrfToken, _ := h.auth.SessionCSRFToken(c.Request)
		c.JSON(http.StatusOK, authStatusResponse{
			Authenticated:    true,
			Username:         username,
			CSRFToken:        csrfToken,
			MaxUploadBytes:   h.maxUploadBytes,
			AllowedMimeTypes: media.AllowedUploadMIMETypes(),
		})
		return
	}

	var input loginRequest
	if err := json.NewDecoder(c.Request.Body).Decode(&input); err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: "invalid_payload", Message: "invalid login payload"})
		return
	}

	if !h.auth.VerifyLoginCSRF(c.Request, input.CSRFToken) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	if !h.auth.AuthenticateCredentials(input.Username, input.Password) {
		c.JSON(http.StatusUnauthorized, apiErrorResponse{Code: "invalid_credentials", Message: "用户名或密码错误"})
		return
	}

	csrfToken := h.auth.StartSession(c, input.Username)
	h.auth.ClearLoginCSRF(c)
	c.JSON(http.StatusOK, authStatusResponse{
		Authenticated:    true,
		Username:         input.Username,
		CSRFToken:        csrfToken,
		MaxUploadBytes:   h.maxUploadBytes,
		AllowedMimeTypes: media.AllowedUploadMIMETypes(),
	})
}

func (h Handler) LogoutJSON(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	h.auth.EndSession(c)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h Handler) ListMedia(c *gin.Context) {
	h.serveSPA(c)
}

func (h Handler) ListTrash(c *gin.Context) {
	h.serveSPA(c)
}

func (h Handler) ListMediaJSON(c *gin.Context) {
	assets, err := h.service.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErrorResponse{Code: "internal_error", Message: "list media failed"})
		return
	}

	result := make([]mediaAssetResponse, 0, len(assets))
	for _, asset := range assets {
		result = append(result, toMediaAssetResponse(asset))
	}

	c.JSON(http.StatusOK, gin.H{"assets": result})
}

func (h Handler) ListTrashJSON(c *gin.Context) {
	assets, err := h.service.ListTrash(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiErrorResponse{Code: "internal_error", Message: "list trash failed"})
		return
	}

	result := make([]mediaAssetResponse, 0, len(assets))
	for _, asset := range assets {
		result = append(result, toTrashMediaAssetResponse(asset))
	}

	c.JSON(http.StatusOK, gin.H{"assets": result})
}

func (h Handler) GetMediaJSON(c *gin.Context) {
	asset, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	response := toMediaAssetResponse(asset)
	if warning := h.service.PlaybackWarning(c.Request.Context(), asset); warning != nil {
		response.PlaybackWarning = &playbackWarningResponse{
			Code:    warning.Code,
			Message: warning.Message,
		}
	}

	c.JSON(http.StatusOK, gin.H{"asset": response})
}

func (h Handler) ShowMedia(c *gin.Context) {
	asset, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMediaError(c, err)
		return
	}

	if asset.MediaType == media.MediaTypePDF {
		c.String(http.StatusNotFound, "detail page not available for pdf")
		return
	}

	h.serveSPA(c)
}

func (h Handler) UploadMediaJSON(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes)
	if err := c.Request.ParseMultipartForm(h.maxUploadBytes); err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: "invalid_payload", Message: "invalid upload payload"})
		return
	}

	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: "missing_file", Message: "file is required"})
		return
	}

	result, err := h.uploadFromHeader(c, fileHeader, h.uploadDuplicateAction(c))
	if err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	if result.RequiresDecision {
		c.JSON(http.StatusConflict, gin.H{
			"code":    "trashed_duplicate",
			"message": "发现回收站中的同内容文件，请选择恢复旧项或继续新建",
			"asset":   toMediaAssetResponse(result.DecisionAsset),
		})
		return
	}

	status := http.StatusCreated
	if result.Existing || result.Restored {
		status = http.StatusOK
	}

	c.JSON(status, gin.H{
		"asset":    toMediaAssetResponse(result.Asset),
		"existing": result.Existing,
		"restored": result.Restored,
	})
}

func (h Handler) DeleteMediaJSON(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h Handler) RestoreMediaJSON(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	if err := h.service.Restore(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h Handler) DeleteMediaPermanentlyJSON(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	if err := h.service.DeletePermanently(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h Handler) EmptyTrashJSON(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, h.requestCSRFToken(c)) {
		c.JSON(http.StatusForbidden, apiErrorResponse{Code: "invalid_csrf", Message: "invalid csrf token"})
		return
	}

	if err := h.service.EmptyTrash(c.Request.Context()); err != nil {
		h.writeMediaErrorJSON(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h Handler) DownloadMedia(c *gin.Context) {
	asset, file, err := h.service.Download(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMediaError(c, err)
		return
	}
	defer file.Close()

	c.Header("Content-Type", asset.MIMEType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", strconv.Quote(asset.OriginalFilename)))
	http.ServeContent(c.Writer, c.Request, asset.OriginalFilename, asset.CreatedAt, file)
}

func (h Handler) ViewMedia(c *gin.Context) {
	asset, file, err := h.service.Download(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMediaError(c, err)
		return
	}
	defer file.Close()

	c.Header("Content-Type", asset.MIMEType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=%s", strconv.Quote(asset.OriginalFilename)))
	http.ServeContent(c.Writer, c.Request, asset.OriginalFilename, asset.CreatedAt, file)
}

func (h Handler) ThumbnailMedia(c *gin.Context) {
	contentType, thumbnail, err := h.service.Thumbnail(c.Request.Context(), c.Param("id"))
	h.respondThumbnail(c, contentType, thumbnail, err)
}

func (h Handler) TrashThumbnailMedia(c *gin.Context) {
	contentType, thumbnail, err := h.service.TrashThumbnail(c.Request.Context(), c.Param("id"))
	h.respondThumbnail(c, contentType, thumbnail, err)
}

func (h Handler) respondThumbnail(c *gin.Context, contentType string, thumbnail []byte, err error) {
	if err != nil {
		if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrFileMissing) || errors.Is(err, media.ErrThumbnailGeneration) {
			c.Header("Cache-Control", "private, max-age=60")
			c.Data(http.StatusOK, "image/svg+xml", placeholderThumbnailSVG())
			return
		}
		h.writeMediaError(c, err)
		return
	}

	c.Header("Cache-Control", "private, max-age=600")
	c.Data(http.StatusOK, contentType, thumbnail)
}

func placeholderThumbnailSVG() []byte {
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 360 270" role="img" aria-label="preview unavailable"><rect width="360" height="270" fill="#e2e8f0"/><rect x="24" y="24" width="312" height="222" rx="16" fill="#cbd5e1"/><text x="180" y="146" text-anchor="middle" font-family="Arial, sans-serif" font-size="24" fill="#475569" letter-spacing="2">PREVIEW</text></svg>`)
}

func (h Handler) uploadFromHeader(c *gin.Context, fileHeader *multipart.FileHeader, duplicateAction media.DuplicateAction) (media.UploadResult, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return media.UploadResult{}, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	bytesRead, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return media.UploadResult{}, readErr
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return media.UploadResult{}, err
	}

	result, err := h.service.Upload(c.Request.Context(), media.UploadInput{
		OriginalFilename: fileHeader.Filename,
		MIMEType:         http.DetectContentType(buffer[:bytesRead]),
		SizeBytes:        fileHeader.Size,
		Reader:           file,
		DuplicateAction:  duplicateAction,
	})
	if err != nil {
		return media.UploadResult{}, err
	}

	return result, nil
}

func (h Handler) writeMediaError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, media.ErrUnsupportedMediaType):
		c.String(http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, media.ErrDuplicateContentHash):
		c.String(http.StatusConflict, err.Error())
	case errors.Is(err, media.ErrNotFound), errors.Is(err, media.ErrFileMissing):
		c.String(http.StatusNotFound, err.Error())
	case errors.Is(err, media.ErrInvalidStoragePath):
		c.String(http.StatusBadRequest, err.Error())
	case errors.Is(err, media.ErrThumbnailGeneration):
		c.String(http.StatusInternalServerError, err.Error())
	default:
		c.String(http.StatusInternalServerError, "internal server error")
	}
}

func (h Handler) writeMediaErrorJSON(c *gin.Context, err error) {
	switch {
	case errors.Is(err, media.ErrUnsupportedMediaType):
		c.JSON(http.StatusUnsupportedMediaType, apiErrorResponse{Code: "unsupported_media_type", Message: err.Error()})
	case errors.Is(err, media.ErrDuplicateContentHash):
		c.JSON(http.StatusConflict, apiErrorResponse{Code: "duplicate_content_hash", Message: err.Error()})
	case errors.Is(err, media.ErrNotFound), errors.Is(err, media.ErrFileMissing):
		c.JSON(http.StatusNotFound, apiErrorResponse{Code: "not_found", Message: err.Error()})
	case errors.Is(err, media.ErrInvalidStoragePath):
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: "invalid_storage_path", Message: err.Error()})
	case errors.Is(err, media.ErrThumbnailGeneration):
		c.JSON(http.StatusInternalServerError, apiErrorResponse{Code: "thumbnail_generation_failed", Message: err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, apiErrorResponse{Code: "internal_error", Message: "internal server error"})
	}
}

func (h Handler) requestCSRFToken(c *gin.Context) string {
	token := c.GetHeader("X-CSRF-Token")
	if token != "" {
		return token
	}
	return c.PostForm("csrf_token")
}

func (h Handler) serveSPA(c *gin.Context) {
	c.File(h.spaEntryPath)
}

func (h Handler) uploadDuplicateAction(c *gin.Context) media.DuplicateAction {
	action := media.DuplicateAction(c.PostForm("trashed_duplicate_action"))
	if action == media.DuplicateActionRestore || action == media.DuplicateActionNew {
		return action
	}
	return media.DuplicateActionPrompt
}

type apiErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type loginRequest struct {
	Username  string `json:"username"`
	Password  string `json:"password"`
	CSRFToken string `json:"csrfToken"`
}

type authStatusResponse struct {
	Authenticated    bool     `json:"authenticated"`
	Username         string   `json:"username,omitempty"`
	CSRFToken        string   `json:"csrfToken"`
	MaxUploadBytes   int64    `json:"maxUploadBytes"`
	AllowedMimeTypes []string `json:"allowedMimeTypes"`
}

type mediaAssetResponse struct {
	ID               string `json:"id"`
	OriginalFilename string `json:"originalFilename"`
	MediaType        string `json:"mediaType"`
	MIMEType         string `json:"mimeType"`
	SizeBytes        int64  `json:"sizeBytes"`
	CreatedAt        string `json:"createdAt"`
	DeletedAt        string `json:"deletedAt,omitempty"`
	ViewURL          string `json:"viewUrl"`
	ThumbnailURL     string `json:"thumbnailUrl"`
	DownloadURL      string `json:"downloadUrl"`
	PlaybackWarning  *playbackWarningResponse `json:"playbackWarning,omitempty"`
}

type playbackWarningResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toMediaAssetResponse(asset media.Asset) mediaAssetResponse {
	basePath := "/media/" + asset.ID
	response := mediaAssetResponse{
		ID:               asset.ID,
		OriginalFilename: asset.OriginalFilename,
		MediaType:        string(asset.MediaType),
		MIMEType:         asset.MIMEType,
		SizeBytes:        asset.SizeBytes,
		CreatedAt:        asset.CreatedAt.UTC().Format(time.RFC3339),
		ViewURL:          basePath + "/view",
		ThumbnailURL:     basePath + "/thumbnail",
		DownloadURL:      basePath + "/download",
	}

	if asset.DeletedAt != nil {
		response.DeletedAt = asset.DeletedAt.UTC().Format(time.RFC3339)
	}

	return response
}

func toTrashMediaAssetResponse(asset media.Asset) mediaAssetResponse {
	response := toMediaAssetResponse(asset)
	response.ThumbnailURL = "/trash/" + asset.ID + "/thumbnail"
	return response
}
