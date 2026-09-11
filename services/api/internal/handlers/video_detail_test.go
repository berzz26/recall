package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/detection"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_media"
	"github.com/berzz26/recall/services/api/internal/video_segment"
)

func TestGetFrameImage_ReturnsJPEG(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://berzz:strongpassword@100.108.102.22:5432/recall?sslmode=disable"
	}
	db, err := database.New(url)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	store, _ := storage.NewLocalStorage(dir)
	videoRepo := video.NewRepository(db.DB)
	videoSvc := video.NewServiceWithConfig(videoRepo, store, 1<<30)

	ctx := context.Background()
	v, err := videoSvc.CreateVideo(ctx, "test.mp4", strPtr(uuid.NewString()), strPtr("video/mp4"), int64Ptr(100), func() *video.SourceType { s := video.SourceTypeUpload; return &s }(), nil)
	if err != nil {
		v, _ = videoRepo.Create(ctx, "test.mp4", uuid.NewString(), "video/mp4", 100, video.SourceTypeUpload, nil)
	}
	segRepo := video_segment.NewRepository(db.DB)
	segs, _ := video_segment.BuildSegments(v.ID, 60, 30*1000000000)
	savedSegs, _ := segRepo.ReplaceForVideo(ctx, v.ID, segs)
	segID := savedSegs[0].ID
	if len(savedSegs) == 0 {
		t.Fatalf("no segments")
	}
	// create frame with real jpeg bytes via storage
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9}
	frameRepo := video_frame.NewRepository(db.DB)
	key := "videos/" + v.ID.String() + "/frames/000000.jpg"
	_ = store.Save(ctx, key, bytes.NewReader(jpeg))
	f := video_frame.VideoFrame{VideoID: v.ID, SegmentID: segID, FrameIndex: 0, TimestampSeconds: 0, StorageKey: key, Width: 640, Height: 480}
	savedFrames, err := frameRepo.CreateBatch(ctx, []video_frame.VideoFrame{f})
	if err != nil {
		t.Fatalf("create frame: %v", err)
	}
	fid := savedFrames[0].ID
	defer func() {
		frameRepo.DeleteByVideoID(ctx, v.ID)
		segRepo.DeleteByVideoID(ctx, v.ID)
		videoRepo.Delete(ctx, v.ID)
	}()

	// handler
	h := NewVideoDetailHandler(video_media.NewRepository(db.DB), segRepo, frameRepo, detection.NewRepository(db.DB), store)
	app := fiber.New()
	app.Get("/api/v1/videos/:id/frames/:frameId/image", h.GetFrameImage)

	req := httptest.NewRequest("GET", "/api/v1/videos/"+v.ID.String()+"/frames/"+fid.String()+"/image", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 got %d body %s", resp.StatusCode, string(body))
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "image/jpeg" {
		t.Fatalf("expected image/jpeg got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatalf("empty body")
	}
	if body[0] != 0xFF || body[1] != 0xD8 {
		t.Fatalf("not jpeg magic")
	}
	// verify non-existent frame returns 404
	req2 := httptest.NewRequest("GET", "/api/v1/videos/"+v.ID.String()+"/frames/"+uuid.NewString()+"/image", nil)
	resp2, _ := app.Test(req2)
	if resp2.StatusCode != 404 {
		t.Fatalf("expected 404 for missing frame got %d", resp2.StatusCode)
	}
}

func strPtr(s string) *string { return &s }
func int64Ptr(i int64) *int64 { return &i }
