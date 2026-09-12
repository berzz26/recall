package tracker

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

type IoUTracker struct {
	IouThreshold float64
	MaxGapSeconds float64
}

func NewIoUTracker() *IoUTracker {
	return &IoUTracker{IouThreshold: 0.2, MaxGapSeconds: 5.0}
}

func (t *IoUTracker) Name() string    { return "iou" }
func (t *IoUTracker) Version() string { return "1" }

func iou(a, b DetectionInput) float64 {
	ax1, ay1 := a.BBoxX, a.BBoxY
	ax2, ay2 := a.BBoxX+a.BBoxWidth, a.BBoxY+a.BBoxHeight
	bx1, by1 := b.BBoxX, b.BBoxY
	bx2, by2 := b.BBoxX+b.BBoxWidth, b.BBoxY+b.BBoxHeight
	ix1 := max(ax1, bx1)
	iy1 := max(ay1, by1)
	ix2 := min(ax2, bx2)
	iy2 := min(ay2, by2)
	if ix2 <= ix1 || iy2 <= iy1 {
		return 0
	}
	inter := (ix2 - ix1) * (iy2 - iy1)
	areaA := a.BBoxWidth * a.BBoxHeight
	areaB := b.BBoxWidth * b.BBoxHeight
	union := areaA + areaB - inter
	if union <= 0 {
		return 0
	}
	return inter / union
}

func max(a, b float64) float64 { if a > b { return a }; return b }
func min(a, b float64) float64 { if a < b { return a }; return b }

type activeTrack struct {
	TrackIndex int
	Label      string
	LastBBox   DetectionInput
	LastTime   float64
}

func (t *IoUTracker) Track(ctx context.Context, frames []FrameInput, detectionsByFrame map[uuid.UUID][]DetectionInput) (map[uuid.UUID]int, error) {
	// sort frames by timestamp
	sort.Slice(frames, func(i, j int) bool { return frames[i].Timestamp < frames[j].Timestamp })
	assignments := make(map[uuid.UUID]int)
	active := make(map[int]*activeTrack)
	nextIndex := 0

	for _, fr := range frames {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		dets := detectionsByFrame[fr.FrameID]
		// sort by confidence descending for greedy matching
		sort.Slice(dets, func(i, j int) bool { return dets[i].Confidence > dets[j].Confidence })

		// expire old tracks
		for idx, at := range active {
			if fr.Timestamp-at.LastTime > t.MaxGapSeconds {
				delete(active, idx)
			}
		}

		usedTrack := make(map[int]bool)
		for _, d := range dets {
			bestIdx := -1
			bestIoU := 0.0
			for idx, at := range active {
				if usedTrack[idx] {
					continue
				}
				if at.Label != d.Label {
					continue
				}
				score := iou(at.LastBBox, d)
				if score > bestIoU && score >= t.IouThreshold {
					bestIoU = score
					bestIdx = idx
				}
			}
			if bestIdx != -1 {
				assignments[d.ID] = bestIdx
				usedTrack[bestIdx] = true
				at := active[bestIdx]
				at.LastBBox = d
				at.LastTime = fr.Timestamp
			} else {
				idx := nextIndex
				nextIndex++
				assignments[d.ID] = idx
				usedTrack[idx] = true
				active[idx] = &activeTrack{TrackIndex: idx, Label: d.Label, LastBBox: d, LastTime: fr.Timestamp}
			}
		}
	}
	return assignments, nil
}
