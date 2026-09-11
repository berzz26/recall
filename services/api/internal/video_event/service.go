package video_event

import (
	"context"
	"encoding/json"
	"math"

	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/video_track"
)

type Service struct {
	repo              *Repository
	trackRepo         *video_track.Repository
	segmentRepo       *video_segment.Repository
	detectionRepo     *detection.Repository
	movementThreshold float64
}

func NewService(repo *Repository, trackRepo *video_track.Repository, segmentRepo *video_segment.Repository, detectionRepo *detection.Repository) *Service {
	return &Service{repo: repo, trackRepo: trackRepo, segmentRepo: segmentRepo, detectionRepo: detectionRepo, movementThreshold: 0.05}
}

func NewServiceWithThreshold(repo *Repository, trackRepo *video_track.Repository, segmentRepo *video_segment.Repository, detectionRepo *detection.Repository, thresh float64) *Service {
	if thresh <= 0 || thresh > 1 {
		thresh = 0.05
	}
	return &Service{repo: repo, trackRepo: trackRepo, segmentRepo: segmentRepo, detectionRepo: detectionRepo, movementThreshold: thresh}
}

func findSegmentForTimestamp(segs []video_segment.VideoSegment, ts float64) *uuid.UUID {
	for _, s := range segs {
		if ts >= s.StartTime && ts < s.EndTime {
			id := s.ID
			return &id
		}
	}
	if len(segs) > 0 {
		last := segs[len(segs)-1]
		if ts >= last.StartTime && ts <= last.EndTime {
			id := last.ID
			return &id
		}
	}
	return nil
}

func center(bx, by, bw, bh float64) (float64, float64) {
	return bx + bw/2, by + bh/2
}

func distance(x1, y1, x2, y2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	return math.Sqrt(dx*dx + dy*dy)
}

func (s *Service) GenerateForVideo(ctx context.Context, videoID uuid.UUID) ([]Event, error) {
	tracks, err := s.trackRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if len(tracks) == 0 {
		// delete existing events and return empty - no tracks = no events
		if _, err := s.repo.ReplaceForVideo(ctx, videoID, []Event{}); err != nil {
			return nil, err
		}
		return []Event{}, nil
	}
	segs, err := s.segmentRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	allDets, err := s.detectionRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]detection.Detection)
	for _, d := range allDets {
		byID[d.ID] = d
	}
	var allEvents []Event
	for _, tr := range tracks {
		trackDets, err := s.trackRepo.GetDetectionsByTrackID(ctx, tr.ID)
		if err != nil {
			return nil, err
		}
		if len(trackDets) == 0 {
			continue
		}
		type detWithBBox struct {
			Timestamp float64
			BBoxX, BBoxY, BBoxW, BBoxH float64
		}
		var ordered []detWithBBox
		for _, td := range trackDets {
			if d, ok := byID[td.DetectionID]; ok {
				ordered = append(ordered, detWithBBox{Timestamp: td.TimestampSeconds, BBoxX: d.BBoxX, BBoxY: d.BBoxY, BBoxW: d.BBoxWidth, BBoxH: d.BBoxHeight})
			} else {
				ordered = append(ordered, detWithBBox{Timestamp: td.TimestampSeconds})
			}
		}
		// sort by timestamp
		// trackDets already ordered by timestamp, but ordered slice follows same
		// appearance
		appearedSID := findSegmentForTimestamp(segs, tr.StartTimestamp)
		allEvents = append(allEvents, Event{
			VideoID: videoID, TrackID: &tr.ID, SegmentID: appearedSID,
			EventType: EventAppeared, Label: tr.Label, StartTimestamp: tr.StartTimestamp, Metadata: json.RawMessage(`{}`),
		})
		// presence
		presentSID := findSegmentForTimestamp(segs, tr.StartTimestamp)
		// if spans multiple segments, leave segment_id null
		endSeg := findSegmentForTimestamp(segs, tr.EndTimestamp)
		if presentSID != nil && endSeg != nil && *presentSID != *endSeg {
			presentSID = nil
		}
		end := tr.EndTimestamp
		allEvents = append(allEvents, Event{
			VideoID: videoID, TrackID: &tr.ID, SegmentID: presentSID,
			EventType: EventPresent, Label: tr.Label, StartTimestamp: tr.StartTimestamp, EndTimestamp: &end, Metadata: json.RawMessage(`{}`),
		})
		// disappeared
		disSID := findSegmentForTimestamp(segs, tr.EndTimestamp)
		allEvents = append(allEvents, Event{
			VideoID: videoID, TrackID: &tr.ID, SegmentID: disSID,
			EventType: EventDisappeared, Label: tr.Label, StartTimestamp: tr.EndTimestamp, Metadata: json.RawMessage(`{}`),
		})

		// movement: find intervals where center displacement > threshold
		if len(ordered) >= 2 {
			var movingStart *float64
			var movingEnd float64
			flush := func() {
				if movingStart != nil {
					ms := *movingStart
					me := movingEnd
					sid := findSegmentForTimestamp(segs, ms)
					emSeg := findSegmentForTimestamp(segs, me)
					if sid != nil && emSeg != nil && *sid != *emSeg {
						sid = nil
					}
					meta, _ := json.Marshal(map[string]any{"movement_threshold": s.movementThreshold})
					allEvents = append(allEvents, Event{
						VideoID: videoID, TrackID: &tr.ID, SegmentID: sid,
						EventType: EventMoved, Label: tr.Label, StartTimestamp: ms, EndTimestamp: &me, Metadata: meta,
					})
					movingStart = nil
				}
			}
			for i := 1; i < len(ordered); i++ {
				prev := ordered[i-1]
				cur := ordered[i]
				cx1, cy1 := center(prev.BBoxX, prev.BBoxY, prev.BBoxW, prev.BBoxH)
				cx2, cy2 := center(cur.BBoxX, cur.BBoxY, cur.BBoxW, cur.BBoxH)
				dist := distance(cx1, cy1, cx2, cy2)
				if dist > s.movementThreshold {
					if movingStart == nil {
						v := prev.Timestamp
						movingStart = &v
					}
					movingEnd = cur.Timestamp
				} else {
					flush()
				}
			}
			flush()
		}
	}

	saved, err := s.repo.ReplaceForVideo(ctx, videoID, allEvents)
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID, eventType, label string, trackID *uuid.UUID) ([]Event, error) {
	return s.repo.GetByVideoID(ctx, videoID, eventType, label, trackID)
}

func (s *Service) GetByTrackID(ctx context.Context, trackID uuid.UUID) ([]Event, error) {
	return s.repo.GetByTrackID(ctx, trackID)
}

func (s *Service) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	return s.repo.DeleteByVideoID(ctx, videoID)
}
