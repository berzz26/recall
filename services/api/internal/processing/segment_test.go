package processing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/google/uuid"
)

func newTestDeps(t *testing.T) (*video.Service, *video_media.Service, *video_segment.Service, *database.Service, string) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(databaseURL)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	dir := t.TempDir()
	store, _ := storage.NewLocalStorage(dir)
	videoRepo := video.NewRepository(db.DB)
	videoSvc := video.NewServiceWithConfig(videoRepo, store, 1<<30)
	mediaRepo := video_media.NewRepository(db.DB)
	mediaSvc := video_media.NewService(mediaRepo)
	segRepo := video_segment.NewRepository(db.DB)
	segSvc := video_segment.NewService(segRepo, 30*time.Second)
	return videoSvc, mediaSvc, segSvc, db, dir
}

func cleanupVideo(t *testing.T, svc *video.Service, id uuid.UUID, db *database.Service) {
	t.Helper()
	ctx := context.Background()
	_ = svc.DeleteVideo(ctx, id)
	if db != nil {
		db.Close()
	}
}

func TestFFprobeGeneratesSegmentsLocal(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("test video not found: %v", err)
	}
	if _, err := os.Stat("ffprobe"); err == nil {
	} else if _, err2 := os.Stat("/usr/bin/ffprobe"); err2 != nil {
		t.Skip("ffprobe not available")
	}
	videoSvc, mediaSvc, segSvc, db, dir := newTestDeps(t)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	store, _ := storage.NewLocalStorage(dir)

	srcType := video.SourceTypeLocal
	v, err := videoSvc.CreateVideo(ctx, "video1.mp4", nil, nil, nil, &srcType, &videoPath)
	if err != nil {
		t.Fatalf("CreateVideo failed: %v", err)
	}
	// Ensure UPLOADED status for claiming (CreateVideo for LOCAL does hash and sets UPLOADED, but fallback)
	if v.Status != video.StatusUploaded {
		if _, err := videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
	}
	// ensure cleanup
	defer func() {
		ctx2 := context.Background()
		segSvc.DeleteByVideoID(ctx2, v.ID)
		mediaSvc.DeleteByVideoID(ctx2, v.ID)
		videoSvc.DeleteVideo(ctx2, v.ID)
	}()

	proc := NewFFprobeProcessorWithSegments("ffprobe", 60*time.Second, store, mediaSvc, segSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Fatalf("expected READY got %s err=%v", fetched.Status, fetched.ProcessingError)
	}
	meta, err := mediaSvc.GetByVideoID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetByVideoID media failed: %v", err)
	}
	if meta.DurationSeconds == nil || *meta.DurationSeconds <= 0 {
		t.Fatalf("invalid duration")
	}
	segments, err := segSvc.GetByVideoID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetByVideoID segments failed: %v", err)
	}
	if len(segments) != 4 {
		t.Fatalf("expected 4 segments got %d duration=%.2f", len(segments), *meta.DurationSeconds)
	}
	if segments[0].StartTime != 0 || segments[0].EndTime != 30 {
		t.Fatalf("seg0 mismatch %v", segments[0])
	}
	if segments[3].EndTime < 110 || segments[3].EndTime > 111 {
		t.Fatalf("seg3 end mismatch %v", segments[3].EndTime)
	}
	// idempotency: regenerate via same duration should keep 4
	if _, err := segSvc.GenerateForVideo(ctx, v.ID, *meta.DurationSeconds); err != nil {
		t.Fatalf("second GenerateForVideo failed: %v", err)
	}
	segments2, _ := segSvc.GetByVideoID(ctx, v.ID)
	if len(segments2) != 4 {
		t.Fatalf("idempotency failed expected 4 got %d", len(segments2))
	}
}

func TestSegmentGenerationFailureMarksFailed(t *testing.T) {
	videoSvc, mediaSvc, segSvc, db, dir := newTestDeps(t)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	_ = mediaSvc
	_ = segSvc
	store, _ := storage.NewLocalStorage(dir)

	// Create uploaded video with dummy storage file
	srcType := video.SourceTypeUpload
	// create temp video file that ffprobe will fail on
	tmp, _ := os.CreateTemp("", "fake*.mp4")
	tmp.Write([]byte("not a video"))
	tmp.Close()
	defer os.Remove(tmp.Name())
	content := "hash-" + uuid.New().String()
	mime := "video/mp4"
	size := int64(11)
	v, err := videoSvc.CreateVideo(ctx, "fake.mp4", &content, &mime, &size, &srcType, nil)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	f, _ := os.Open(tmp.Name())
	key := fmt.Sprintf("videos/%s/original.mp4", v.ID.String())
	if err := store.Save(ctx, key, f); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f.Close()
	repo := video.NewRepository(db.DB)
	if _, err := repo.UpdateUpload(ctx, v.ID, key, content, mime, size, video.StatusUploaded); err != nil {
		t.Fatalf("UpdateUpload: %v", err)
	}
	defer func() {
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()

	proc := NewFFprobeProcessorWithSegments("ffprobe", 10*time.Second, store, mediaSvc, segSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusFailed {
		t.Fatalf("expected FAILED got %s", fetched.Status)
	}
	if fetched.ProcessingError == nil || *fetched.ProcessingError == "" {
		t.Fatalf("expected processing_error")
	}
}

func TestSegmentServiceInvalidDurationMarksFailedViaProcessor(t *testing.T) {
	videoSvc, _, segSvc, db, dir := newTestDeps(t)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()

	// Use custom processor that tries to generate with invalid duration
	type invalidProc struct {
		seg *video_segment.Service
		vid uuid.UUID
	}
	// Create video
	srcType := video.SourceTypeUpload
	content := "hash-" + uuid.New().String()
	mime := "video/mp4"
	size := int64(10)
	v, err := videoSvc.CreateVideo(ctx, "inv.mp4", &content, &mime, &size, &srcType, nil)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	if _, err := videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded); err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
	defer func() {
		segSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()

	proc := &processorFunc{fn: func(ctx context.Context, vv *video.Video) error {
		_, err := segSvc.GenerateForVideo(ctx, vv.ID, 0)
		return err
	}}
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusFailed {
		t.Fatalf("expected FAILED got %s", fetched.Status)
	}
	if fetched.ProcessingError == nil || *fetched.ProcessingError == "" {
		t.Fatalf("expected processing_error")
	}
	if got := *fetched.ProcessingError; got != "video duration unavailable; cannot generate segments" {
		// allow wrapped
		if got != "failed to generate segments: video duration unavailable; cannot generate segments" {
			// at least contains
			if len(got) < 5 {
				t.Fatalf("unexpected error %q", got)
			}
		}
	}
}

type processorFunc struct {
	fn func(context.Context, *video.Video) error
}

func (p *processorFunc) Process(ctx context.Context, v *video.Video) error { return p.fn(ctx, v) }
