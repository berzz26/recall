package video_segment

import (
	"time"

	"github.com/google/uuid"
)

type VideoSegment struct {
	ID           uuid.UUID `json:"id"`
	VideoID      uuid.UUID `json:"video_id"`
	SegmentIndex int       `json:"segment_index"`
	StartTime    float64   `json:"start_time"`
	EndTime      float64   `json:"end_time"`
	Duration     float64   `json:"duration"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
