package detector

import (
	"context"

	"github.com/google/uuid"
)

type FrameInput struct {
	FrameID    uuid.UUID
	VideoID    uuid.UUID
	SegmentID  uuid.UUID
	Timestamp  float64
	Width      int
	Height     int
	StorageKey string
	LocalPath  string
}

type DetectionResult struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
	BBoxX      float64 `json:"bbox_x"`
	BBoxY      float64 `json:"bbox_y"`
	BBoxWidth  float64 `json:"bbox_width"`
	BBoxHeight float64 `json:"bbox_height"`
}

type VisualAnalyzer interface {
	AnalyzeBatch(ctx context.Context, frames []FrameInput) (map[uuid.UUID][]DetectionResult, error)
}
