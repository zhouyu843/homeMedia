package http

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func NewRouter(handler Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), SecurityHeadersMiddleware())
	router.Static("/static", "./web/static")
	router.StaticFile("/favicon.svg", "./web/static/app/favicon.svg")
	redirectFavicon := func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/favicon.svg")
	}
	router.GET("/favicon.ico", redirectFavicon)
	router.HEAD("/favicon.ico", redirectFavicon)

	loginLimiter := NewIPRateLimiter(rate.Every(6*time.Second), 5)
	uploadLimiter := NewIPRateLimiter(rate.Every(2*time.Second), 10)

	router.GET("/", handler.Home)
	router.GET("/login", handler.LoginPage)
	router.GET("/api/auth/status", handler.AuthStatus)
	router.POST("/api/login", loginLimiter.Middleware(), handler.LoginJSON)

	protected := router.Group("/")
	protected.Use(handler.auth.RequireAuth())
	protected.POST("/api/logout", handler.LogoutJSON)
	protected.GET("/media", handler.ListMedia)
	protected.GET("/trash", handler.ListTrash)
	protected.GET("/api/trash", handler.ListTrashJSON)
	protected.GET("/trash/:id/thumbnail", handler.TrashThumbnailMedia)
	protected.GET("/api/media", handler.ListMediaJSON)
	protected.GET("/api/media/:id", handler.GetMediaJSON)
	protected.GET("/media/:id", handler.ShowMedia)
	protected.GET("/media/:id/view", handler.ViewMedia)
	protected.GET("/media/:id/preview", handler.PreviewMedia)
	protected.GET("/media/:id/thumbnail", handler.ThumbnailMedia)
	protected.GET("/media/:id/download", handler.DownloadMedia)
	protected.POST("/api/media/:id/delete", handler.DeleteMediaJSON)
	protected.POST("/api/media/:id/restore", handler.RestoreMediaJSON)
	protected.POST("/api/media/:id/permanent-delete", handler.DeleteMediaPermanentlyJSON)
	protected.POST("/api/trash/empty", handler.EmptyTrashJSON)
	protected.POST("/api/uploads", uploadLimiter.Middleware(), handler.UploadMediaJSON)

	return router
}
