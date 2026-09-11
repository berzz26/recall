package video_segment

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo            *Repository
	segmentDuration time.Duration
}

func NewService(repo *Repository, segmentDuration time.Duration) *Service {
	if segmentDuration <= 0 {
		segmentDuration = 30 * time.Second
	}
	return &Service{repo: repo, segmentDuration: segmentDuration}
}

func BuildSegments(videoID uuid.UUID, durationSeconds float64, segmentDuration time.Duration) ([]VideoSegment, error) {
	if segmentDuration <= 0 {
		return nil, fmt.Errorf("segment duration must be > 0")
	}
	if math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) || durationSeconds <= 0 {
		return nil, fmt.Errorf("video duration unavailable; cannot generate segments")
	}
	segSec := segmentDuration.Seconds()
	if segSec <= 0 {
		return nil, fmt.Errorf("segment duration must be > 0")
	}
	n := int(math.Ceil(durationSeconds / segSec))
	if n <= 0 {
		return nil, fmt.Errorf("video duration unavailable; cannot generate segments")
	}
	segments := make([]VideoSegment, 0, n)
	for i := 0; i < n; i++ {
		start := float64(i) * segSec
		end := float64(i+1) * segSec
		if end > durationSeconds {
			end = durationSeconds
		}
		if start >= end {
			break
		}
		dur := end - start
		if dur <= 0 {
			break
		}
		segments = append(segments, VideoSegment{
			VideoID:      videoID,
			SegmentIndex: i,
			StartTime:    start,
			EndTime:      end,
			Duration:     dur,
		})
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("video duration unavailable; cannot generate segments")
	}
	return segments, nil
}

func (s *Service) GenerateForVideo(ctx context.Context, videoID uuid.UUID, durationSeconds float64) ([]VideoSegment, error) {
	segments, err := BuildSegments(videoID, durationSeconds, s.segmentDuration)
	if err != nil {
		return nil, err
	}
	return s.repo.ReplaceForVideo(ctx, videoID, segments)
}

func (s *Service) GenerateForVideoWithDuration(ctx context.Context, videoID uuid.UUID, durationSeconds float64, segmentDuration time.Duration) ([]VideoSegment, error) {
	segments, err := BuildSegments(videoID, durationSeconds, segmentDuration)
	if err != nil {
		return nil, err
	}
	return s.repo.ReplaceForVideo(ctx, videoID, segments)
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]VideoSegment, error) {
	return s.repo.GetByVideoID(ctx, videoID)
}

func (s *Service) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	return s.repo.DeleteByVideoID(ctx, videoID)
}
