package segment_description

import (
	"time"

	"github.com/google/uuid"
)

type Description struct {
	ID           uuid.UUID `json:"id"`
	VideoID      uuid.UUID `json:"video_id"`
	SegmentID    uuid.UUID `json:"segment_id"`
	Description  string    `json:"description"`
	ModelName    string    `json:"model_name"`
	ModelVersion string    `json:"model_version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
