package segment_embedding

import (
	"time"

	"github.com/google/uuid"
)

type Embedding struct {
	ID            uuid.UUID `json:"id"`
	VideoID       uuid.UUID `json:"video_id"`
	SegmentID     uuid.UUID `json:"segment_id"`
	DescriptionID uuid.UUID `json:"description_id"`
	Embedding     []float32 `json:"embedding"`
	ModelName     string    `json:"model_name"`
	ModelVersion  string    `json:"model_version"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type SearchResult struct {
	Embedding   Embedding `json:"embedding"`
	Description string    `json:"description"`
	StartTime   float64   `json:"start_time"`
	EndTime     float64   `json:"end_time"`
	Similarity  float64   `json:"similarity"`
}
