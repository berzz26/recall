package video_segment

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

const fields = `id, video_id, segment_index, start_time, end_time, duration, created_at, updated_at`

func scan(row interface{ Scan(dest ...any) error }) (*VideoSegment, error) {
	var s VideoSegment
	err := row.Scan(&s.ID, &s.VideoID, &s.SegmentIndex, &s.StartTime, &s.EndTime, &s.Duration, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]VideoSegment, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_segments WHERE video_id = $1 ORDER BY segment_index ASC`, fields)
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []VideoSegment
	for rows.Next() {
		s, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *s)
	}
	if list == nil {
		list = []VideoSegment{}
	}
	return list, rows.Err()
}

func (r *Repository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_segments WHERE video_id = $1`, videoID)
	return err
}

func (r *Repository) ReplaceForVideo(ctx context.Context, videoID uuid.UUID, segments []VideoSegment) ([]VideoSegment, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM video_segments WHERE video_id = $1`, videoID); err != nil {
		return nil, err
	}

	var result []VideoSegment
	for _, seg := range segments {
		query := fmt.Sprintf(`INSERT INTO video_segments (video_id, segment_index, start_time, end_time, duration) VALUES ($1,$2,$3,$4,$5) RETURNING %s`, fields)
		row := tx.QueryRow(ctx, query, seg.VideoID, seg.SegmentIndex, seg.StartTime, seg.EndTime, seg.Duration)
		s, err := scan(row)
		if err != nil {
			return nil, err
		}
		result = append(result, *s)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
