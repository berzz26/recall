package video_frame

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_segment"
)

type Service struct {
	repo           *Repository
	storage        storage.Storage
	sampleInterval time.Duration
	ffmpegPath     string
	ffmpegTimeout  time.Duration
	jpegQuality    int
}

func NewService(repo *Repository, store storage.Storage, sampleInterval time.Duration, ffmpegPath string, ffmpegTimeout time.Duration, jpegQuality int) *Service {
	if sampleInterval <= 0 {
		sampleInterval = 2 * time.Second
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}
	if ffmpegTimeout <= 0 {
		ffmpegTimeout = 60 * time.Second
	}
	if jpegQuality < 1 || jpegQuality > 100 {
		jpegQuality = 85
	}
	return &Service{repo: repo, storage: store, sampleInterval: sampleInterval, ffmpegPath: ffmpegPath, ffmpegTimeout: ffmpegTimeout, jpegQuality: jpegQuality}
}

func SampleTimestamps(durationSeconds float64, interval time.Duration) ([]float64, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("frame sample interval must be > 0")
	}
	if math.IsNaN(durationSeconds) || math.IsInf(durationSeconds, 0) || durationSeconds <= 0 {
		return nil, fmt.Errorf("video duration unavailable; cannot sample frames")
	}
	iv := interval.Seconds()
	if iv <= 0 {
		return nil, fmt.Errorf("frame sample interval must be > 0")
	}
	var ts []float64
	for t := 0.0; t < durationSeconds-1e-9; t += iv {
		ts = append(ts, t)
	}
	if len(ts) == 0 {
		return nil, fmt.Errorf("video duration unavailable; cannot sample frames")
	}
	return ts, nil
}

func FindSegment(segments []video_segment.VideoSegment, timestamp float64) *video_segment.VideoSegment {
	for i := range segments {
		s := &segments[i]
		if timestamp >= s.StartTime && timestamp < s.EndTime {
			return s
		}
	}
	if len(segments) > 0 {
		last := &segments[len(segments)-1]
		if timestamp >= last.StartTime && timestamp <= last.EndTime {
			return last
		}
	}
	return nil
}

func frameStorageKey(videoID uuid.UUID, frameIndex int) string {
	return fmt.Sprintf("videos/%s/frames/%06d.jpg", videoID.String(), frameIndex)
}

