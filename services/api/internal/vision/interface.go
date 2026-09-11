package vision

import (
	"context"

	"github.com/google/uuid"
)

type SegmentDescriptionInput struct {
	VideoID      uuid.UUID
	SegmentID    uuid.UUID
	StartTime    float64
	EndTime      float64
	Frames       []FrameRef
	Detections   []DetectionRef
	Tracks       []TrackRef
	Events       []EventRef
}

type FrameRef struct {
	ID        uuid.UUID
	Timestamp float64
	Width     int
	Height    int
}

type DetectionRef struct {
	Label      string
	Confidence float64
	Count      int
}

type TrackRef struct {
	Label          string
	TrackIndex     int
	StartTimestamp float64
	EndTimestamp   float64
	DetectionCount int
}

type EventRef struct {
	EventType string
	Label     string
	Start     float64
	End       *float64
}

type DescriptionResult struct {
	Description  string
	ModelName    string
	ModelVersion string
}

type VisionDescriber interface {
	DescribeSegment(ctx context.Context, input SegmentDescriptionInput) (DescriptionResult, error)
}
