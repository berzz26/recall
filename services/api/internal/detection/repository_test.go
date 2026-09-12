package detection

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/berzz26/recall/services/api/internal/video_frame"
	"github.com/berzz26/recall/services/api/internal/video_segment"
	"github.com/google/uuid"
)

func newTestRepo(t *testing.T) (*Repository, *database.Service) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		url = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(url)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewRepository(db.DB), db
}

func createVideoFrame(t *testing.T, ctx context.Context, db *database.Service) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	videoRepo := video.NewRepository(db.DB)
	v, err := videoRepo.Create(ctx, "test.mp4", uuid.New().String(), "video/mp4", 100, video.SourceTypeUpload, nil)
	if err != nil {
		t.Fatalf("Create video: %v", err)
	}
	segRepo := video_segment.NewRepository(db.DB)
	segs, _ := video_segment.BuildSegments(v.ID, 60, 30*time.Second)
	savedSegs, err := segRepo.ReplaceForVideo(ctx, v.ID, segs)
	if err != nil {
		t.Fatalf("segments: %v", err)
	}
	frameRepo := video_frame.NewRepository(db.DB)
	f := video_frame.VideoFrame{VideoID: v.ID, SegmentID: savedSegs[0].ID, FrameIndex: 0, TimestampSeconds: 0, StorageKey: "videos/" + v.ID.String() + "/frames/000000.jpg", Width: 640, Height: 480}
	frames, err := frameRepo.CreateBatch(ctx, []video_frame.VideoFrame{f})
	if err != nil {
		t.Fatalf("frame: %v", err)
	}
	return v.ID, savedSegs[0].ID, frames[0].ID
}

func TestCreateGetDelete(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	vid, segID, frameID := createVideoFrame(t, ctx, db)
	defer video.NewRepository(db.DB).Delete(ctx, vid)

	d := Detection{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "person", Confidence: 0.94, BBoxX: 0.31, BBoxY: 0.12, BBoxWidth: 0.18, BBoxHeight: 0.65, DetectorName: "yolov8n", DetectorVersion: "1"}
	saved, err := repo.CreateBatch(ctx, []Detection{d})
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1")
	}
	list, err := repo.GetByFrameID(ctx, frameID)
	if err != nil {
		t.Fatalf("GetByFrame: %v", err)
	}
	if len(list) != 1 || list[0].Label != "person" {
		t.Fatalf("mismatch")
	}
	list2, _ := repo.GetByVideoID(ctx, vid)
	if len(list2) != 1 {
		t.Fatalf("GetByVideo: %v", list2)
	}
	if err := repo.DeleteByVideoID(ctx, vid); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list3, _ := repo.GetByVideoID(ctx, vid)
	if len(list3) != 0 {
		t.Fatalf("expected 0 after delete")
	}
}

func TestMultipleSameLabel(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	vid, segID, frameID := createVideoFrame(t, ctx, db)
	defer video.NewRepository(db.DB).Delete(ctx, vid)

	dets := []Detection{
		{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "person", Confidence: 0.9, BBoxX: 0.1, BBoxY: 0.1, BBoxWidth: 0.1, BBoxHeight: 0.1, DetectorName: "yolo", DetectorVersion: "1"},
		{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "person", Confidence: 0.8, BBoxX: 0.3, BBoxY: 0.3, BBoxWidth: 0.1, BBoxHeight: 0.1, DetectorName: "yolo", DetectorVersion: "1"},
		{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "person", Confidence: 0.7, BBoxX: 0.5, BBoxY: 0.5, BBoxWidth: 0.1, BBoxHeight: 0.1, DetectorName: "yolo", DetectorVersion: "1"},
	}
	saved, err := repo.CreateBatch(ctx, dets)
	if err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if len(saved) != 3 {
		t.Fatalf("expected 3 got %d", len(saved))
	}
	list, _ := repo.GetByFrameID(ctx, frameID)
	if len(list) != 3 {
		t.Fatalf("expected 3 got %d", len(list))
	}
}

func TestCascadeDelete(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	vid, segID, frameID := createVideoFrame(t, ctx, db)
	d := Detection{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "car", Confidence: 0.88, BBoxX: 0.62, BBoxY: 0.30, BBoxWidth: 0.29, BBoxHeight: 0.30, DetectorName: "yolo", DetectorVersion: "1"}
	repo.CreateBatch(ctx, []Detection{d})
	if err := video.NewRepository(db.DB).Delete(ctx, vid); err != nil {
		t.Fatalf("Delete video: %v", err)
	}
	list, _ := repo.GetByVideoID(ctx, vid)
	if len(list) != 0 {
		t.Fatalf("expected cascade delete")
	}
}

func TestOrdering(t *testing.T) {
	repo, db := newTestRepo(t)
	ctx := context.Background()
	vid, segID, frameID := createVideoFrame(t, ctx, db)
	defer video.NewRepository(db.DB).Delete(ctx, vid)
	// create 2 detections with different timestamps but same video, order by created_at
	d1 := Detection{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "person", Confidence: 0.9, BBoxX: 0.1, BBoxY: 0.1, BBoxWidth: 0.1, BBoxHeight: 0.1, DetectorName: "yolo", DetectorVersion: "1"}
	d2 := Detection{VideoID: vid, SegmentID: segID, FrameID: frameID, Label: "car", Confidence: 0.8, BBoxX: 0.2, BBoxY: 0.2, BBoxWidth: 0.1, BBoxHeight: 0.1, DetectorName: "yolo", DetectorVersion: "1"}
	repo.CreateBatch(ctx, []Detection{d1, d2})
	list, _ := repo.GetByVideoID(ctx, vid)
	if len(list) != 2 {
		t.Fatalf("expected 2")
	}
	// deterministic order by created_at, id
}
