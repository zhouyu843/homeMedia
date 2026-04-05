package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE id = $1 AND deleted_at IS NULL
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

func (r MediaRepository) FindDeletedByID(ctx context.Context, id string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE id = $1 AND deleted_at IS NOT NULL
	`

	var asset media.Asset
	if err := scanAssetRow(r.db.QueryRowContext(ctx, query, id), &asset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.Asset{}, media.ErrNotFound
		}
		return media.Asset{}, fmt.Errorf("find deleted media asset by id: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindByContentHash(ctx context.Context, contentHash string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE content_hash = $1 AND deleted_at IS NULL
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

func (r MediaRepository) FindDeletedByContentHash(ctx context.Context, contentHash string) (media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE content_hash = $1 AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
		LIMIT 1
	`

	var asset media.Asset
	if err := scanAssetRow(r.db.QueryRowContext(ctx, query, contentHash), &asset); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return media.Asset{}, media.ErrNotFound
		}
		return media.Asset{}, fmt.Errorf("find deleted media asset by content hash: %w", err)
	}

	return asset, nil
}

func (r MediaRepository) FindWithoutContentHashBySize(ctx context.Context, sizeBytes int64) ([]media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE size_bytes = $1 AND content_hash IS NULL AND deleted_at IS NULL
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
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE deleted_at IS NULL
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

func (r MediaRepository) ListTrash(ctx context.Context) ([]media.Asset, error) {
	query := `
		SELECT id, original_filename, stored_filename, media_type, mime_type, size_bytes, COALESCE(content_hash, ''), storage_path, created_at, deleted_at
		FROM media_assets
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC, created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list trashed media assets: %w", err)
	}
	defer rows.Close()

	var assets []media.Asset
	for rows.Next() {
		var asset media.Asset
		if err := scanAssetRow(rows, &asset); err != nil {
			return nil, fmt.Errorf("scan trashed media asset: %w", err)
		}
		assets = append(assets, asset)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trashed media assets: %w", err)
	}

	return assets, nil
}

func (r MediaRepository) SoftDelete(ctx context.Context, id string, deletedAt time.Time) error {
	query := `
		UPDATE media_assets
		SET deleted_at = $2
		WHERE id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, deletedAt)
	if err != nil {
		return fmt.Errorf("soft delete media asset: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("soft delete media asset rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return media.ErrNotFound
	}

	return nil
}

func (r MediaRepository) Restore(ctx context.Context, id string) error {
	query := `
		UPDATE media_assets
		SET deleted_at = NULL
		WHERE id = $1 AND deleted_at IS NOT NULL
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "uq_media_assets_content_hash" {
			return media.ErrDuplicateContentHash
		}
		return fmt.Errorf("restore media asset: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("restore media asset rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return media.ErrNotFound
	}

	return nil
}

func (r MediaRepository) Delete(ctx context.Context, id string) error {
	query := `
		DELETE FROM media_assets
		WHERE id = $1 AND deleted_at IS NOT NULL
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete media asset: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete media asset rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return media.ErrNotFound
	}

	return nil
}

func (r MediaRepository) CountByStoragePath(ctx context.Context, storagePath string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM media_assets
		WHERE storage_path = $1 AND deleted_at IS NULL
	`

	var count int
	if err := r.db.QueryRowContext(ctx, query, storagePath).Scan(&count); err != nil {
		return 0, fmt.Errorf("count media assets by storage path: %w", err)
	}

	return count, nil
}

type assetScanner interface {
	Scan(dest ...any) error
}

func scanAssetRow(scanner assetScanner, asset *media.Asset) error {
	var deletedAt sql.NullTime

	err := scanner.Scan(
		&asset.ID,
		&asset.OriginalFilename,
		&asset.StoredFilename,
		&asset.MediaType,
		&asset.MIMEType,
		&asset.SizeBytes,
		&asset.ContentHash,
		&asset.StoragePath,
		&asset.CreatedAt,
		&deletedAt,
	)
	if err != nil {
		return err
	}

	asset.DeletedAt = nil
	if deletedAt.Valid {
		deletedAtValue := deletedAt.Time
		asset.DeletedAt = &deletedAtValue
	}

	return nil
}
