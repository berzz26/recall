package video_track

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

const trackFields = `id, video_id, segment_id, label, track_index, start_timestamp, end_timestamp, tracker_name, tracker_version, created_at, updated_at`
const linkFields = `id, track_id, detection_id, frame_id, timestamp_seconds, created_at`

func scanTrack(row interface{ Scan(dest ...any) error }) (*Track, error) {
	var t Track
	err := row.Scan(&t.ID, &t.VideoID, &t.SegmentID, &t.Label, &t.TrackIndex, &t.StartTimestamp, &t.EndTimestamp, &t.TrackerName, &t.TrackerVersion, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *Repository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_tracks WHERE video_id = $1`, videoID)
	return err
}

// ReplaceForVideo deletes existing tracks for video and inserts new ones atomically.
// linksByTrackIndex maps track_index -> list of detections for that track
func (r *Repository) ReplaceForVideo(ctx context.Context, videoID uuid.UUID, tracks []Track, linksByTrackIndex map[int][]TrackDetection) ([]Track, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `DELETE FROM video_tracks WHERE video_id = $1`, videoID); err != nil {
		return nil, err
	}
	saved := make([]Track, 0, len(tracks))
	indexToID := make(map[int]uuid.UUID)
	for _, t := range tracks {
		query := fmt.Sprintf(`INSERT INTO video_tracks (video_id, segment_id, label, track_index, start_timestamp, end_timestamp, tracker_name, tracker_version) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING %s`, trackFields)
		row := tx.QueryRow(ctx, query, t.VideoID, t.SegmentID, t.Label, t.TrackIndex, t.StartTimestamp, t.EndTimestamp, t.TrackerName, t.TrackerVersion)
		s, err := scanTrack(row)
		if err != nil {
			return nil, err
		}
		saved = append(saved, *s)
		indexToID[s.TrackIndex] = s.ID
	}
	for idx, list := range linksByTrackIndex {
		tid, ok := indexToID[idx]
		if !ok {
			continue
		}
		for _, l := range list {
			q := fmt.Sprintf(`INSERT INTO video_track_detections (track_id, detection_id, frame_id, timestamp_seconds) VALUES ($1,$2,$3,$4) RETURNING %s`, linkFields)
			_, err := tx.Exec(ctx, q, tid, l.DetectionID, l.FrameID, l.TimestampSeconds)
			if err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return saved, nil
}

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]Track, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_tracks WHERE video_id = $1 ORDER BY track_index ASC`, trackFields)
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *t)
	}
	if list == nil {
		list = []Track{}
	}
	return list, rows.Err()
}

func (r *Repository) GetDetectionsByTrackID(ctx context.Context, trackID uuid.UUID) ([]TrackDetection, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_track_detections WHERE track_id = $1 ORDER BY timestamp_seconds ASC`, linkFields)
	rows, err := r.db.Query(ctx, query, trackID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrackDetection
	for rows.Next() {
		var td TrackDetection
		err := rows.Scan(&td.ID, &td.TrackID, &td.DetectionID, &td.FrameID, &td.TimestampSeconds, &td.CreatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, td)
	}
	if list == nil {
		list = []TrackDetection{}
	}
	return list, rows.Err()
}

func (r *Repository) GetDetectionsByVideoID(ctx context.Context, videoID uuid.UUID) ([]TrackDetection, error) {
	query := `SELECT vtd.id, vtd.track_id, vtd.detection_id, vtd.frame_id, vtd.timestamp_seconds, vtd.created_at FROM video_track_detections vtd JOIN video_tracks vt ON vt.id = vtd.track_id WHERE vt.video_id = $1 ORDER BY vtd.timestamp_seconds ASC`
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrackDetection
	for rows.Next() {
		var td TrackDetection
		err := rows.Scan(&td.ID, &td.TrackID, &td.DetectionID, &td.FrameID, &td.TimestampSeconds, &td.CreatedAt)
		if err != nil {
			return nil, err
		}
		list = append(list, td)
	}
	if list == nil {
		list = []TrackDetection{}
	}
	return list, rows.Err()
}

func (r *Repository) GetTracksWithCounts(ctx context.Context, videoID uuid.UUID) ([]TrackWithCount, error) {
	query := `SELECT vt.id, vt.video_id, vt.segment_id, vt.label, vt.track_index, vt.start_timestamp, vt.end_timestamp, vt.tracker_name, vt.tracker_version, vt.created_at, vt.updated_at, COUNT(vtd.id) as cnt FROM video_tracks vt LEFT JOIN video_track_detections vtd ON vtd.track_id = vt.id WHERE vt.video_id = $1 GROUP BY vt.id ORDER BY vt.track_index ASC`
	rows, err := r.db.Query(ctx, query, videoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []TrackWithCount
	for rows.Next() {
		var t TrackWithCount
		err := rows.Scan(&t.ID, &t.VideoID, &t.SegmentID, &t.Label, &t.TrackIndex, &t.StartTimestamp, &t.EndTimestamp, &t.TrackerName, &t.TrackerVersion, &t.CreatedAt, &t.UpdatedAt, &t.DetectionCount)
		if err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	if list == nil {
		list = []TrackWithCount{}
	}
	return list, rows.Err()
}
