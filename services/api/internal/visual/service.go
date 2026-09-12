package visual

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/detector"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video_frame"
)

type Service struct {
	detectionRepo *detection.Repository
	frameRepo     *video_frame.Repository
	storage       storage.Storage
	analyzer      detector.VisualAnalyzer
	threshold     float64
	detectorName  string
	detectorVersion string
}

func NewService(detRepo *detection.Repository, frameRepo *video_frame.Repository, store storage.Storage, analyzer detector.VisualAnalyzer, threshold float64, name, version string) *Service {
	if threshold < 0 || threshold > 1 {
		threshold = 0.25
	}
	if name == "" {
		name = "yolov8n"
	}
	if version == "" {
		version = "1"
	}
	return &Service{detectionRepo: detRepo, frameRepo: frameRepo, storage: store, analyzer: analyzer, threshold: threshold, detectorName: name, detectorVersion: version}
}

func (s *Service) AnalyzeVideo(ctx context.Context, videoID uuid.UUID) ([]detection.Detection, error) {
	visualStart := time.Now()
	frames, err := s.frameRepo.GetByVideoID(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("get frames: %w", err)
	}
	if err := s.detectionRepo.DeleteByVideoID(ctx, videoID); err != nil {
		return nil, fmt.Errorf("delete old detections: %w", err)
	}
	if len(frames) == 0 {
		slog.Info("visual: no frames, skipped", "video_id", videoID.String(), "duration_ms", time.Since(visualStart).Milliseconds())
		return []detection.Detection{}, nil
	}
	if s.analyzer == nil {
		return nil, fmt.Errorf("analyzer not configured")
	}
	slog.Info("visual: start", "video_id", videoID.String(), "frames", len(frames), "threshold", s.threshold, "detector", s.detectorName)
	type tmpFile struct {
		path string
		frame video_frame.VideoFrame
	}
	var tmps []tmpFile
	var inputs []detector.FrameInput
	cleanup := func() {
		for _, t := range tmps {
			os.Remove(t.path)
		}
	}
	defer cleanup()

	for _, f := range frames {
		rc, err := s.storage.Open(ctx, f.StorageKey)
		if err != nil {
			return nil, fmt.Errorf("open frame %s: %w", f.ID, err)
		}
		ext := filepath.Ext(f.StorageKey)
		if ext == "" {
			ext = ".jpg"
		}
		tmp, err := os.CreateTemp("", "visual-*"+ext)
		if err != nil {
			rc.Close()
			return nil, err
		}
		if _, err := io.Copy(tmp, rc); err != nil {
			tmp.Close()
			rc.Close()
			os.Remove(tmp.Name())
			return nil, fmt.Errorf("copy frame %s: %w", f.ID, err)
		}
		tmp.Close()
		rc.Close()
		p := tmp.Name()
		tmps = append(tmps, tmpFile{path: p, frame: f})
		inputs = append(inputs, detector.FrameInput{
			FrameID: f.ID, VideoID: f.VideoID, SegmentID: f.SegmentID, Timestamp: f.TimestampSeconds, Width: f.Width, Height: f.Height, StorageKey: f.StorageKey, LocalPath: p,
		})
	}

	prepMs := time.Since(visualStart).Milliseconds()
	slog.Info("visual: frames prepared for detection", "video_id", videoID.String(), "frames", len(inputs), "prep_duration_ms", prepMs)
	analyzeStart := time.Now()
	results, err := s.analyzer.AnalyzeBatch(ctx, inputs)
	analyzeMs := time.Since(analyzeStart).Milliseconds()
	if err != nil {
		slog.Error("visual: detection failed", "video_id", videoID.String(), "duration_ms", analyzeMs, "error", err)
		return nil, fmt.Errorf("analyze: %w", err)
	}
	slog.Info("visual: detection complete", "video_id", videoID.String(), "frames", len(inputs), "duration_ms", analyzeMs)

	var toInsert []detection.Detection
	for _, f := range frames {
		dets := results[f.ID]
		for _, r := range dets {
			if r.Confidence < s.threshold {
				continue
			}
			if r.Label == "" {
				continue
			}
			x, y, w, h := detection.ClampBBox(r.BBoxX, r.BBoxY, r.BBoxWidth, r.BBoxHeight)
			if w <= 0 || h <= 0 {
				continue
			}
			if r.Confidence < 0 || r.Confidence > 1 {
				continue
			}
			toInsert = append(toInsert, detection.Detection{
				VideoID: f.VideoID, SegmentID: f.SegmentID, FrameID: f.ID, Label: r.Label, Confidence: r.Confidence,
				BBoxX: x, BBoxY: y, BBoxWidth: w, BBoxHeight: h,
				DetectorName: s.detectorName, DetectorVersion: s.detectorVersion,
			})
		}
	}

	persistStart := time.Now()
	saved, err := s.detectionRepo.CreateBatch(ctx, toInsert)
	persistMs := time.Since(persistStart).Milliseconds()
	if err != nil {
		slog.Error("visual: persist failed", "video_id", videoID.String(), "duration_ms", persistMs, "error", err)
		return nil, fmt.Errorf("persist detections: %w", err)
	}
	totalMs := time.Since(visualStart).Milliseconds()
	slog.Info("visual: complete",
		"video_id", videoID.String(),
		"frames", len(frames),
		"detections", len(saved),
		"prep_ms", prepMs,
		"analyze_ms", analyzeMs,
		"persist_ms", persistMs,
		"total_duration_ms", totalMs,
	)
	return saved, nil
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) ([]detection.Detection, error) {
	return s.detectionRepo.GetByVideoID(ctx, videoID)
}

func (s *Service) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	return s.detectionRepo.DeleteByVideoID(ctx, videoID)
}
