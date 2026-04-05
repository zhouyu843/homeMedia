package http

import (
	"html/template"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

func NewRouter(handler Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), SecurityHeadersMiddleware())
	router.SetHTMLTemplate(template.Must(handler.templates.Clone()))
	router.Static("/static", "./web/static")

	loginLimiter := NewIPRateLimiter(rate.Every(6*time.Second), 5)
	uploadLimiter := NewIPRateLimiter(rate.Every(2*time.Second), 10)

	router.GET("/", handler.Home)
	router.GET("/login", handler.LoginPage)
	router.POST("/login", loginLimiter.Middleware(), handler.LoginSubmit)

	protected := router.Group("/")
	protected.Use(handler.auth.RequireAuth())
	protected.POST("/logout", handler.Logout)
	protected.GET("/media", handler.ListMedia)
	protected.GET("/api/media", handler.ListMediaJSON)
	protected.GET("/media/:id", handler.ShowMedia)
	protected.GET("/media/:id/view", handler.ViewMedia)
	protected.GET("/media/:id/thumbnail", handler.ThumbnailMedia)
	protected.GET("/media/:id/download", handler.DownloadMedia)
	protected.POST("/media/:id/delete", handler.DeleteMedia)
	protected.POST("/api/uploads", uploadLimiter.Middleware(), handler.UploadMediaJSON)
	protected.POST("/uploads", uploadLimiter.Middleware(), handler.UploadMedia)

	return router
}
