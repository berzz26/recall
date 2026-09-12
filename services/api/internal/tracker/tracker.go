package tracker

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type DetectionInput struct {
	ID        uuid.UUID
	Label     string
	Confidence float64
	BBoxX     float64
	BBoxY     float64
	BBoxWidth float64
	BBoxHeight float64
	FrameID   uuid.UUID
	Timestamp float64
}

type FrameInput struct {
	FrameID   uuid.UUID
	Timestamp float64
}

type TrackAssignment struct {
	DetectionID uuid.UUID
	TrackIndex  int
	TrackID     uuid.UUID // to be filled after persistence
}

type Tracker interface {
	Track(ctx context.Context, frames []FrameInput, detectionsByFrame map[uuid.UUID][]DetectionInput) (map[uuid.UUID]int, error)
	Name() string
	Version() string
}

// Simple result: map detectionID -> trackIndex

// New creates a tracker based on TRACKER_TYPE.
// trackerType must be "iou" or "bytetrack"; any other value returns an error.
// For "iou", the ByteTrack thresholds/buffer are ignored and defaults (0.2 / 5s) are used.
// For "bytetrack", the provided thresholds and buffer are used.
func New(trackerType string, highThreshold, lowThreshold, matchThreshold float64, trackBuffer int) (Tracker, error) {
	switch trackerType {
	case "iou":
		return NewIoUTracker(), nil
	case "bytetrack":
		return NewByteTrack(highThreshold, lowThreshold, matchThreshold, trackBuffer), nil
	default:
		return nil, fmt.Errorf("invalid TRACKER_TYPE %q: must be one of [iou, bytetrack]", trackerType)
	}
}
