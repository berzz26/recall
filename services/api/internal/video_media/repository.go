package video_media

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

const fields = `id, video_id, format_name, format_long_name, duration_seconds, size_bytes, bit_rate, start_time, video_codec, video_width, video_height, video_fps, video_fps_raw, audio_codec, audio_sample_rate, audio_channels, audio_channel_layout, created_at, updated_at`

func scan(row interface{ Scan(dest ...any) error }) (*MediaMetadata, error) {
	var m MediaMetadata
	err := row.Scan(
		&m.ID, &m.VideoID, &m.FormatName, &m.FormatLongName, &m.DurationSeconds, &m.SizeBytes, &m.BitRate, &m.StartTime,
		&m.VideoCodec, &m.VideoWidth, &m.VideoHeight, &m.VideoFps, &m.VideoFpsRaw,
		&m.AudioCodec, &m.AudioSampleRate, &m.AudioChannels, &m.AudioChannelLayout,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *Repository) Upsert(ctx context.Context, m *MediaMetadata) (*MediaMetadata, error) {
	query := fmt.Sprintf(`
		INSERT INTO video_media_metadata (video_id, format_name, format_long_name, duration_seconds, size_bytes, bit_rate, start_time, video_codec, video_width, video_height, video_fps, video_fps_raw, audio_codec, audio_sample_rate, audio_channels, audio_channel_layout)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		ON CONFLICT (video_id) DO UPDATE SET
			format_name = EXCLUDED.format_name,
			format_long_name = EXCLUDED.format_long_name,
			duration_seconds = EXCLUDED.duration_seconds,
			size_bytes = EXCLUDED.size_bytes,
			bit_rate = EXCLUDED.bit_rate,
			start_time = EXCLUDED.start_time,
			video_codec = EXCLUDED.video_codec,
			video_width = EXCLUDED.video_width,
			video_height = EXCLUDED.video_height,
			video_fps = EXCLUDED.video_fps,
			video_fps_raw = EXCLUDED.video_fps_raw,
			audio_codec = EXCLUDED.audio_codec,
			audio_sample_rate = EXCLUDED.audio_sample_rate,
			audio_channels = EXCLUDED.audio_channels,
			audio_channel_layout = EXCLUDED.audio_channel_layout,
			updated_at = now()
		RETURNING %s
	`, fields)
	row := r.db.QueryRow(ctx, query,
		m.VideoID, m.FormatName, m.FormatLongName, m.DurationSeconds, m.SizeBytes, m.BitRate, m.StartTime,
		m.VideoCodec, m.VideoWidth, m.VideoHeight, m.VideoFps, m.VideoFpsRaw,
		m.AudioCodec, m.AudioSampleRate, m.AudioChannels, m.AudioChannelLayout,
	)
	return scan(row)
}

func (r *Repository) GetByVideoID(ctx context.Context, videoID uuid.UUID) (*MediaMetadata, error) {
	query := fmt.Sprintf(`SELECT %s FROM video_media_metadata WHERE video_id = $1`, fields)
	row := r.db.QueryRow(ctx, query, videoID)
	return scan(row)
}

func (r *Repository) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM video_media_metadata WHERE video_id = $1`, videoID)
	return err
}
