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

func TestServiceCreateAndGet(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	v, err := svc.CreateVideo(ctx, "service.mp4", "hashsvc", "video/mp4", 999)
	if err != nil {
		t.Fatalf("CreateVideo failed: %v", err)
	}
	if v.Filename != "service.mp4" {
		t.Fatalf("unexpected filename")
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
