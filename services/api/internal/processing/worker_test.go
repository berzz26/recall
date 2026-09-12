package processing

import (
	"bytes"
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
	"github.com/google/uuid"
)

func newTestVideoService(t *testing.T) (*video.Service, func()) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}
	db, err := database.New(databaseURL)
	if err != nil {
		t.Skipf("database not available: %v", err)
	}
	dir := t.TempDir()
	store, _ := storage.NewLocalStorage(dir)
	repo := video.NewRepository(db.DB)
	svc := video.NewServiceWithConfig(repo, store, 1<<30)
	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}
	// clean leftover UPLOADED/PROCESSING videos for isolation
	ctx := context.Background()
	repo2 := video.NewRepository(db.DB)
	list, _ := repo2.List(ctx, 1000, 0)
	for _, v := range list {
		if v.Status == video.StatusUploaded || v.Status == video.StatusProcessing {
			repo2.Delete(ctx, v.ID)
		}
	}
	return svc, cleanup
}

func createUploadedVideo(t *testing.T, svc *video.Service) *video.Video {
	t.Helper()
	ctx := context.Background()
	content := "hash-" + uuid.New().String()
	mime := "video/mp4"
	size := int64(100)
	src := video.SourceTypeUpload
	v, err := svc.CreateVideo(ctx, "test.mp4", &content, &mime, &size, &src, nil)
	if err != nil {
		t.Fatalf("CreateVideo failed: %v", err)
	}
	updated, err := svc.UpdateStatus(ctx, v.ID, video.StatusUploaded)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	return updated
}

func TestClaiming(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()

	v := createUploadedVideo(t, svc)

	claimed, err := svc.ClaimNext(ctx)
	if err != nil {
		t.Fatalf("ClaimNext failed: %v", err)
	}
	if claimed == nil {
		t.Fatalf("expected claimed video")
	}
	if claimed.ID != v.ID {
		t.Fatalf("claimed wrong video")
	}
	if claimed.Status != video.StatusProcessing {
		t.Fatalf("expected PROCESSING got %s", claimed.Status)
	}

	claimed2, err := svc.ClaimNext(ctx)
	if err != nil {
		t.Fatalf("second ClaimNext error: %v", err)
	}
	if claimed2 != nil {
		t.Fatalf("expected no video when already PROCESSING")
	}

	svc.MarkReady(ctx, v.ID)
	claimed3, _ := svc.ClaimNext(ctx)
	if claimed3 != nil {
		t.Fatalf("READY should not be claimable")
	}

	v2 := createUploadedVideo(t, svc)
	svc.MarkFailed(ctx, v2.ID, "error")
	claimed4, _ := svc.ClaimNext(ctx)
	if claimed4 != nil {
		t.Fatalf("FAILED should not be claimable")
	}
	svc.DeleteVideo(ctx, v.ID)
	svc.DeleteVideo(ctx, v2.ID)
}

func TestConcurrency(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()

	n := 5
	ids := make(map[string]bool)
	for i := 0; i < n; i++ {
		v := createUploadedVideo(t, svc)
		ids[v.ID.String()] = true
	}

	var mu sync.Mutex
	claimed := make(map[string]bool)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := svc.ClaimNext(ctx)
			if err == nil && v != nil {
				mu.Lock()
				if claimed[v.ID.String()] {
					t.Errorf("duplicate claim %s", v.ID)
				}
				claimed[v.ID.String()] = true
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	mu.Lock()
	if len(claimed) != n {
		t.Fatalf("expected %d claimed got %d", n, len(claimed))
	}
	mu.Unlock()

	for idStr := range ids {
		parsed, _ := uuid.Parse(idStr)
		fetched, _ := svc.GetVideo(ctx, parsed)
		if fetched.Status != video.StatusProcessing {
			t.Fatalf("expected PROCESSING")
		}
		svc.DeleteVideo(ctx, parsed)
	}
}

func TestSuccessLifecycle(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()
	v := createUploadedVideo(t, svc)
	worker := NewWorker(svc, &NoopProcessor{}, 10*time.Millisecond)
	if err := worker.ProcessOne(ctx); err != nil {
		t.Fatalf("ProcessOne failed: %v", err)
	}
	fetched, _ := svc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusReady {
		t.Fatalf("expected READY got %s", fetched.Status)
	}
	svc.DeleteVideo(ctx, v.ID)
}

func TestFailureLifecycle(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()
	v := createUploadedVideo(t, svc)
	worker := NewWorker(svc, &FailingProcessor{}, 10*time.Millisecond)
	worker.ProcessOne(ctx)
	fetched, _ := svc.GetVideo(ctx, v.ID)
	if fetched.Status != video.StatusFailed {
		t.Fatalf("expected FAILED got %s", fetched.Status)
	}
	if fetched.ProcessingError == nil || *fetched.ProcessingError == "" {
		t.Fatalf("expected processing_error set")
	}
	svc.DeleteVideo(ctx, v.ID)
}

func TestWorkerShutdown(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	worker := NewWorker(svc, &NoopProcessor{}, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("worker did not stop")
	}
}

func TestEmptyQueue(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	worker := NewWorker(svc, &NoopProcessor{}, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		worker.Start(ctx)
		close(done)
	}()
	<-done
	_ = bytes.NewReader
}

func TestLocalAndUploadBothClaimable(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()

	tmp, _ := os.CreateTemp("", "local*.mp4")
	tmp.Write([]byte("local"))
	tmp.Close()
	path := tmp.Name()
	defer os.Remove(path)
	srcType := video.SourceTypeLocal
	local, err := svc.CreateVideo(ctx, "local.mp4", nil, nil, nil, &srcType, &path)
	if err != nil {
		t.Fatalf("CreateVideo LOCAL failed: %v", err)
	}

	upload := createUploadedVideo(t, svc)

	c1, err := svc.ClaimNext(ctx)
	if err != nil || c1 == nil {
		t.Fatalf("first claim failed")
	}
	c2, err := svc.ClaimNext(ctx)
	if err != nil || c2 == nil {
		t.Fatalf("second claim failed")
	}
	if c1.ID == c2.ID {
		t.Fatalf("should claim different videos")
	}

	ids := map[string]bool{c1.ID.String(): true, c2.ID.String(): true}
	if !ids[local.ID.String()] || !ids[upload.ID.String()] {
		t.Fatalf("expected both LOCAL and UPLOAD claimed")
	}

	svc.DeleteVideo(ctx, local.ID)
	svc.DeleteVideo(ctx, upload.ID)
}

func TestClaimNextNoRows(t *testing.T) {
	svc, cleanup := newTestVideoService(t)
	defer cleanup()
	ctx := context.Background()
	PGPASSWORD := os.Getenv("DATABASE_URL")
	_ = PGPASSWORD
	v, err := svc.ClaimNext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil when no UPLOADED, got %v", v.ID)
	}
	svc.DeleteVideo(ctx, uuid.New())
}
