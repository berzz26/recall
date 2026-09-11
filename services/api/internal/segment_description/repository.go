package segment_description

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

const fields = `id, video_id, segment_id, description, model_name, model_version, created_at, updated_at`

func scan(row interface{ Scan(dest ...any) error }) (*Description, error) {
	var d Description
	err := row.Scan(&d.ID, &d.VideoID, &d.SegmentID, &d.Description, &d.ModelName, &d.ModelVersion, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repository) Upsert(ctx context.Context, d Description) (*Description, error) {
	query := fmt.Sprintf(`
		INSERT INTO video_segment_descriptions (video_id, segment_id, description, model_name, model_version)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (segment_id, model_name, model_version) DO UPDATE SET
			description = EXCLUDED.description,
			updated_at = now()
		RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, d.VideoID, d.SegmentID, d.Description, d.ModelName, d.ModelVersion)
	return scan(row)
}

func (r *Repository) ReplaceForVideo(ctx context.Context, videoID uuid.UUID, descs []Description) ([]Description, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM video_segment_descriptions WHERE video_id = $1`, videoID); err != nil {
		return nil, err
	}
	var out []Description
	for _, d := range descs {
		query := fmt.Sprintf(`INSERT INTO video_segment_descriptions (video_id, segment_id, description, model_name, model_version) VALUES ($1,$2,$3,$4,$5) RETURNING %s`, fields)
		row := tx.QueryRow(ctx, query, d.VideoID, d.SegmentID, d.Description, d.ModelName, d.ModelVersion)
		saved, err := scan(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *saved)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]Description, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_segment_descriptions WHERE video_id = $1 ORDER BY created_at ASC`, fields)
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Description
	for rows.Next() {
		d, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *d)
	}
	if list == nil {
		list = []Description{}
	}
	return list, rows.Err()
}

func (r *Repository) GetBySegmentID(ctx context.Context, segmentID uuid.UUID) (*Description, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_segment_descriptions WHERE segment_id = $1 ORDER BY model_name, model_version LIMIT 1`, fields)
	row := r.db.QueryRow(ctx, query, segmentID)
	return scan(row)
}

func (r *Repository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_segment_descriptions WHERE video_id = $1`, videoID)
	return err
}
