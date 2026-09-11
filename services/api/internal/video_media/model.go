package video_media

import (
	"time"

	"github.com/google/uuid"
)

type MediaMetadata struct {
	ID               uuid.UUID  `json:"id"`
	VideoID          uuid.UUID  `json:"video_id"`
	FormatName       *string    `json:"format_name"`
	FormatLongName   *string    `json:"format_long_name"`
	DurationSeconds  *float64   `json:"duration_seconds"`
	SizeBytes        *int64     `json:"size_bytes"`
	BitRate          *int64     `json:"bit_rate"`
	StartTime        *float64   `json:"start_time"`
	VideoCodec       *string    `json:"video_codec"`
	VideoWidth       *int       `json:"video_width"`
	VideoHeight      *int       `json:"video_height"`
	VideoFps         *float64   `json:"video_fps"`
	VideoFpsRaw      *string    `json:"video_fps_raw"`
	AudioCodec       *string    `json:"audio_codec"`
	AudioSampleRate  *int       `json:"audio_sample_rate"`
	AudioChannels    *int       `json:"audio_channels"`
	AudioChannelLayout *string  `json:"audio_channel_layout"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}
