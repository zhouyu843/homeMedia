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

	"github.com/gin-gonic/gin"

	"homeMedia/internal/media"
)

type MediaService interface {
	Upload(ctx context.Context, input media.UploadInput) (media.Asset, error)
	List(ctx context.Context) ([]media.Asset, error)
	Get(ctx context.Context, id string) (media.Asset, error)
	Download(ctx context.Context, id string) (media.Asset, io.ReadSeekCloser, error)
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
		"Assets":    assets,
		"CSRFToken": csrfToken,
	}); err != nil {
		c.String(http.StatusInternalServerError, "render list failed")
	}
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

	if !h.auth.VerifySessionCSRF(c.Request, c.PostForm("csrf_token")) {
		c.String(http.StatusForbidden, "invalid csrf token")
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "file is required")
		return
	}

	asset, err := h.uploadFromHeader(c, fileHeader)
	if err != nil {
		h.writeMediaError(c, err)
		return
	}

	c.Redirect(http.StatusSeeOther, "/media/"+asset.ID)
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

func (h Handler) uploadFromHeader(c *gin.Context, fileHeader *multipart.FileHeader) (media.Asset, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return media.Asset{}, err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	bytesRead, readErr := file.Read(buffer)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return media.Asset{}, readErr
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return media.Asset{}, err
	}

	asset, err := h.service.Upload(c.Request.Context(), media.UploadInput{
		OriginalFilename: fileHeader.Filename,
		MIMEType:         http.DetectContentType(buffer[:bytesRead]),
		SizeBytes:        fileHeader.Size,
		Reader:           file,
	})
	if err != nil {
		return media.Asset{}, err
	}

	return asset, nil
}

func (h Handler) writeMediaError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, media.ErrUnsupportedMediaType):
		c.String(http.StatusUnsupportedMediaType, err.Error())
	case errors.Is(err, media.ErrNotFound), errors.Is(err, media.ErrFileMissing):
		c.String(http.StatusNotFound, err.Error())
	case errors.Is(err, media.ErrInvalidStoragePath):
		c.String(http.StatusBadRequest, err.Error())
	default:
		c.String(http.StatusInternalServerError, "internal server error")
	}
}
