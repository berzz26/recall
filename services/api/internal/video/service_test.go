package video

import (
	"context"
	"os"
	"testing"

	"github.com/berzz26/recall/pkg/database"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(databaseURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	repo := NewRepository(db.DB)
	return NewService(repo)
}

func strPtr2(s string) *string { return &s }
func int64Ptr(i int64) *int64 { return &i }
func sourcePtr(s SourceType) *SourceType { return &s }

func TestServiceCreateAndGet(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	v, err := svc.CreateVideo(ctx, "service.mp4", strPtr2("hashsvc"), strPtr2("video/mp4"), int64Ptr(999), sourcePtr(SourceTypeUpload), nil)
	if err != nil {
		t.Fatalf("CreateVideo failed: %v", err)
	}
	if v.Filename != "service.mp4" {
		t.Fatalf("unexpected filename")
	}
	if v.SourceType != SourceTypeUpload {
		t.Fatalf("expected UPLOAD")
	}

	fetched, err := svc.GetVideo(ctx, v.ID)
	if err != nil {
		t.Fatalf("GetVideo failed: %v", err)
	}
	if fetched.ID != v.ID {
		t.Fatalf("id mismatch")
	}

	if err := svc.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestServiceCreateLocal(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	path := "/cctv/camera-01/video.mp4"
	v, err := svc.CreateVideo(ctx, "local.mp4", strPtr2("hash123"), strPtr2("video/mp4"), int64Ptr(123), sourcePtr(SourceTypeLocal), &path)
	if err != nil {
		t.Fatalf("CreateVideo LOCAL failed: %v", err)
	}
	if v.SourcePath == nil || *v.SourcePath != path {
		t.Fatalf("source_path not persisted")
	}
	if v.StorageKey != nil {
		t.Fatalf("storage_key should be nil")
	}
	if err := svc.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestServiceCreateLocalMissingPath(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateVideo(ctx, "local.mp4", strPtr2("hash"), strPtr2("video/mp4"), int64Ptr(100), sourcePtr(SourceTypeLocal), nil)
	if err == nil {
		t.Fatalf("expected error for missing source_path")
	}
}

func TestServiceCreateUploadMinimal(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	v, err := svc.CreateVideo(ctx, "upload.mp4", nil, nil, nil, sourcePtr(SourceTypeUpload), nil)
	if err != nil {
		t.Fatalf("CreateVideo minimal UPLOAD failed: %v", err)
	}
	if v.SourceType != SourceTypeUpload {
		t.Fatalf("expected UPLOAD")
	}
	if v.SourcePath != nil {
		t.Fatalf("source_path should be nil for UPLOAD")
	}
	if err := svc.DeleteVideo(ctx, v.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}
