package vision

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrRateLimited = errors.New("gemini rate limited")

func IsRateLimited(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	// Fallback string match for wrapped errors without sentinel (e.g. "rate limited" / "429")
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limited") || strings.Contains(msg, "429")
}

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

type HistorySegment struct {
	SegmentID    uuid.UUID   `json:"segment_id"`
	SegmentIndex int         `json:"segment_index"`
	StartTime    float64     `json:"start_time"`
	EndTime      float64     `json:"end_time"`
	Labels       []string    `json:"labels"`
	Tracks       []TrackInput `json:"tracks"`
	Events       []EventInput `json:"events"`
}

type SegmentHistory struct {
	PriorSegments    []HistorySegment `json:"prior_segments"`
	PersistentTracks []TrackInput     `json:"persistent_tracks"`
	PriorEvents      []EventInput     `json:"prior_events"`
}

type SegmentInput struct {
	SegmentID  uuid.UUID
	StartTime  float64
	EndTime    float64
	Frames     []FrameInput
	Detections []DetectionInput
	Tracks     []TrackInput
	Events     []EventInput
	History    *SegmentHistory `json:"history,omitempty"`
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
