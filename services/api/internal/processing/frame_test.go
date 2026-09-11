package processing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
)

func newFrameDeps(t *testing.T, interval time.Duration) (*video.Service, *video_media.Service, *video_segment.Service, *video_frame.Service, *database.Service, storage.Storage, string) {
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
	videoRepo := video.NewRepository(db.DB)
	videoSvc := video.NewServiceWithConfig(videoRepo, store, 1<<30)
	mediaRepo := video_media.NewRepository(db.DB)
	mediaSvc := video_media.NewService(mediaRepo)
	segRepo := video_segment.NewRepository(db.DB)
	segSvc := video_segment.NewService(segRepo, 30*time.Second)
	frameRepo := video_frame.NewRepository(db.DB)
	frameSvc := video_frame.NewService(frameRepo, store, interval, "ffmpeg", 30*time.Second, 85)
	return videoSvc, mediaSvc, segSvc, frameSvc, db, store, dir
}

func TestFrameExtractionLocal(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("video not found: %v", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	videoSvc, mediaSvc, segSvc, frameSvc, db, store, dir := newFrameDeps(t, 2*time.Second)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	srcType := video.SourceTypeLocal
	v, err := videoSvc.CreateVideo(ctx, "video1.mp4", nil, nil, nil, &srcType, &videoPath)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	if v.Status != video.StatusUploaded {
		if _, err := videoSvc.UpdateStatus(ctx, v.ID, video.StatusUploaded); err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
	}
	defer func() {
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithFrames("ffprobe", 30*time.Second, store, mediaSvc, segSvc, frameSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne: %v", err)
	}
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Fatalf("expected READY got %s err=%v", fetched.Status, fetched.ProcessingError)
	}
	frames, err := frameSvc.GetByVideoID(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetByVideoID: %v", err)
	}
	if len(frames) < 50 || len(frames) > 60 {
		t.Fatalf("expected ~56 frames got %d", len(frames))
	}
	if frames[0].TimestampSeconds != 0 {
		t.Fatalf("first ts %v", frames[0].TimestampSeconds)
	}
	if frames[0].SegmentID == uuid.Nil {
		t.Fatalf("segment not set")
	}
	if frames[0].Width <= 0 || frames[0].Height <= 0 {
		t.Fatalf("dimensions invalid %d %d", frames[0].Width, frames[0].Height)
	}
	if !strings.HasPrefix(frames[0].StorageKey, "videos/") || strings.HasPrefix(frames[0].StorageKey, "/") {
		t.Fatalf("storage_key should be relative videos/... got %s", frames[0].StorageKey)
	}
	for i, f := range frames {
		if f.FrameIndex != i {
			t.Fatalf("index %d mismatch %d", i, f.FrameIndex)
		}
		exists, _ := store.Exists(ctx, f.StorageKey)
		if !exists {
			t.Fatalf("frame file missing %s", f.StorageKey)
		}
	}
	frameRepo2 := video_frame.NewRepository(db.DB)
	frameSvc2 := video_frame.NewService(frameRepo2, store, 1*time.Second, "ffmpeg", 30*time.Second, 85)
	meta, _ := mediaSvc.GetByVideoID(ctx, v.ID)
	segs, _ := segSvc.GetByVideoID(ctx, v.ID)
	frames2, err := frameSvc2.GenerateForVideo(ctx, fetched, segs, *meta.DurationSeconds, 1270, 720)
	if err != nil {
		t.Fatalf("regen 1s: %v", err)
	}
	if len(frames2) <= len(frames) {
		t.Fatalf("1s should produce more frames than 2s: %d vs %d", len(frames2), len(frames))
	}
	framesAfter, _ := frameSvc.GetByVideoID(ctx, v.ID)
	if len(framesAfter) != len(frames2) {
		t.Fatalf("regeneration left stale frames")
	}
}

func TestFrameUpload(t *testing.T) {
	videoPath := "/home/berzz/recallTestVideo/video1.mp4"
	if _, err := os.Stat(videoPath); err != nil {
		t.Skipf("video not found: %v", err)
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	videoSvc, mediaSvc, segSvc, frameSvc, db, store, dir := newFrameDeps(t, 2*time.Second)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	content, _ := os.ReadFile(videoPath)
	tmpFile := "/tmp/upload-frame-test.mp4"
	_ = os.WriteFile(tmpFile, content, 0644)
	defer os.Remove(tmpFile)
	srcType := video.SourceTypeUpload
	hash := uuid.New().String()
	mime := "video/mp4"
	size := int64(len(content))
	v, err := videoSvc.CreateVideo(ctx, "upload.mp4", &hash, &mime, &size, &srcType, nil)
	if err != nil {
		t.Fatalf("CreateVideo: %v", err)
	}
	repo := video.NewRepository(db.DB)
	key := fmt.Sprintf("videos/%s/original.mp4", v.ID.String())
	rf, _ := os.Open(tmpFile)
	if err := store.Save(ctx, key, rf); err != nil {
		t.Fatalf("Save: %v", err)
	}
	rf.Close()
	if _, err := repo.UpdateUpload(ctx, v.ID, key, hash, mime, size, video.StatusUploaded); err != nil {
		t.Fatalf("UpdateUpload: %v", err)
	}
	defer func() {
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithFrames("ffprobe", 30*time.Second, store, mediaSvc, segSvc, frameSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Fatalf("expected READY got %s %v", fetched.Status, fetched.ProcessingError)
	}
	frames, _ := frameSvc.GetByVideoID(ctx, v.ID)
	if len(frames) == 0 {
		t.Fatalf("expected frames for upload")
	}
}

func TestFrameInvalidVideoFails(t *testing.T) {
	videoSvc, mediaSvc, segSvc, frameSvc, db, store, dir := newFrameDeps(t, 2*time.Second)
	defer func() { db.Close(); os.RemoveAll(dir) }()
	ctx := context.Background()
	srcType := video.SourceTypeUpload
	hash := uuid.New().String()
	mime := "video/mp4"
	size := int64(11)
	v, _ := videoSvc.CreateVideo(ctx, "bad.mp4", &hash, &mime, &size, &srcType, nil)
	key := fmt.Sprintf("videos/%s/original.mp4", v.ID.String())
	tmp, _ := os.CreateTemp("", "bad*.mp4")
	tmp.Write([]byte("not a video"))
	tmp.Close()
	defer os.Remove(tmp.Name())
	rf, _ := os.Open(tmp.Name())
	_ = store.Save(ctx, key, rf)
	rf.Close()
	repo := video.NewRepository(db.DB)
	repo.UpdateUpload(ctx, v.ID, key, hash, mime, 11, video.StatusUploaded)
	defer func() {
		frameSvc.DeleteByVideoID(ctx, v.ID)
		segSvc.DeleteByVideoID(ctx, v.ID)
		mediaSvc.DeleteByVideoID(ctx, v.ID)
		videoSvc.DeleteVideo(ctx, v.ID)
	}()
	proc := NewFFprobeProcessorWithFrames("ffprobe", 10*time.Second, store, mediaSvc, segSvc, frameSvc)
	worker := NewWorker(videoSvc, proc, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := videoSvc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusFailed {
		t.Fatalf("expected FAILED got %s", fetched.Status)
	}
	if fetched.ProcessingError == nil || *fetched.ProcessingError == "" {
		t.Fatalf("expected processing_error")
	}
	frames, _ := frameSvc.GetByVideoID(ctx, v.ID)
	if len(frames) != 0 {
		t.Fatalf("should have 0 frames after failure, got %d", len(frames))
	}
}
