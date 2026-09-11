package tracker

import (
	"context"

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
}

// Simple result: map detectionID -> trackIndex
