package detection

import (
	"time"

	"github.com/google/uuid"
)

type Detection struct {
	ID              uuid.UUID `json:"id"`
	VideoID         uuid.UUID `json:"video_id"`
	SegmentID       uuid.UUID `json:"segment_id"`
	FrameID         uuid.UUID `json:"frame_id"`
	Label           string    `json:"label"`
	Confidence      float64   `json:"confidence"`
	BBoxX           float64   `json:"bbox_x"`
	BBoxY           float64   `json:"bbox_y"`
	BBoxWidth       float64   `json:"bbox_width"`
	BBoxHeight      float64   `json:"bbox_height"`
	DetectorName    string    `json:"detector_name"`
	DetectorVersion string    `json:"detector_version"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func Validate(d Detection) error {
	if d.Label == "" {
		return ErrInvalidLabel
	}
	if d.Confidence < 0 || d.Confidence > 1 {
		return ErrInvalidConfidence
	}
	if d.BBoxX < 0 || d.BBoxX > 1 || d.BBoxY < 0 || d.BBoxY > 1 || d.BBoxWidth <= 0 || d.BBoxWidth > 1 || d.BBoxHeight <= 0 || d.BBoxHeight > 1 {
		return ErrInvalidBBox
	}
	if d.BBoxX+d.BBoxWidth > 1.000001 || d.BBoxY+d.BBoxHeight > 1.000001 {
		return ErrInvalidBBox
	}
	return nil
}

var (
	ErrInvalidLabel      = errString("invalid label")
	ErrInvalidConfidence = errString("invalid confidence")
	ErrInvalidBBox       = errString("invalid bbox")
)

type errString string

func (e errString) Error() string { return string(e) }

func ClampBBox(x, y, w, h float64) (float64, float64, float64, float64) {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if w < 0 {
		w = 0
	}
	if h < 0 {
		h = 0
	}
	if x > 1 {
		x = 1
	}
	if y > 1 {
		y = 1
	}
	if w > 1 {
		w = 1
	}
	if h > 1 {
		h = 1
	}
	if x+w > 1 {
		w = 1 - x
	}
	if y+h > 1 {
		h = 1 - y
	}
	if w <= 0 {
		w = 0.01
	}
	if h <= 0 {
		h = 0.01
	}
	return x, y, w, h
}
