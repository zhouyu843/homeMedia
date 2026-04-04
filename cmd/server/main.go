package main

import (
	"context"
	"database/sql"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"homeMedia/internal/config"
	apphttp "homeMedia/internal/http"
	"homeMedia/internal/media"
	"homeMedia/internal/repository/postgres"
	"homeMedia/internal/storage/local"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatal(err)
	}

	repository := postgres.NewMediaRepository(db)
	store, err := local.New(cfg.UploadRootDir)
	if err != nil {
		log.Fatal(err)
	}
	service := media.NewService(repository, store)
	authService := apphttp.NewAuthService(
		cfg.AdminUsername,
		cfg.AdminPassword,
		cfg.SessionSecret,
		time.Duration(cfg.SessionTTLHours)*time.Hour,
	)

	templates := template.Must(template.ParseGlob("web/templates/*.html"))
	handler := apphttp.NewHandler(serviceAdapter{service: service}, templates, cfg.MaxUploadSizeMB*1024*1024, authService)
	router := apphttp.NewRouter(handler)

	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on :%s", cfg.AppPort)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

type serviceAdapter struct {
	service media.Service
}

func (a serviceAdapter) Upload(ctx context.Context, input media.UploadInput) (media.Asset, error) {
	return a.service.Upload(ctx, input)
}

func (a serviceAdapter) List(ctx context.Context) ([]media.Asset, error) {
	return a.service.List(ctx)
}

func (a serviceAdapter) Get(ctx context.Context, id string) (media.Asset, error) {
	return a.service.Get(ctx, id)
}

func (a serviceAdapter) Download(ctx context.Context, id string) (media.Asset, io.ReadSeekCloser, error) {
	return a.service.Download(ctx, id)
}

func (a serviceAdapter) Thumbnail(ctx context.Context, id string) (string, []byte, error) {
	return a.service.Thumbnail(ctx, id)
}
