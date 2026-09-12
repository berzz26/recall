package search

import (
	"github.com/google/uuid"
)

type SearchRequest struct {
	Query   string     `json:"query"`
	Limit   int        `json:"limit,omitempty"`
	VideoID *uuid.UUID `json:"video_id,omitempty"`
}

type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	VideoID     uuid.UUID       `json:"video_id"`
	VideoName   string          `json:"video_filename,omitempty"`
	Filename    string          `json:"filename,omitempty"`
	SegmentID   uuid.UUID       `json:"segment_id"`
	StartTime   float64         `json:"start_time"`
	EndTime     float64         `json:"end_time"`
	Description string          `json:"description"`
	Similarity  float64         `json:"similarity"`
	Detections  []DetectionInfo `json:"detections"`
	Tracks      []TrackInfo     `json:"tracks"`
	Events      []EventInfo     `json:"events"`
}

type DetectionInfo struct {
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence"`
}

type TrackInfo struct {
	TrackID   uuid.UUID `json:"track_id"`
	Label     string    `json:"label"`
	StartTime float64   `json:"start_time"`
	EndTime   float64   `json:"end_time"`
}

type EventInfo struct {
	EventID    uuid.UUID `json:"event_id"`
	EventType  string    `json:"event_type"`
	Label      string    `json:"label"`
	StartTime  float64   `json:"start_time"`
	EndTime    *float64  `json:"end_time,omitempty"`
	Confidence *float64  `json:"confidence,omitempty"`
}
