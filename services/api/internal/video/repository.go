package video

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const videoFields = `id, filename, content_hash, mime_type, size_bytes, source_type, source_path, storage_key, status, created_at, updated_at`

func scanVideo(row interface{ Scan(dest ...any) error }) (*Video, error) {
	var v Video
	err := row.Scan(
		&v.ID,
		&v.Filename,
		&v.ContentHash,
		&v.MimeType,
		&v.SizeBytes,
		&v.SourceType,
		&v.SourcePath,
		&v.StorageKey,
		&v.Status,
		&v.CreatedAt,
		&v.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *Repository) Create(ctx context.Context, filename, contentHash, mimeType string, sizeBytes int64, sourceType SourceType, sourcePath *string) (*Video, error) {
	query := fmt.Sprintf(`
		INSERT INTO videos (filename, content_hash, mime_type, size_bytes, source_type, source_path, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING %s
	`, videoFields)

	row := r.db.QueryRow(ctx, query, filename, contentHash, mimeType, sizeBytes, sourceType, sourcePath, StatusUploading)
	return scanVideo(row)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*Video, error) {
	query := fmt.Sprintf(`SELECT %s FROM videos WHERE id = $1`, videoFields)
	row := r.db.QueryRow(ctx, query, id)
	return scanVideo(row)
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]Video, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := fmt.Sprintf(`SELECT %s FROM videos ORDER BY created_at DESC LIMIT $1 OFFSET $2`, videoFields)
	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Video
	for rows.Next() {
		v, err := scanVideo(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *v)
	}
	if list == nil {
		list = []Video{}
	}
	return list, nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM videos WHERE id = $1`, id)
	return err
}

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (*Video, error) {
	query := fmt.Sprintf(`
		UPDATE videos SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING %s
	`, videoFields)
	row := r.db.QueryRow(ctx, query, id, status)
	return scanVideo(row)
}

func (r *Repository) Count(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM videos`).Scan(&count)
	return count, err
}
