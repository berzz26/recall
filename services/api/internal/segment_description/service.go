package segment_description

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/video_event"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/video_track"
	"github.com/berzz26/recall/services/api/internal/vision"
	"github.com/google/uuid"
)

type Service struct {
	repo                 *Repository
	segmentRepo          *video_segment.Repository
	frameRepo            *video_frame.Repository
	detectionRepo        *detection.Repository
	trackRepo            *video_track.Repository
	eventRepo            *video_event.Repository
	describer            vision.VisionDescriber
	expectedModelName    string
	expectedModelVersion string
}

func NewService(repo *Repository, segRepo *video_segment.Repository, frameRepo *video_frame.Repository, detRepo *detection.Repository, trackRepo *video_track.Repository, eventRepo *video_event.Repository, describer vision.VisionDescriber, modelName, modelVersion string) *Service {
	return &Service{repo: repo, segmentRepo: segRepo, frameRepo: frameRepo, detectionRepo: detRepo, trackRepo: trackRepo, eventRepo: eventRepo, describer: describer, expectedModelName: modelName, expectedModelVersion: modelVersion}
}

func (s *Service) GenerateForVideo(ctx context.Context, videoID uuid.UUID) ([]Description, error) {
	svcStart := time.Now()
	if s.describer == nil {
		return nil, fmt.Errorf("vision describer not configured")
	}
	segments, err := s.segmentRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		slog.Info("segment_description: no segments, skipping VLM", "video_id", videoID.String(), "duration_ms", time.Since(svcStart).Milliseconds())
		return s.repo.ReplaceForVideo(ctx, videoID, nil)
	}
	slog.Info("segment_description: start", "video_id", videoID.String(), "segments", len(segments))
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

	frameByID := map[uuid.UUID]video_frame.VideoFrame{}
	for _, f := range frames {
		frameByID[f.ID] = f
	}

	var segInputs []vision.SegmentInput
	for _, seg := range segments {
		var segFrames []vision.FrameInput
		for _, f := range frames {
			if f.SegmentID == seg.ID || (f.TimestampSeconds >= seg.StartTime && f.TimestampSeconds < seg.EndTime) {
				segFrames = append(segFrames, vision.FrameInput{ID: f.ID, Timestamp: f.TimestampSeconds, StorageKey: f.StorageKey})
			}
		}
		if len(segFrames) == 0 {
			return nil, fmt.Errorf("segment %d has no frames; cannot describe without visual evidence", seg.SegmentIndex)
		}
		var segDets []vision.DetectionInput
		for _, d := range dets {
			f, ok := frameByID[d.FrameID]
			if !ok {
				continue
			}
			if f.TimestampSeconds >= seg.StartTime && f.TimestampSeconds < seg.EndTime {
				segDets = append(segDets, vision.DetectionInput{Label: d.Label, FrameID: d.FrameID, Timestamp: f.TimestampSeconds})
			}
		}
		var segTracks []vision.TrackInput
		for _, tr := range tracksWithCounts {
			if tr.EndTimestamp < seg.StartTime || tr.StartTimestamp >= seg.EndTime {
				continue
			}
			segTracks = append(segTracks, vision.TrackInput{Label: tr.Label, TrackIndex: tr.TrackIndex, Start: tr.StartTimestamp, End: tr.EndTimestamp})
		}
		var segEvents []vision.EventInput
		for _, e := range events {
			eEnd := e.StartTimestamp
			if e.EndTimestamp != nil {
				eEnd = *e.EndTimestamp
			}
			if eEnd < seg.StartTime || e.StartTimestamp >= seg.EndTime {
				continue
			}
			segEvents = append(segEvents, vision.EventInput{EventType: e.EventType, Label: e.Label, Start: e.StartTimestamp, End: e.EndTimestamp})
		}
		segInputs = append(segInputs, vision.SegmentInput{
			SegmentID: seg.ID, StartTime: seg.StartTime, EndTime: seg.EndTime,
			Frames: segFrames, Detections: segDets, Tracks: segTracks, Events: segEvents,
		})
	}

	// Invoke the VLM exactly once for the video. Descriptions must come
	// from visual inspection; never synthesize from metadata here.
	vlmStart := time.Now()
	slog.Info("segment_description: VLM invoke start", "video_id", videoID.String(), "segments", len(segInputs))
	results, err := s.describer.DescribeVideo(ctx, vision.VideoDescriptionInput{VideoID: videoID, Segments: segInputs})
	vlmMs := time.Since(vlmStart).Milliseconds()
	isRateLimited := err != nil && vision.IsRateLimited(err)
	if err != nil && !isRateLimited {
		slog.Error("segment_description: VLM failed", "video_id", videoID.String(), "duration_ms", vlmMs, "error", err)
		return nil, err
	}
	if isRateLimited {
		if results == nil {
			results = []vision.DescriptionResult{}
		}
		slog.Warn("segment_description: VLM rate limited, persisting partial descriptions", "video_id", videoID.String(), "duration_ms", vlmMs, "descriptions", len(results), "segments", len(segments), "error", err)
		if len(results) == 0 {
			slog.Info("segment_description: no descriptions due to rate limit, skipping (optional)", "video_id", videoID.String(), "duration_ms", vlmMs)
			// Persisting empty clears any stale descriptions but does not fail pipeline since description is optional.
			empty, repErr := s.repo.ReplaceForVideo(ctx, videoID, nil)
			if repErr != nil {
				slog.Error("segment_description: persist failed after rate limit", "video_id", videoID.String(), "error", repErr)
				return nil, repErr
			}
			return empty, nil
		}
	} else {
		slog.Info("segment_description: VLM complete", "video_id", videoID.String(), "segments", len(segInputs), "descriptions", len(results), "duration_ms", vlmMs)
	}
	if !isRateLimited && len(results) != len(segments) {
		return nil, fmt.Errorf("vision returned %d descriptions for %d segments", len(results), len(segments))
	}
	if isRateLimited && len(results) > len(segments) {
		return nil, fmt.Errorf("vision returned %d descriptions for %d segments", len(results), len(segments))
	}
	bySegment := map[uuid.UUID]vision.DescriptionResult{}
	for _, r := range results {
		if r.Description == "" {
			return nil, fmt.Errorf("empty vision description for segment %s", r.SegmentID)
		}
		if _, dup := bySegment[r.SegmentID]; dup {
			return nil, fmt.Errorf("duplicate vision description for segment %s", r.SegmentID)
		}
		bySegment[r.SegmentID] = r
	}

	var descs []Description
	for _, seg := range segments {
		r, ok := bySegment[seg.ID]
		if !ok {
			if isRateLimited {
				slog.Warn("segment_description: missing description for segment due to rate limit, skipping", "video_id", videoID.String(), "segment_index", seg.SegmentIndex, "segment_id", seg.ID.String())
				continue
			}
			return nil, fmt.Errorf("missing vision description for segment %d", seg.SegmentIndex)
		}
		desc := strings.TrimSpace(r.Description)
		if desc == "" {
			return nil, fmt.Errorf("empty vision description for segment %d", seg.SegmentIndex)
		}
		if len(desc) > 2000 {
			return nil, fmt.Errorf("vision description for segment %d exceeds 2000 characters", seg.SegmentIndex)
		}
		if r.SegmentID != seg.ID {
			return nil, fmt.Errorf("vision description segment ID mismatch for segment %d", seg.SegmentIndex)
		}
		if r.ModelName != s.expectedModelName {
			return nil, fmt.Errorf("vision description model name mismatch for segment %d", seg.SegmentIndex)
		}
		if r.ModelVersion != s.expectedModelVersion {
			return nil, fmt.Errorf("vision description model version mismatch for segment %d", seg.SegmentIndex)
		}
		descs = append(descs, Description{
			VideoID: videoID, SegmentID: seg.ID,
			Description: desc,
			ModelName:   r.ModelName, ModelVersion: r.ModelVersion,
		})
	}

	persistStart := time.Now()
	result, err := s.repo.ReplaceForVideo(ctx, videoID, descs)
	persistMs := time.Since(persistStart).Milliseconds()
	totalMs := time.Since(svcStart).Milliseconds()
	if err != nil {
		slog.Error("segment_description: persist failed", "video_id", videoID.String(), "duration_ms", persistMs, "error", err)
		return nil, err
	}
	slog.Info("segment_description: complete", "video_id", videoID.String(), "segments", len(segments), "descriptions", len(result), "vlm_ms", vlmMs, "persist_ms", persistMs, "total_duration_ms", totalMs)
	return result, nil
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]Description, error) {
	return s.repo.GetByVideoID(ctx, videoID)
}

func (s *Service) GetBySegmentID(ctx context.Context, segmentID uuid.UUID) (*Description, error) {
	return s.repo.GetBySegmentID(ctx, segmentID)
}
