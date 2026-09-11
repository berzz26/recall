package segment_description

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/video_event"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/video_track"
	"github.com/berzz26/recall/services/api/internal/vision"
)

type Service struct {
	repo          *Repository
	segmentRepo   *video_segment.Repository
	frameRepo     *video_frame.Repository
	detectionRepo *detection.Repository
	trackRepo     *video_track.Repository
	eventRepo     *video_event.Repository
	describer     vision.VisionDescriber
	maxFrames     int
}

func NewService(repo *Repository, segRepo *video_segment.Repository, frameRepo *video_frame.Repository, detRepo *detection.Repository, trackRepo *video_track.Repository, eventRepo *video_event.Repository, describer vision.VisionDescriber, maxFrames int) *Service {
	if maxFrames <= 0 {
		maxFrames = 6
	}
	if describer == nil {
		describer = vision.NewDeterministicDescriber("deterministic-local", "1", maxFrames)
	}
	return &Service{repo: repo, segmentRepo: segRepo, frameRepo: frameRepo, detectionRepo: detRepo, trackRepo: trackRepo, eventRepo: eventRepo, describer: describer, maxFrames: maxFrames}
}

func selectFrames(frames []video_frame.VideoFrame, seg video_segment.VideoSegment, max int) []video_frame.VideoFrame {
	var inSeg []video_frame.VideoFrame
	for _, f := range frames {
		if f.TimestampSeconds >= seg.StartTime && f.TimestampSeconds < seg.EndTime {
			inSeg = append(inSeg, f)
		}
	}
	if len(inSeg) == 0 {
		return nil
	}
	sort.Slice(inSeg, func(i, j int) bool { return inSeg[i].TimestampSeconds < inSeg[j].TimestampSeconds })
	if len(inSeg) <= max {
		return inSeg
	}
	// evenly distributed
	var out []video_frame.VideoFrame
	step := float64(len(inSeg)) / float64(max)
	for i := 0; i < max; i++ {
		idx := int(float64(i) * step)
		if idx >= len(inSeg) {
			idx = len(inSeg) - 1
		}
		out = append(out, inSeg[idx])
	}
	// ensure last frame is included
	if out[len(out)-1].ID != inSeg[len(inSeg)-1].ID {
		out[len(out)-1] = inSeg[len(inSeg)-1]
	}
	return out
}

func (s *Service) GenerateForVideo(ctx context.Context, videoID uuid.UUID) ([]Description, error) {
	segments, err := s.segmentRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return s.repo.ReplaceForVideo(ctx, videoID, nil)
	}
	frames, err := s.frameRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	dets, err := s.detectionRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	tracksWithCounts, err := s.trackRepo.GetTracksWithCounts(ctx, videoID)
	if err != nil {
		return nil, err
	}
	events, err := s.eventRepo.GetByVideoID(ctx, videoID, "", "", nil)
	if err != nil {
		return nil, err
	}

	// Build maps for quick lookup
	// Detections per label count for whole video, but we need per segment
	// For each segment, filter
	var descs []Description
	for _, seg := range segments {
		selFrames := selectFrames(frames, seg, s.maxFrames)
		// Build frame refs
		var frameRefs []vision.FrameRef
		for _, f := range selFrames {
			frameRefs = append(frameRefs, vision.FrameRef{ID: f.ID, Timestamp: f.TimestampSeconds, Width: f.Width, Height: f.Height})
		}
		// Detections in segment: those whose frame is in segment OR whose timestamp in segment
		var segDets []detection.Detection
		for _, d := range dets {
			// find frame timestamp
			for _, f := range frames {
				if f.ID == d.FrameID && f.TimestampSeconds >= seg.StartTime && f.TimestampSeconds < seg.EndTime {
					segDets = append(segDets, d)
					break
				}
			}
		}
		// Aggregate detections by label
		countByLabel := make(map[string]int)
		for _, d := range segDets {
			countByLabel[d.Label]++
		}
		var detRefs []vision.DetectionRef
		for label, cnt := range countByLabel {
			detRefs = append(detRefs, vision.DetectionRef{Label: label, Count: cnt})
		}
		// Tracks in segment: tracks whose interval overlaps segment
		var segTracks []vision.TrackRef
		for _, tr := range tracksWithCounts {
			// check overlap
			if tr.EndTimestamp < seg.StartTime || tr.StartTimestamp >= seg.EndTime {
				continue
			}
			segTracks = append(segTracks, vision.TrackRef{Label: tr.Label, TrackIndex: tr.TrackIndex, StartTimestamp: tr.StartTimestamp, EndTimestamp: tr.EndTimestamp, DetectionCount: tr.DetectionCount})
		}
		// Events in segment
		var segEvents []vision.EventRef
		for _, e := range events {
			if e.StartTimestamp >= seg.StartTime && e.StartTimestamp < seg.EndTime {
				segEvents = append(segEvents, vision.EventRef{EventType: e.EventType, Label: e.Label, Start: e.StartTimestamp, End: e.EndTimestamp})
			} else if e.EndTimestamp != nil && *e.EndTimestamp >= seg.StartTime && e.StartTimestamp < seg.EndTime {
				segEvents = append(segEvents, vision.EventRef{EventType: e.EventType, Label: e.Label, Start: e.StartTimestamp, End: e.EndTimestamp})
			}
		}

		input := vision.SegmentDescriptionInput{
			VideoID: videoID, SegmentID: seg.ID, StartTime: seg.StartTime, EndTime: seg.EndTime,
			Frames: frameRefs, Detections: detRefs, Tracks: segTracks, Events: segEvents,
		}
		result, err := s.describer.DescribeSegment(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("describe segment %d: %w", seg.SegmentIndex, err)
		}
		if result.Description == "" {
			return nil, fmt.Errorf("empty description for segment %d", seg.SegmentIndex)
		}
		if len(result.Description) > 2000 {
			result.Description = result.Description[:2000]
		}
		if result.ModelName == "" {
			result.ModelName = "deterministic-local"
		}
		if result.ModelVersion == "" {
			result.ModelVersion = "1"
		}
		descs = append(descs, Description{
			VideoID: videoID, SegmentID: seg.ID,
			Description: result.Description,
			ModelName: result.ModelName,
			ModelVersion: result.ModelVersion,
		})
	}

	saved, err := s.repo.ReplaceForVideo(ctx, videoID, descs)
	if err != nil {
		return nil, err
	}
	return saved, nil
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]Description, error) {
	return s.repo.GetByVideoID(ctx, videoID)
}

func (s *Service) GetBySegmentID(ctx context.Context, segmentID uuid.UUID) (*Description, error) {
	return s.repo.GetBySegmentID(ctx, segmentID)
}
