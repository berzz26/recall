package processing

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/detector"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/berzz26/recall/services/api/internal/visual"
)

func newDetectionDeps(t *testing.T, analyzer detector.VisualAnalyzer) (*video.Service, *video_media.Service, *video_segment.Service, *video_frame.Service, *visual.Service, *database.Service, storage.Storage, string) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(url)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	dir := t.TempDir()
	store, _ := storage.NewLocalStorage(dir)
	videoSvc := video.NewServiceWithConfig(video.NewRepository(db.DB), store, 1<<30)
	mediaSvc := video_media.NewService(video_media.NewRepository(db.DB))
	segSvc := video_segment.NewService(video_segment.NewRepository(db.DB), 30*time.Second)
	frameSvc := video_frame.NewService(video_frame.NewRepository(db.DB), store, 2*time.Second, "ffmpeg", 30*time.Second, 85)
	detRepo := detection.NewRepository(db.DB)
	visualSvc := visual.NewService(detRepo, video_frame.NewRepository(db.DB), store, analyzer, 0.25, "test-detector", "1")
	return videoSvc, mediaSvc, segSvc, frameSvc, visualSvc, db, store, dir
}

func TestVisualWithFakeAnalyzer(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("video not found: %v", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	// use fake to avoid needing real model
	fake := &detector.FakeVisualAnalyzer{}
	videoSvc, mediaSvc, segSvc, frameSvc, visualSvc, db, store, dir := newDetectionDeps(t, fake)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	srcType := video.SourceTypeLocal
	v, err := videoSvc.CreateVideo(ctx, "video1.mp4", nil, nil, nil, &srcType, &videoPath)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	if v.Status != video.StatusUploaded {
		videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded)
	}
	defer func() {
		visualSvc.DeleteByVideoID(ctx, v.ID)
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithVisual("ffprobe", 30*time.Second, store, mediaSvc, segSvc, frameSvc, visualSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Fatalf("expected READY got %s err=%v", fetched.Status, fetched.ProcessingError)
	}
	dets, err := visualSvc.GetByVideoID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetByVideoID: %v", err)
	}
	if len(dets) == 0 {
		t.Fatalf("expected detections got 0")
	}
	// check bbox valid
	for _, d := range dets {
		if d.Confidence < 0 || d.Confidence > 1 {
			t.Fatalf("confidence invalid %v", d.Confidence)
		}
		if d.BBoxX < 0 || d.BBoxY < 0 || d.BBoxWidth <= 0 || d.BBoxHeight <= 0 || d.BBoxX+d.BBoxWidth > 1.000001 {
			t.Fatalf("bbox invalid %+v", d)
		}
		if d.DetectorName != "test-detector" {
			t.Fatalf("detector name mismatch %s", d.DetectorName)
		}
	}
	// check that frame_id maps to valid frames
	frames, _ := frameSvc.GetByVideoID(ctx, v.ID)
	if len(frames) == 0 {
		t.Fatalf("no frames")
	}
	// each detection's frame_id should be one of frames
	frameIDs := map[uuid.UUID]bool{}
	for _, f := range frames {
		frameIDs[f.ID] = true
	}
	for _, d := range dets {
		if !frameIDs[d.FrameID] {
			t.Fatalf("detection frame_id not in frames")
		}
	}
}

func TestVisualFailureMarksFailed(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("video not found: %v", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	fake := &detector.FakeVisualAnalyzer{Err: ErrFakeFail}
	videoSvc, mediaSvc, segSvc, frameSvc, visualSvc, db, store, dir := newDetectionDeps(t, fake)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	srcType := video.SourceTypeLocal
	v, _ := videoSvc.CreateVideo(ctx, "video1.mp4", nil, nil, nil, &srcType, &videoPath)
	videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded)
	defer func() {
		visualSvc.DeleteByVideoID(ctx, v.ID)
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithVisual("ffprobe", 30*time.Second, store, mediaSvc, segSvc, frameSvc, visualSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusFailed {
		t.Fatalf("expected FAILED got %s", fetched.Status)
	}
	if fetched.ProcessingError == nil || *fetched.ProcessingError == "" {
		t.Fatalf("expected processing_error")
	}
	dets, _ := visualSvc.GetByVideoID(ctx, v.ID)
	if len(dets) != 0 {
		t.Fatalf("expected 0 detections after failure got %d", len(dets))
	}
}

var ErrFakeFail = errString("fake analyzer failure")
type errString string
func (e errString) Error() string { return string(e) }

func TestYoloIntegrationSkippedIfNoModel(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("video not found: %v", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	script := "/home/berzz/recall/workers/detector/detect.py"
	if _, err := os.Stat(script); err != nil {
		t.Skip("detector script not found")
	}
	// try yolo with real model - may be skipped if ultralytics not installed
	storeDir := t.TempDir()
	store, _ := storage.NewLocalStorage(storeDir)
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(url)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	defer db.Close()
	defer os.RemoveAll(storeDir)
	ctx := context.Background()
	videoSvc := video.NewServiceWithConfig(video.NewRepository(db.DB), store, 1<<30)
	mediaSvc := video_media.NewService(video_media.NewRepository(db.DB))
	segSvc := video_segment.NewService(video_segment.NewRepository(db.DB), 30*time.Second)
	frameSvc := video_frame.NewService(video_frame.NewRepository(db.DB), store, 2*time.Second, "ffmpeg", 30*time.Second, 85)
	detRepo := detection.NewRepository(db.DB)
	venvPython := "/home/berzz/recall/workers/detector/.venv/bin/python"
	modelPath := "/home/berzz/recall/workers/detector/yolov8n.pt"
	if _, err := os.Stat(venvPython); err != nil {
		venvPython = "python3"
		modelPath = ""
	}
	yolo := detector.NewYoloDetector(venvPython, script, modelPath, 0.25)
	visualSvc := visual.NewService(detRepo, video_frame.NewRepository(db.DB), store, yolo, 0.25, "yolov8n", "1")
	srcType := video.SourceTypeLocal
	v, _ := videoSvc.CreateVideo(ctx, "video1.mp4", nil, nil, nil, &srcType, &videoPath)
	videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded)
	defer func() {
		visualSvc.DeleteByVideoID(ctx, v.ID)
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithVisual("ffprobe", 30*time.Second, store, mediaSvc, segSvc, frameSvc, visualSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Skipf("yolo not available, got status %s err %v", fetched.Status, fetched.ProcessingError)
	}
	dets, _ := visualSvc.GetByVideoID(ctx, v.ID)
	t.Logf("yolo produced %d detections", len(dets))
}
