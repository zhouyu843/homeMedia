package main

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"homeMedia/internal/config"
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

	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatal(err)
	}

	repository := postgres.NewMediaRepository(db)
	store, err := local.New(cfg.UploadRootDir)
	if err != nil {
		log.Fatal(err)
	}
	service := media.NewService(repository, store)

	assets, err := service.ListWithoutPreview(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	var scanned, generated, failed int
	for _, asset := range assets {
		if asset.MediaType == media.MediaTypePDF {
			continue
		}

		scanned++

		if err := service.BackfillPreview(context.Background(), asset.ID); err != nil {
			failed++
			log.Printf("preview backfill failed for asset=%s file=%q: %v", asset.ID, asset.OriginalFilename, err)
			continue
		}

		generated++
		log.Printf("preview backfill ok for asset=%s file=%q", asset.ID, asset.OriginalFilename)
	}

	log.Printf(
		"preview backfill finished: scanned=%d generated=%d failed=%d",
		scanned, generated, failed,
	)

	if failed > 0 {
		log.Fatalf("preview backfill completed with %d failures", failed)
	}
}
