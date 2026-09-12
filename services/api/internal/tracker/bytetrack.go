package tracker

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

// ByteTrack implements the ByteTrack two-stage association in pure Go.
// See spec: high/low split, Hungarian assignment with IoU threshold, label-aware matching,
// frame-based lifecycle with TRACKER_TRACK_BUFFER.
type ByteTrack struct {
	HighThreshold  float64
	LowThreshold   float64
	MatchThreshold float64
	TrackBuffer    int
}

func NewByteTrack(high, low, match float64, buffer int) *ByteTrack {
	return &ByteTrack{
		HighThreshold:  high,
		LowThreshold:   low,
		MatchThreshold: match,
		TrackBuffer:    buffer,
	}
}

func (t *ByteTrack) Name() string    { return "bytetrack" }
func (t *ByteTrack) Version() string { return "1" }

type btTrack struct {
	Index         int
	Label         string
	BBox          DetectionInput
	LastTimestamp float64
	Misses        int
	Age           int
}

func (t *ByteTrack) Track(ctx context.Context, frames []FrameInput, detectionsByFrame map[uuid.UUID][]DetectionInput) (map[uuid.UUID]int, error) {
	sort.Slice(frames, func(i, j int) bool { return frames[i].Timestamp < frames[j].Timestamp })

	assignments := make(map[uuid.UUID]int)
	active := make(map[int]*btTrack)
	nextIndex := 0

	for _, fr := range frames {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		dets := detectionsByFrame[fr.FrameID]

		var highDets []DetectionInput
		var lowDets []DetectionInput
		for _, d := range dets {
			if d.Confidence >= t.HighThreshold {
				highDets = append(highDets, d)
			} else if d.Confidence >= t.LowThreshold {
				lowDets = append(lowDets, d)
			} else {
				// discard
			}
		}
		sort.Slice(highDets, func(i, j int) bool { return highDets[i].Confidence > highDets[j].Confidence })
		sort.Slice(lowDets, func(i, j int) bool { return lowDets[i].Confidence > lowDets[j].Confidence })

		// Build sorted active list for deterministic ordering
		var trackList []*btTrack
		for _, at := range active {
			trackList = append(trackList, at)
		}
		sort.Slice(trackList, func(i, j int) bool { return trackList[i].Index < trackList[j].Index })

		// First stage: active tracks vs high detections
		matchedTrack := make(map[int]bool)
		matchedHighIdx := make(map[int]bool) // index in highDets slice

		if len(trackList) > 0 && len(highDets) > 0 {
			cost, N := buildCostMatrix(trackList, highDets, t.MatchThreshold)
			assign := hungarian(cost, N)
			for i, tr := range trackList {
				j := assign[i]
				if j >= 0 && j < len(highDets) {
					if cost[i][j] < 1e8 { // valid match (not INF)
						d := highDets[j]
						// double check label consistency already enforced via INF
						assignments[d.ID] = tr.Index
						tr.BBox = d
						tr.LastTimestamp = fr.Timestamp
						tr.Misses = 0
						tr.Age++
						matchedTrack[tr.Index] = true
						matchedHighIdx[j] = true
					}
				}
			}
		}

		// Collect remaining tracks after first stage
		var remainingTracks []*btTrack
		for _, tr := range trackList {
			if !matchedTrack[tr.Index] {
				remainingTracks = append(remainingTracks, tr)
			}
		}

		// Second stage: remaining tracks vs low detections
		matchedLowIdx := make(map[int]bool)
		if len(remainingTracks) > 0 && len(lowDets) > 0 {
			cost, N := buildCostMatrix(remainingTracks, lowDets, t.MatchThreshold)
			assign := hungarian(cost, N)
			for i, tr := range remainingTracks {
				j := assign[i]
				if j >= 0 && j < len(lowDets) {
					if cost[i][j] < 1e8 {
						d := lowDets[j]
						assignments[d.ID] = tr.Index
						tr.BBox = d
						tr.LastTimestamp = fr.Timestamp
						tr.Misses = 0
						tr.Age++
						matchedTrack[tr.Index] = true
						matchedLowIdx[j] = true
					}
				}
			}
		}

		// Unmatched tracks management: increment misses, terminate if exceeded
		for _, tr := range trackList {
			if !matchedTrack[tr.Index] {
				tr.Misses++
				if tr.Misses > t.TrackBuffer {
					delete(active, tr.Index)
				}
			}
		}

		// New tracks from unmatched high-confidence detections
		for idx, d := range highDets {
			if matchedHighIdx[idx] {
				continue
			}
			newIdx := nextIndex
			nextIndex++
			assignments[d.ID] = newIdx
			active[newIdx] = &btTrack{
				Index:         newIdx,
				Label:         d.Label,
				BBox:          d,
				LastTimestamp: fr.Timestamp,
				Misses:        0,
				Age:           0,
			}
		}
		// low unmatched are discarded
		_ = matchedLowIdx
	}

	return assignments, nil
}

const INF = 1e9
const DUMMY_COST = 1.0

func buildCostMatrix(tracks []*btTrack, dets []DetectionInput, matchThreshold float64) ([][]float64, int) {
	nRows := len(tracks)
	nCols := len(dets)
	N := nRows
	if nCols > N {
		N = nCols
	}
	cost := make([][]float64, N)
	for i := 0; i < N; i++ {
		cost[i] = make([]float64, N)
		for j := 0; j < N; j++ {
			cost[i][j] = DUMMY_COST
		}
	}
	for i, tr := range tracks {
		for j, d := range dets {
			if tr.Label != d.Label {
				cost[i][j] = INF
				continue
			}
			score := iou(tr.BBox, d)
			if score >= matchThreshold {
				cost[i][j] = 1 - score
			} else {
				cost[i][j] = INF
			}
		}
	}
	return cost, N
}

// hungarian solves the assignment problem for a square cost matrix N x N (minimization).
// Returns assignment array where assign[row] = col.
func hungarian(cost [][]float64, n int) []int {
	if n == 0 {
		return []int{}
	}
	u := make([]float64, n+1)
	v := make([]float64, n+1)
	p := make([]int, n+1)
	way := make([]int, n+1)

	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, n+1)
		for j := 0; j <= n; j++ {
			minv[j] = 1e18
		}
		used := make([]bool, n+1)
		for {
			used[j0] = true
			i0 := p[j0]
			delta := 1e18
			j1 := 0
			for j := 1; j <= n; j++ {
				if !used[j] {
					cur := cost[i0-1][j-1] - u[i0] - v[j]
					if cur < minv[j] {
						minv[j] = cur
						way[j] = j0
					}
					if minv[j] < delta {
						delta = minv[j]
						j1 = j
					}
				}
			}
			for j := 0; j <= n; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
			if j0 == 0 {
				break
			}
		}
	}

	ans := make([]int, n)
	for j := 1; j <= n; j++ {
		if p[j] != 0 {
			ans[p[j]-1] = j - 1
		}
	}
	return ans
}
