package http

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"homeMedia/internal/media"
)

type MediaService interface {
	Upload(ctx context.Context, input media.UploadInput) (media.UploadResult, error)
	List(ctx context.Context) ([]media.Asset, error)
	ListTrash(ctx context.Context) ([]media.Asset, error)
	Get(ctx context.Context, id string) (media.Asset, error)
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
	templates      *template.Template
	maxUploadBytes int64
	auth           *AuthService
}

func NewHandler(service MediaService, templates *template.Template, maxUploadBytes int64, auth *AuthService) Handler {
	return Handler{
		service:        service,
		templates:      templates,
		maxUploadBytes: maxUploadBytes,
		auth:           auth,
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

	h.renderLogin(c, http.StatusOK, "")
}

func (h Handler) LoginSubmit(c *gin.Context) {
	if h.auth.IsAuthenticated(c.Request) {
		c.Redirect(http.StatusSeeOther, "/media")
		return
	}

	if !h.auth.VerifyLoginCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	username := c.PostForm("username")
	password := c.PostForm("password")
	if !h.auth.AuthenticateCredentials(username, password) {
		h.renderLogin(c, http.StatusUnauthorized, "用户名或密码错误")
		return
	}

	h.auth.StartSession(c, username)
	h.auth.ClearLoginCSRF(c)
	c.Redirect(http.StatusSeeOther, "/media")
}

func (h Handler) Logout(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	h.auth.EndSession(c)
	c.Redirect(http.StatusSeeOther, "/login")
}

func (h Handler) ListMedia(c *gin.Context) {
	assets, err := h.service.List(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "list media failed")
		return
	}

	csrfToken, ok := h.auth.SessionCSRFToken(c.Request)
	if !ok {
		c.String(http.StatusUnauthorized, "unauthorized")
		return
	}

	c.Status(http.StatusOK)
	if err := h.templates.ExecuteTemplate(c.Writer, "list.html", gin.H{
		"Assets":              assets,
		"CSRFToken":           csrfToken,
		"MaxUploadBytes":      h.maxUploadBytes,
		"AllowedMIMETypes":    media.AllowedUploadMIMETypes(),
		"AllowedMIMETypesCSV": strings.Join(media.AllowedUploadMIMETypes(), ","),
	}); err != nil {
		c.String(http.StatusInternalServerError, "render list failed")
	}
}

func (h Handler) ListTrash(c *gin.Context) {
	assets, err := h.service.ListTrash(c.Request.Context())
	if err != nil {
		c.String(http.StatusInternalServerError, "list trash failed")
		return
	}

	csrfToken, ok := h.auth.SessionCSRFToken(c.Request)
	if !ok {
		c.String(http.StatusUnauthorized, "unauthorized")
		return
	}

	c.Status(http.StatusOK)
	if err := h.templates.ExecuteTemplate(c.Writer, "trash.html", gin.H{
		"Assets":     assets,
		"CSRFToken": csrfToken,
	}); err != nil {
		c.String(http.StatusInternalServerError, "render trash failed")
	}
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

func (h Handler) ShowMedia(c *gin.Context) {
	asset, err := h.service.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeMediaError(c, err)
		return
	}

	csrfToken, ok := h.auth.SessionCSRFToken(c.Request)
	if !ok {
		c.String(http.StatusUnauthorized, "unauthorized")
		return
	}

	c.Status(http.StatusOK)
	if err := h.templates.ExecuteTemplate(c.Writer, "detail.html", gin.H{
		"Asset":     asset,
		"CSRFToken": csrfToken,
	}); err != nil {
		c.String(http.StatusInternalServerError, "render detail failed")
	}
}

func (h Handler) UploadMedia(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes)
	if err := c.Request.ParseMultipartForm(h.maxUploadBytes); err != nil {
		c.String(http.StatusBadRequest, "invalid upload payload")
		return
	}

	if !h.auth.VerifySessionCSRF(c.Request, h.uploadCSRFToken(c)) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}

	result, err := h.uploadFromHeader(c, fileHeader, media.DuplicateActionNew)
	if err != nil {
		h.writeMediaError(c, err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/media/"+result.Asset.ID)
}

func (h Handler) UploadMediaJSON(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxUploadBytes)
	if err := c.Request.ParseMultipartForm(h.maxUploadBytes); err != nil {
		c.JSON(http.StatusBadRequest, apiErrorResponse{Code: "invalid_payload", Message: "invalid upload payload"})
		return
	}

	if !h.auth.VerifySessionCSRF(c.Request, h.uploadCSRFToken(c)) {
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
	if result.Existing {
		status = http.StatusOK
	} else if result.Restored {
		status = http.StatusOK
	}

	c.JSON(status, gin.H{
		"asset":    toMediaAssetResponse(result.Asset),
		"existing": result.Existing,
		"restored": result.Restored,
	})
}

func (h Handler) DeleteMedia(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	if err := h.service.Delete(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaError(c, err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/media")
}

func (h Handler) RestoreMedia(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	if err := h.service.Restore(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaError(c, err)
		return
	}

	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	c.Redirect(http.StatusSeeOther, "/trash")
}

func (h Handler) DeleteMediaPermanently(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	if err := h.service.DeletePermanently(c.Request.Context(), c.Param("id")); err != nil {
		h.writeMediaError(c, err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/trash")
}

func (h Handler) EmptyTrash(c *gin.Context) {
	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	if err := h.service.EmptyTrash(c.Request.Context()); err != nil {
		h.writeMediaError(c, err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/trash")
}

func (h Handler) renderLogin(c *gin.Context, status int, errMsg string) {
	csrfToken, err := h.auth.IssueLoginCSRF(c)
	if err != nil {
		c.String(http.StatusInternalServerError, "issue csrf failed")
		return
	}

	c.Status(status)
	if err := h.templates.ExecuteTemplate(c.Writer, "login.html", gin.H{
		"Error":     errMsg,
		"CSRFToken": csrfToken,
	}); err != nil {
		c.String(http.StatusInternalServerError, "render login failed")
	}
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

func (h Handler) uploadCSRFToken(c *gin.Context) string {
	token := c.GetHeader("X-CSRF-Token")
	if token != "" {
		return token
	}
	return c.PostForm("csrf_token")
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

type mediaAssetResponse struct {
	ID               string `json:"id"`
	OriginalFilename string `json:"originalFilename"`
	MediaType        string `json:"mediaType"`
	MIMEType         string `json:"mimeType"`
	SizeBytes        int64  `json:"sizeBytes"`
	CreatedAt        string `json:"createdAt"`
	DetailURL        string `json:"detailUrl"`
	ViewURL          string `json:"viewUrl"`
	ThumbnailURL     string `json:"thumbnailUrl"`
	DownloadURL      string `json:"downloadUrl"`
}

func toMediaAssetResponse(asset media.Asset) mediaAssetResponse {
	basePath := "/media/" + asset.ID
	return mediaAssetResponse{
		ID:               asset.ID,
		OriginalFilename: asset.OriginalFilename,
		MediaType:        string(asset.MediaType),
		MIMEType:         asset.MIMEType,
		SizeBytes:        asset.SizeBytes,
		CreatedAt:        asset.CreatedAt.UTC().Format(time.RFC3339),
		DetailURL:        basePath,
		ViewURL:          basePath + "/view",
		ThumbnailURL:     basePath + "/thumbnail",
		DownloadURL:      basePath + "/download",
	}
}
