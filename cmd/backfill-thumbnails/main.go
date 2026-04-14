package main

import (
	"context"
	"database/sql"
	"flag"
	"io"
	"log"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"homeMedia/internal/config"
	"homeMedia/internal/media"
	"homeMedia/internal/repository/postgres"
	"homeMedia/internal/storage/local"
)

type backfillStats struct {
	Scanned   int
	Generated int
	Skipped   int
	Failed    int
	Trash     int
}

func main() {
	includeTrash := flag.Bool("include-trash", false, "also backfill trashed assets")
	mediaTypesFlag := flag.String("media-types", "video", "comma-separated media types to backfill: image,video,pdf")
	flag.Parse()

	selectedMediaTypes := parseMediaTypes(*mediaTypesFlag)
	if len(selectedMediaTypes) == 0 {
		log.Fatal("no valid media types selected")
	}

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

	stats, err := backfillActiveAssets(context.Background(), service, store, selectedMediaTypes)
	if err != nil {
		log.Fatal(err)
	}
	if *includeTrash {
		trashStats, trashErr := backfillTrashAssets(context.Background(), service, store, selectedMediaTypes)
		stats.Scanned += trashStats.Scanned
		stats.Generated += trashStats.Generated
		stats.Skipped += trashStats.Skipped
		stats.Failed += trashStats.Failed
		stats.Trash += trashStats.Scanned
		if trashErr != nil {
			log.Fatal(trashErr)
		}
	}

	log.Printf(
		"thumbnail backfill finished: scanned=%d generated=%d skipped=%d failed=%d trash=%d",
		stats.Scanned,
		stats.Generated,
		stats.Skipped,
		stats.Failed,
		stats.Trash,
	)

	if stats.Failed > 0 {
		log.Fatalf("thumbnail backfill completed with %d failures", stats.Failed)
	}
}

func backfillActiveAssets(ctx context.Context, service media.Service, store media.FileStore, selectedMediaTypes map[media.MediaType]struct{}) (backfillStats, error) {
	assets, err := service.List(ctx)
	if err != nil {
		return backfillStats{}, err
	}

	return backfillAssets(ctx, assets, service.Thumbnail, store, selectedMediaTypes)
}

func backfillTrashAssets(ctx context.Context, service media.Service, store media.FileStore, selectedMediaTypes map[media.MediaType]struct{}) (backfillStats, error) {
	assets, err := service.ListTrash(ctx)
	if err != nil {
		return backfillStats{}, err
	}

	return backfillAssets(ctx, assets, service.TrashThumbnail, store, selectedMediaTypes)
}

func backfillAssets(
	ctx context.Context,
	assets []media.Asset,
	thumbnailFunc func(context.Context, string) (string, []byte, error),
	store media.FileStore,
	selectedMediaTypes map[media.MediaType]struct{},
) (backfillStats, error) {
	stats := backfillStats{}

	for _, asset := range assets {
		if _, ok := selectedMediaTypes[asset.MediaType]; !ok {
			continue
		}

		stats.Scanned++
		if hasUsableThumbnail(store, asset.ThumbnailStoragePath) {
			stats.Skipped++
			continue
		}

		if _, _, err := thumbnailFunc(ctx, asset.ID); err != nil {
			stats.Failed++
			log.Printf("thumbnail backfill failed for asset=%s file=%q: %v", asset.ID, asset.OriginalFilename, err)
			continue
		}

		stats.Generated++
	}

	return stats, nil
}

func hasUsableThumbnail(store media.FileStore, thumbnailStoragePath string) bool {
	if thumbnailStoragePath == "" {
		return false
	}

	file, err := store.Open(thumbnailStoragePath)
	if err != nil {
		return false
	}
	defer file.Close()

	buffer := make([]byte, 1)
	bytesRead, readErr := file.Read(buffer)
	return bytesRead > 0 || readErr == nil || readErr == io.EOF
}

func parseMediaTypes(raw string) map[media.MediaType]struct{} {
	selected := map[media.MediaType]struct{}{}
	for _, value := range strings.Split(raw, ",") {
		switch media.MediaType(strings.TrimSpace(strings.ToLower(value))) {
		case media.MediaTypeImage:
			selected[media.MediaTypeImage] = struct{}{}
		case media.MediaTypeVideo:
			selected[media.MediaTypeVideo] = struct{}{}
		case media.MediaTypePDF:
			selected[media.MediaTypePDF] = struct{}{}
		}
	}
	return selected
}