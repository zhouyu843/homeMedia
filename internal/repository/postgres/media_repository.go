package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"homeMedia/internal/media"
)

type MediaRepository struct {
	db *sql.DB
}

func NewMediaRepository(db *sql.DB) MediaRepository {
	return MediaRepository{db: db}
}

func (r MediaRepository) Save(ctx context.Context, asset media.Asset) (media.Asset, error) {
	query := `
		INSERT INTO media_assets (
			id,
			original_filename,
			stored_filename,
			media_type,
			mime_type,
			size_bytes,
			storage_path,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(
		ctx,
		query,
		asset.ID,
		asset.OriginalFilename,
		asset.StoredFilename,
		asset.MediaType,
		asset.MIMEType,
		asset.SizeBytes,
		asset.StoragePath,
		asset.CreatedAt,
	)
	if err != nil {
		return media.Asset{}, fmt.Errorf("insert media asset: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindByID(ctx context.Context, id string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, storage_path, created_at
		FROM media_assets
		WHERE id = $1
	`

	var asset media.Asset
	if err := r.db.QueryRowContext(ctx, query, id).Scan(
		&asset.ID,
		&asset.OriginalFilename,
		&asset.StoredFilename,
		&asset.MediaType,
		&asset.MIMEType,
		&asset.SizeBytes,
		&asset.StoragePath,
		&asset.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.Asset{}, media.ErrNotFound
		}
		return media.Asset{}, fmt.Errorf("find media asset by id: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) ListRecent(ctx context.Context) ([]media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, storage_path, created_at
		FROM media_assets
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list media assets: %w", err)
	}
	defer rows.Close()

	var assets []media.Asset
	for rows.Next() {
		var asset media.Asset
		if err := rows.Scan(
			&asset.ID,
			&asset.OriginalFilename,
			&asset.StoredFilename,
			&asset.MediaType,
			&asset.MIMEType,
			&asset.SizeBytes,
			&asset.StoragePath,
			&asset.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan media asset: %w", err)
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media assets: %w", err)
	}

	return assets, nil
}
