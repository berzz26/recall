package video_segment

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/google/uuid"
)

func newTestRepo(t *testing.T) (*Repository, func()) {
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
	return NewRepository(db.DB), func() { db.Close() }
}

func createTestVideo(t *testing.T, ctx context.Context) uuid.UUID {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(databaseURL)
	if err != nil {
		t.Skipf("db not available: %v", err)
	}
	defer db.Close()
	repo := video.NewRepository(db.DB)
	v, err := repo.Create(ctx, "test.mp4", uuid.New().String(), "video/mp4", 100, video.SourceTypeUpload, nil)
	if err != nil {
		t.Fatalf("Create video failed: %v", err)
	}
	return v.ID
}

func TestCreateGetDelete(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	vid := createTestVideo(t, ctx)
	defer func() {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
		}
		db, _ := database.New(databaseURL)
		if db != nil {
			video.NewRepository(db.DB).Delete(ctx, vid)
			db.Close()
		}
	}()

	id := vid
	segs, _ := BuildSegments(id, 60, 30*time.Second)
	inserted, err := repo.ReplaceForVideo(ctx, id, segs)
	if err != nil {
		t.Fatalf("Replace failed: %v", err)
	}
	if len(inserted) != 2 {
		t.Fatalf("expected 2 got %d", len(inserted))
	}

	got, err := repo.GetByVideoID(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 got %d", len(got))
	}
	if got[0].SegmentIndex != 0 || got[1].SegmentIndex != 1 {
		t.Fatalf("index mismatch")
	}

	if err := repo.DeleteByVideoID(ctx, id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	got2, _ := repo.GetByVideoID(ctx, id)
	if len(got2) != 0 {
		t.Fatalf("expected 0 after delete got %d", len(got2))
	}
}

func TestUniqueConstraint(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	vid := createTestVideo(t, ctx)
	defer func() {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
		}
		db, _ := database.New(databaseURL)
		if db != nil {
			video.NewRepository(db.DB).Delete(ctx, vid)
			db.Close()
		}
	}()

	segs, _ := BuildSegments(vid, 60, 30*time.Second)
	if _, err := repo.ReplaceForVideo(ctx, vid, segs); err != nil {
		t.Fatalf("first insert failed: %v", err)
	}
	dup := []VideoSegment{
		{VideoID: vid, SegmentIndex: 0, StartTime: 0, EndTime: 30, Duration: 30},
		{VideoID: vid, SegmentIndex: 0, StartTime: 0, EndTime: 30, Duration: 30},
	}
	_, err := repo.ReplaceForVideo(ctx, vid, dup)
	if err == nil {
		t.Fatalf("expected duplicate error due to unique constraint")
	}
	// ensure still has something? after rollback should have original? but our Replace deletes first then inserts duplicates in same tx -> rollback keeps original deleted? Actually tx will rollback whole, so original segments still there? We deleted then duplicate fails -> rollback so original should be restored? No, we deleted inside tx, rollback undoes delete, so original remains.
	got, _ := repo.GetByVideoID(ctx, vid)
	if len(got) != 2 {
		t.Fatalf("expected original 2 after failed replace got %d", len(got))
	}
}

func TestIdempotency(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	vid := createTestVideo(t, ctx)
	defer func() {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
		}
		db, _ := database.New(databaseURL)
		if db != nil {
			video.NewRepository(db.DB).Delete(ctx, vid)
			db.Close()
		}
	}()

	segs, _ := BuildSegments(vid, 110.94, 30*time.Second)
	repo.ReplaceForVideo(ctx, vid, segs)
	repo.ReplaceForVideo(ctx, vid, segs)
	got, _ := repo.GetByVideoID(ctx, vid)
	if len(got) != 4 {
		t.Fatalf("expected 4 after idempotent double insert got %d", len(got))
	}
}

func TestRegeneration(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	vid := createTestVideo(t, ctx)
	defer func() {
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
		}
		db, _ := database.New(databaseURL)
		if db != nil {
			video.NewRepository(db.DB).Delete(ctx, vid)
			db.Close()
		}
	}()

	s1, _ := BuildSegments(vid, 110, 30*time.Second)
	repo.ReplaceForVideo(ctx, vid, s1)
	got1, _ := repo.GetByVideoID(ctx, vid)
	if len(got1) != 4 {
		t.Fatalf("expected 4 for 110 got %d", len(got1))
	}
	s2, _ := BuildSegments(vid, 140, 30*time.Second)
	repo.ReplaceForVideo(ctx, vid, s2)
	got2, _ := repo.GetByVideoID(ctx, vid)
	if len(got2) != 5 {
		t.Fatalf("expected 5 for 140 got %d", len(got2))
	}
	if got2[4].EndTime != 140 {
		t.Fatalf("expected end 140 got %v", got2[4].EndTime)
	}
}

func TestCascadeDelete(t *testing.T) {
	repo, _ := newTestRepo(t)
	ctx := context.Background()
	vid := createTestVideo(t, ctx)
	segs, _ := BuildSegments(vid, 60, 30*time.Second)
	repo.ReplaceForVideo(ctx, vid, segs)

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, _ := database.New(databaseURL)
	if db == nil {
		t.Fatalf("db nil")
	}
	defer db.Close()
	if err := video.NewRepository(db.DB).Delete(ctx, vid); err != nil {
		t.Fatalf("delete video failed: %v", err)
	}
	got, _ := repo.GetByVideoID(ctx, vid)
	if len(got) != 0 {
		t.Fatalf("expected cascade delete segments got %d", len(got))
	}
}
