package video_frame

import (
	"time"

	"github.com/google/uuid"
)

type VideoFrame struct {
	ID               uuid.UUID `json:"id"`
	VideoID          uuid.UUID `json:"video_id"`
	SegmentID        uuid.UUID `json:"segment_id"`
	FrameIndex       int       `json:"frame_index"`
	TimestampSeconds float64   `json:"timestamp_seconds"`
	StorageKey       string    `json:"storage_key"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}
