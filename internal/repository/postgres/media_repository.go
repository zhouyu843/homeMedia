package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"

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
			content_hash,
			storage_path,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
		asset.ContentHash,
		asset.StoragePath,
		asset.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_media_assets_content_hash" {
			return media.Asset{}, media.ErrDuplicateContentHash
		}
		return media.Asset{}, fmt.Errorf("insert media asset: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindByID(ctx context.Context, id string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at
		FROM media_assets
		WHERE id = $1
	`

	var asset media.Asset
	if err := scanAssetRow(r.db.QueryRowContext(ctx, query, id), &asset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.Asset{}, media.ErrNotFound
		}
		return media.Asset{}, fmt.Errorf("find media asset by id: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindByContentHash(ctx context.Context, contentHash string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at
		FROM media_assets
		WHERE content_hash = $1
	`

	var asset media.Asset
	if err := scanAssetRow(r.db.QueryRowContext(ctx, query, contentHash), &asset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.Asset{}, media.ErrNotFound
		}
		return media.Asset{}, fmt.Errorf("find media asset by content hash: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindWithoutContentHashBySize(ctx context.Context, sizeBytes int64) ([]media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at
		FROM media_assets
		WHERE size_bytes = $1 AND content_hash IS NULL
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, sizeBytes)
	if err != nil {
		return nil, fmt.Errorf("find media assets without content hash by size: %w", err)
	}
	defer rows.Close()

	var assets []media.Asset
	for rows.Next() {
		var asset media.Asset
		if err := scanAssetRow(rows, &asset); err != nil {
			return nil, fmt.Errorf("scan media asset without content hash: %w", err)
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media assets without content hash: %w", err)
	}

	return assets, nil
}

func (r MediaRepository) UpdateContentHash(ctx context.Context, id string, contentHash string) error {
	query := `
		UPDATE media_assets
		SET content_hash = $2
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query, id, contentHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_media_assets_content_hash" {
			return media.ErrDuplicateContentHash
		}
		return fmt.Errorf("update media asset content hash: %w", err)
	}

	return nil
}

func (r MediaRepository) ListRecent(ctx context.Context) ([]media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at
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
		if err := scanAssetRow(rows, &asset); err != nil {
			return nil, fmt.Errorf("scan media asset: %w", err)
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate media assets: %w", err)
	}

	return assets, nil
}

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAssetRow(scanner assetScanner, asset *media.Asset) error {
	return scanner.Scan(
		&asset.ID,
		&asset.OriginalFilename,
		&asset.StoredFilename,
		&asset.MediaType,
		&asset.MIMEType,
		&asset.SizeBytes,
		&asset.ContentHash,
		&asset.StoragePath,
		&asset.CreatedAt,
	)
}