func (s *Service) GenerateForVideo(ctx context.Context, v *video.Video, segments []video_segment.VideoSegment, durationSeconds float64, width, height int) ([]VideoFrame, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("storage not configured")
	}
	timestamps, err := SampleTimestamps(durationSeconds, s.sampleInterval)
	if err != nil {
		return nil, err
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("no segments available for frame association")
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid dimensions")
	}

	var videoPath string
	var tempVideo string
	var cleanupVideo func()

	if v.SourceType == video.SourceTypeLocal {
		if v.SourcePath == nil || *v.SourcePath == "" {
			return nil, fmt.Errorf("missing source_path for LOCAL video")
		}
		videoPath = *v.SourcePath
		if _, err := os.Stat(videoPath); err != nil {
			return nil, fmt.Errorf("source file not found: %w", err)
		}
	} else {
		if v.StorageKey == nil || *v.StorageKey == "" {
			return nil, fmt.Errorf("missing storage_key for UPLOAD video")
		}
		rc, err := s.storage.Open(ctx, *v.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("failed to open storage: %w", err)
		}
		ext := filepath.Ext(*v.StorageKey)
		if ext == "" {
			ext = ".mp4"
		}
		tmp, err := os.CreateTemp("", "frame-src-*"+ext)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("failed to create temp video: %w", err)
		}
		tempVideo = tmp.Name()
		cleanupVideo = func() { os.Remove(tempVideo) }
		if _, err := io.Copy(tmp, rc); err != nil {
			tmp.Close()
			rc.Close()
			cleanupVideo()
			return nil, fmt.Errorf("failed to copy to temp video: %w", err)
		}
		tmp.Close()
		rc.Close()
		videoPath = tempVideo
	}
	if cleanupVideo != nil {
		defer cleanupVideo()
	}

	existing, err := s.repo.GetByVideoID(ctx, v.ID)
	if err != nil {
		return nil, err
	}
	var oldKeys []string
	for _, f := range existing {
		oldKeys = append(oldKeys, f.StorageKey)
	}
	if len(existing) > 0 {
		if err := s.repo.DeleteByVideoID(ctx, v.ID); err != nil {
			return nil, fmt.Errorf("failed to delete old frames: %w", err)
		}
		for _, k := range oldKeys {
			_ = s.storage.Delete(ctx, k)
		}
	}

	var createdKeys []string
	var frames []VideoFrame
	cleanupOnFail := func() {
		for _, k := range createdKeys {
			_ = s.storage.Delete(ctx, k)
		}
	}

	for i, ts := range timestamps {
		seg := FindSegment(segments, ts)
		if seg == nil {
			cleanupOnFail()
			return nil, fmt.Errorf("no segment for timestamp %f", ts)
		}
		tmpFile, err := os.CreateTemp("", "frame-*.jpg")
		if err != nil {
			cleanupOnFail()
			return nil, fmt.Errorf("failed to create temp frame: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()

		q := fmt.Sprintf("%d", s.jpegQuality)
		extractCtx, cancel := context.WithTimeout(ctx, s.ffmpegTimeout)
		cmd := exec.CommandContext(extractCtx, s.ffmpegPath,
			"-ss", fmt.Sprintf("%.6f", ts),
			"-i", videoPath,
			"-frames:v", "1",
			"-q:v", q,
			"-y",
			tmpPath,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		err = cmd.Run()
		cancel()
		if err != nil {
			os.Remove(tmpPath)
			cleanupOnFail()
			msg := strings.TrimSpace(stderr.String())
			if len(msg) > 500 {
				msg = msg[:500]
			}
			if msg == "" {
				msg = err.Error()
			}
			if extractCtx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("ffmpeg timeout at timestamp %f: %w", ts, extractCtx.Err())
			}
			return nil, fmt.Errorf("ffmpeg failed at %f: %s", ts, msg)
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			cleanupOnFail()
			return nil, fmt.Errorf("failed to open temp frame: %w", err)
		}
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		fw := 0
		fh := 0
		if err == nil && cfg.Width > 0 && cfg.Height > 0 {
			fw = cfg.Width
			fh = cfg.Height
		} else {
			fw = width
			fh = height
		}

		f2, err := os.Open(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			cleanupOnFail()
			return nil, fmt.Errorf("failed to reopen temp frame: %w", err)
		}
		key := frameStorageKey(v.ID, i)
		if err := s.storage.Save(ctx, key, f2); err != nil {
			f2.Close()
			os.Remove(tmpPath)
			cleanupOnFail()
			return nil, fmt.Errorf("failed to store frame %d: %w", i, err)
		}
		f2.Close()
		os.Remove(tmpPath)
		createdKeys = append(createdKeys, key)

		frames = append(frames, VideoFrame{
			VideoID:          v.ID,
			SegmentID:        seg.ID,
			FrameIndex:       i,
			TimestampSeconds: ts,
			StorageKey:       key,
			Width:            fw,
			Height:           fh,
		})
	}

	saved, err := s.repo.CreateBatch(ctx, frames)
	if err != nil {
		for _, k := range createdKeys {
			_ = s.storage.Delete(ctx, k)
		}
		return nil, fmt.Errorf("failed to persist frames: %w", err)
	}
	return saved, nil
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]VideoFrame, error) {
	return s.repo.GetByVideoID(ctx, videoID)
}

func (s *Service) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	frames, err := s.repo.GetByVideoID(ctx, videoID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteByVideoID(ctx, videoID); err != nil {
		return err
	}
	for _, f := range frames {
		_ = s.storage.Delete(ctx, f.StorageKey)
	}
	return nil
}
