package vision

import (
	"context"

	"github.com/google/uuid"
)

type FrameInput struct {
	ID         uuid.UUID
	Timestamp  float64
	StorageKey string
}

type DetectionInput struct {
	Label     string
	FrameID   uuid.UUID
	Timestamp float64
}

type TrackInput struct {
	Label      string
	TrackIndex int
	Start      float64
	End        float64
}

type EventInput struct {
	EventType string
	Label     string
	Start     float64
	End       *float64
}

type SegmentInput struct {
	SegmentID  uuid.UUID
	StartTime  float64
	EndTime    float64
	Frames     []FrameInput
	Detections []DetectionInput
	Tracks     []TrackInput
	Events     []EventInput
}

type VideoDescriptionInput struct {
	VideoID  uuid.UUID
	Segments []SegmentInput
}

type DescriptionResult struct {
	SegmentID    uuid.UUID
	Description  string
	ModelName    string
	ModelVersion string
}

type VisionDescriber interface {
	DescribeVideo(ctx context.Context, input VideoDescriptionInput) ([]DescriptionResult, error)
}
