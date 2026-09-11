package video_event

import (
	"context"
	"encoding/json"
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

const fields = `id, video_id, track_id, segment_id, event_type, label, start_timestamp, end_timestamp, confidence, metadata, created_at, updated_at`

func scan(row interface{ Scan(dest ...any) error }) (*Event, error) {
	var e Event
	err := row.Scan(&e.ID, &e.VideoID, &e.TrackID, &e.SegmentID, &e.EventType, &e.Label, &e.StartTimestamp, &e.EndTimestamp, &e.Confidence, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if e.Metadata == nil {
		e.Metadata = json.RawMessage(`{}`)
	}
	return &e, nil
}

func (r *Repository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_events WHERE video_id = $1`, videoID)
	return err
}

func (r *Repository) CreateBatch(ctx context.Context, events []Event) ([]Event, error) {
	if len(events) == 0 {
		return []Event{}, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var out []Event
	for _, e := range events {
		if e.Metadata == nil {
			e.Metadata = json.RawMessage(`{}`)
		}
		query := fmt.Sprintf(`INSERT INTO video_events (video_id, track_id, segment_id, event_type, label, start_timestamp, end_timestamp, confidence, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING %s`, fields)
		row := tx.QueryRow(ctx, query, e.VideoID, e.TrackID, e.SegmentID, e.EventType, e.Label, e.StartTimestamp, e.EndTimestamp, e.Confidence, e.Metadata)
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

func (r *Repository) ReplaceForVideo(ctx context.Context, videoID uuid.UUID, events []Event) ([]Event, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM video_events WHERE video_id = $1`, videoID); err != nil {
		return nil, err
	}
	var out []Event
	for _, e := range events {
		if e.Metadata == nil {
			e.Metadata = json.RawMessage(`{}`)
		}
		query := fmt.Sprintf(`INSERT INTO video_events (video_id, track_id, segment_id, event_type, label, start_timestamp, end_timestamp, confidence, metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING %s`, fields)
		row := tx.QueryRow(ctx, query, e.VideoID, e.TrackID, e.SegmentID, e.EventType, e.Label, e.StartTimestamp, e.EndTimestamp, e.Confidence, e.Metadata)
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

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID, eventType, label string, trackID *uuid.UUID) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_events WHERE video_id = $1`, fields)
	args := []interface{}{videoID}
	idx := 2
	if eventType != "" {
		query += fmt.Sprintf(` AND event_type = $%d`, idx)
		args = append(args, eventType)
		idx++
	}
	if label != "" {
		query += fmt.Sprintf(` AND label = $%d`, idx)
		args = append(args, label)
		idx++
	}
	if trackID != nil {
		query += fmt.Sprintf(` AND track_id = $%d`, idx)
		args = append(args, *trackID)
		idx++
	}
	query += ` ORDER BY start_timestamp ASC, event_type ASC, id ASC`
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Event
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	if list == nil {
		list = []Event{}
	}
	return list, rows.Err()
}

func (r *Repository) GetByTrackID(ctx context.Context, trackID uuid.UUID) ([]Event, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_events WHERE track_id = $1 ORDER BY start_timestamp ASC`, fields)
	rows, err := r.db.Query(ctx, query, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Event
	for rows.Next() {
		e, err := scan(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *e)
	}
	if list == nil {
		list = []Event{}
	}
	return list, rows.Err()
}
