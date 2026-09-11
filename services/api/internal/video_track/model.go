package video_track

import (
	"time"

	"github.com/google/uuid"
)

type Track struct {
	ID             uuid.UUID  `json:"id"`
	VideoID        uuid.UUID  `json:"video_id"`
	SegmentID      *uuid.UUID `json:"segment_id"`
	Label          string     `json:"label"`
	TrackIndex     int        `json:"track_index"`
	StartTimestamp float64    `json:"start_timestamp"`
	EndTimestamp   float64    `json:"end_timestamp"`
	TrackerName    string     `json:"tracker_name"`
	TrackerVersion string     `json:"tracker_version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type TrackDetection struct {
	ID               uuid.UUID `json:"id"`
	TrackID          uuid.UUID `json:"track_id"`
	DetectionID      uuid.UUID `json:"detection_id"`
	FrameID          uuid.UUID `json:"frame_id"`
	TimestampSeconds float64   `json:"timestamp_seconds"`
	CreatedAt        time.Time `json:"created_at"`
}

type TrackWithCount struct {
	Track
	DetectionCount int `json:"detection_count"`
}
