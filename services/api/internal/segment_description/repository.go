package segment_description

import (
	"context"
	"fmt"
	"time"

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
	query := fmt.Sprintf(`
		SELECT %s FROM video_segment_descriptions
		WHERE video_id = $1
		ORDER BY (SELECT start_time FROM video_segments WHERE id = segment_id) ASC, created_at ASC`, fields)
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
type DescriptionWithTimes struct {
    ID           uuid.UUID `json:"id"`
    VideoID      uuid.UUID `json:"video_id"`
    SegmentID    uuid.UUID `json:"segment_id"`
    Description  string    `json:"description"`
    ModelName    string    `json:"model_name"`
    ModelVersion string    `json:"model_version"`
    StartTime    float64   `json:"start_time"`
    EndTime      float64   `json:"end_time"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
func (r *Repository) GetByVideoIDWithSegments(ctx context.Context, videoID uuid.UUID) ([]DescriptionWithTimes, error) {
	query := fmt.Sprintf(`
		SELECT d.id, d.video_id, d.segment_id, d.description, d.model_name, d.model_version,
		       d.created_at, d.updated_at,
		       COALESCE(s.start_time, 0), COALESCE(s.end_time, 0)
		FROM video_segment_descriptions d
		LEFT JOIN video_segments s ON s.id = d.segment_id
		WHERE d.video_id = $1
		ORDER BY s.start_time ASC, d.created_at ASC`)
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []DescriptionWithTimes
	for rows.Next() {
		var d DescriptionWithTimes
		err := rows.Scan(&d.ID, &d.VideoID, &d.SegmentID, &d.Description, &d.ModelName, &d.ModelVersion,
			&d.CreatedAt, &d.UpdatedAt, &d.StartTime, &d.EndTime)
		if err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	if list == nil {
		list = []DescriptionWithTimes{}
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
