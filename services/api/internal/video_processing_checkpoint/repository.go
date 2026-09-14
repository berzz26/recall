package video_processing_checkpoint

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

const fields = `id, video_id, segment_id, status, error, created_at, updated_at, completed_at`
const fieldsPrefixed = `c.id, c.video_id, c.segment_id, c.status, c.error, c.created_at, c.updated_at, c.completed_at`

func scan(row interface{ Scan(dest ...any) error }) (*Checkpoint, error) {
	var c Checkpoint
	err := row.Scan(&c.ID, &c.VideoID, &c.SegmentID, &c.Status, &c.Error, &c.CreatedAt, &c.UpdatedAt, &c.CompletedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *Repository) CreateForVideo(ctx context.Context, videoID uuid.UUID, segmentIDs []uuid.UUID) ([]Checkpoint, error) {
	if len(segmentIDs) == 0 {
		return []Checkpoint{}, nil
	}
	for _, segID := range segmentIDs {
		_, err := r.db.Exec(ctx, `INSERT INTO video_processing_checkpoints (video_id, segment_id, status) VALUES ($1,$2,'PENDING') ON CONFLICT (video_id, segment_id) DO NOTHING`, videoID, segID)
		if err != nil {
			return nil, err
		}
	}
	return r.GetByVideoID(ctx, videoID)
}

func (r *Repository) EnsureForSegments(ctx context.Context, videoID uuid.UUID, segmentIDs []uuid.UUID) error {
	if len(segmentIDs) == 0 {
		return nil
	}
	for _, segID := range segmentIDs {
		_, err := r.db.Exec(ctx, `INSERT INTO video_processing_checkpoints (video_id, segment_id, status) VALUES ($1,$2,'PENDING') ON CONFLICT (video_id, segment_id) DO NOTHING`, videoID, segID)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]Checkpoint, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_processing_checkpoints c JOIN video_segments s ON s.id = c.segment_id WHERE c.video_id = $1 ORDER BY s.segment_index ASC`, fieldsPrefixed)
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Checkpoint
	for rows.Next() {
		c, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	if list == nil {
		list = []Checkpoint{}
	}
	return list, rows.Err()
}

func (r *Repository) GetBySegmentID(ctx context.Context, segmentID uuid.UUID) (*Checkpoint, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_processing_checkpoints WHERE segment_id = $1 LIMIT 1`, fields)
	row := r.db.QueryRow(ctx, query, segmentID)
	return scan(row)
}

func (r *Repository) GetByVideoAndSegment(ctx context.Context, videoID, segmentID uuid.UUID) (*Checkpoint, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_processing_checkpoints WHERE video_id = $1 AND segment_id = $2`, fields)
	row := r.db.QueryRow(ctx, query, videoID, segmentID)
	return scan(row)
}

func (r *Repository) MarkProcessing(ctx context.Context, videoID, segmentID uuid.UUID) (*Checkpoint, error) {
	query := fmt.Sprintf(`UPDATE video_processing_checkpoints SET status = 'PROCESSING', error = NULL, updated_at = now(), completed_at = NULL WHERE video_id = $1 AND segment_id = $2 RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, videoID, segmentID)
	return scan(row)
}

func (r *Repository) MarkComplete(ctx context.Context, videoID, segmentID uuid.UUID) (*Checkpoint, error) {
	query := fmt.Sprintf(`UPDATE video_processing_checkpoints SET status = 'COMPLETE', error = NULL, updated_at = now(), completed_at = now() WHERE video_id = $1 AND segment_id = $2 RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, videoID, segmentID)
	return scan(row)
}

func (r *Repository) MarkFailed(ctx context.Context, videoID, segmentID uuid.UUID, errMsg string) (*Checkpoint, error) {
	query := fmt.Sprintf(`UPDATE video_processing_checkpoints SET status = 'FAILED', error = $3, updated_at = now(), completed_at = NULL WHERE video_id = $1 AND segment_id = $2 RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, videoID, segmentID, errMsg)
	return scan(row)
}

func (r *Repository) ResetFailed(ctx context.Context, videoID, segmentID uuid.UUID) (*Checkpoint, error) {
	query := fmt.Sprintf(`UPDATE video_processing_checkpoints SET status = 'PENDING', error = NULL, updated_at = now(), completed_at = NULL WHERE video_id = $1 AND segment_id = $2 AND status = 'FAILED' RETURNING %s`, fields)
	row := r.db.QueryRow(ctx, query, videoID, segmentID)
	return scan(row)
}

func (r *Repository) DeleteForVideo(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_processing_checkpoints WHERE video_id = $1`, videoID)
	return err
}

func (r *Repository) DeleteForSegment(ctx context.Context, segmentID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_processing_checkpoints WHERE segment_id = $1`, segmentID)
	return err
}
