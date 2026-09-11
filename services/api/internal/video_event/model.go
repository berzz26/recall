package video_event

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	EventAppeared   = "OBJECT_APPEARED"
	EventPresent    = "OBJECT_PRESENT"
	EventDisappeared = "OBJECT_DISAPPEARED"
	EventMoved      = "OBJECT_MOVED"
)

type Event struct {
	ID             uuid.UUID       `json:"id"`
	VideoID        uuid.UUID       `json:"video_id"`
	TrackID        *uuid.UUID      `json:"track_id"`
	SegmentID      *uuid.UUID      `json:"segment_id"`
	EventType      string          `json:"event_type"`
	Label          string          `json:"label"`
	StartTimestamp float64         `json:"start_timestamp"`
	EndTimestamp   *float64        `json:"end_timestamp"`
	Confidence     *float64        `json:"confidence"`
	Metadata       json.RawMessage `json:"metadata"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
