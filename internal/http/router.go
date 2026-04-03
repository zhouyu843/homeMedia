package http

import (
	"html/template"

	"github.com/gin-gonic/gin"
)

func NewRouter(handler Handler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.SetHTMLTemplate(template.Must(handler.templates.Clone()))

	router.GET("/", handler.Home)
	router.GET("/media", handler.ListMedia)
	router.GET("/media/:id", handler.ShowMedia)
	router.GET("/media/:id/view", handler.ViewMedia)
	router.GET("/media/:id/download", handler.DownloadMedia)
	router.POST("/uploads", handler.UploadMedia)

	return router
}
