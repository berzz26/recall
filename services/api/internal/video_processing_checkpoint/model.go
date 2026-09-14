package video_processing_checkpoint

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusComplete   Status = "COMPLETE"
	StatusFailed     Status = "FAILED"
)

type Checkpoint struct {
	ID          uuid.UUID  `json:"id"`
	VideoID     uuid.UUID  `json:"video_id"`
	SegmentID   uuid.UUID  `json:"segment_id"`
	Status      Status     `json:"status"`
	Error       *string    `json:"error"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at"`
}
