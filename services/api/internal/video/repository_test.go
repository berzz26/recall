package video

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/berzz26/recall/pkg/database"
)

func newTestRepo(t *testing.T) *Repository {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(databaseURL)
	if err != nil {
		t.Skipf("skipping integration test, database not available: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewRepository(db.DB)
}

func strPtr(s string) *string { return &s }

func TestVideoCRUD(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, "meeting.mp4", "abc123", "video/mp4", 123456789, SourceTypeUpload, nil)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.Status != StatusUploading {
		t.Fatalf("expected UPLOADING got %s", created.Status)
	}
	if created.SourceType != SourceTypeUpload {
		t.Fatalf("expected UPLOAD got %s", created.SourceType)
	}
	if created.StorageKey != nil {
		t.Fatalf("expected storage_key nil")
	}

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if fetched.ID != created.ID {
		t.Fatalf("mismatched id")
	}

	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) == 0 {
		t.Fatalf("expected at least 1 video")
	}

	updated, err := repo.UpdateStatus(ctx, created.ID, StatusReady)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if updated.Status != StatusReady {
		t.Fatalf("expected READY got %s", updated.Status)
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = repo.GetByID(ctx, created.ID)
	if err == nil {
		t.Fatalf("expected error after delete")
	}
}

func TestVideoLocalSource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	path := "/cctv/camera-01/incident.mp4"
	created, err := repo.Create(ctx, "incident.mp4", "hashlocal123", "video/mp4", 999, SourceTypeLocal, &path)
	if err != nil {
		t.Fatalf("Create LOCAL failed: %v", err)
	}
	if created.SourceType != SourceTypeLocal {
		t.Fatalf("expected LOCAL got %s", created.SourceType)
	}
	if created.SourcePath == nil || *created.SourcePath != path {
		t.Fatalf("expected source_path %s got %v", path, created.SourcePath)
	}
	if created.StorageKey != nil {
		t.Fatalf("expected storage_key nil for LOCAL")
	}

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID LOCAL failed: %v", err)
	}
	if fetched.SourcePath == nil || *fetched.SourcePath != path {
		t.Fatalf("persisted source_path mismatch")
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete LOCAL failed: %v", err)
	}
}

func TestVideoUploadSource(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	created, err := repo.Create(ctx, "upload.mp4", "hashupload", "video/mp4", 123, SourceTypeUpload, nil)
	if err != nil {
		t.Fatalf("Create UPLOAD failed: %v", err)
	}
	if created.SourceType != SourceTypeUpload {
		t.Fatalf("expected UPLOAD got %s", created.SourceType)
	}
	if created.SourcePath != nil {
		t.Fatalf("expected source_path nil for UPLOAD")
	}
	if created.StorageKey != nil {
		t.Fatalf("expected storage_key nil")
	}

	fetched, err := repo.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID UPLOAD failed: %v", err)
	}
	if fetched.SourceType != SourceTypeUpload {
		t.Fatalf("persisted source_type mismatch")
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete UPLOAD failed: %v", err)
	}
}

func TestVideoListEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	_, _ = repo.Create(ctx, "a.mp4", "hash1", "video/mp4", 100, SourceTypeUpload, nil)
	_, _ = repo.Create(ctx, "b.mp4", "hash2", "video/mp4", 200, SourceTypeLocal, strPtr("/tmp/b.mp4"))

	list, err := repo.List(ctx, 5, 0)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 videos got %d", len(list))
	}
}

func TestVideoNotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	_, err := repo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Fatalf("expected not found error")
	}
}
