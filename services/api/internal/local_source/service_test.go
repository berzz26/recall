package local_source

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/storage"
	"github.com/berzz26/recall/services/api/internal/video"
)

func newTestServices(t *testing.T) (*Service, *video.Service, func()) {
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
	videoRepo := video.NewRepository(db.DB)
	videoSvc := video.NewServiceWithConfig(videoRepo, store, 1<<30)
	repo := NewRepository(db.DB)
	svc := NewService(repo, videoSvc, 1*time.Second)
	cleanup := func() {
		svc.StopAllWatchers()
		db.Close()
		os.RemoveAll(dir)
	}
	return svc, videoSvc, cleanup
}

func TestCreateLocalSource(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	src, err := svc.Create(ctx, "CCTV", dir)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if src.Path != dir {
		t.Fatalf("path mismatch")
	}
	svc.Delete(ctx, src.ID)
}

func TestCreateRejectNonexistent(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	_, err := svc.Create(ctx, "bad", "/tmp/not-exist-xyz-123")
	if err == nil {
		t.Fatalf("expected error for nonexistent")
	}
}

func TestCreateRejectFileInsteadOfDir(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	f, _ := os.CreateTemp("", "file*.mp4")
	f.Close()
	defer os.Remove(f.Name())
	_, err := svc.Create(ctx, "bad", f.Name())
	if err == nil {
		t.Fatalf("expected error for file not dir")
	}
}

func TestCreateRejectRelativePath(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	_, err := svc.Create(ctx, "bad", "relative/path")
	if err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestCreateRejectDuplicatePath(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	_, err := svc.Create(ctx, "a", dir)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}
	_, err = svc.Create(ctx, "b", dir)
	if err == nil {
		t.Fatalf("expected duplicate error")
	}
	list, _ := svc.List(ctx)
	for _, s := range list {
		if s.Path == dir {
			svc.Delete(ctx, s.ID)
		}
	}
}

func TestListAndDeleteSource(t *testing.T) {
	svc, _, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	src, _ := svc.Create(ctx, "test", dir)
	list, err := svc.List(ctx)
	if err != nil || len(list) == 0 {
		t.Fatalf("list failed")
	}
	if err := svc.Delete(ctx, src.ID); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("physical dir should not be deleted")
	}
}

func TestInitialScan(t *testing.T) {
	svc, videoSvc, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.mp4"), []byte("a content"), 0644)
	os.WriteFile(filepath.Join(dir, "b.mp4"), []byte("b content"), 0644)
	os.WriteFile(filepath.Join(dir, "c.mkv"), []byte("c content"), 0644)
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignore"), 0644)
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(sub, "d.mov"), []byte("d"), 0644)

	src, err := svc.Create(ctx, "scan", dir)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer svc.Delete(ctx, src.ID)

	list, _ := videoSvc.ListVideos(ctx, 100, 0)
	count := 0
	for _, v := range list {
		if v.SourcePath != nil && (*v.SourcePath == filepath.Join(dir, "a.mp4") || *v.SourcePath == filepath.Join(dir, "b.mp4") || *v.SourcePath == filepath.Join(dir, "c.mkv") || *v.SourcePath == filepath.Join(sub, "d.mov")) {
			if v.ContentHash == "" || v.SizeBytes == 0 || v.SourceMtime == nil {
				t.Fatalf("metadata not populated")
			}
			count++
		}
	}
	if count != 4 {
		t.Fatalf("expected 4 got %d", count)
	}

	svc2, _, _ := newTestServices(t)
	_ = svc2
}

func TestIdempotency(t *testing.T) {
	svc, videoSvc, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.mp4"), []byte("a"), 0644)
	src, _ := svc.Create(ctx, "idem", dir)
	defer svc.Delete(ctx, src.ID)

	count1, _ := videoSvc.IngestLocalDirectory(ctx, dir)
	if count1 != 0 {
		t.Fatalf("expected 0 on second ingest via service, got %d", count1)
	}
}

func TestSameHashDifferentPaths(t *testing.T) {
	svc, videoSvc, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	content := []byte("identical")
	p1 := filepath.Join(dir, "a.mp4")
	p2 := filepath.Join(dir, "b.mp4")
	os.WriteFile(p1, content, 0644)
	os.WriteFile(p2, content, 0644)

	svc.Create(ctx, "dup", dir)
	defer func() {
		list, _ := svc.List(ctx)
		for _, s := range list {
			if s.Path == dir {
				svc.Delete(ctx, s.ID)
			}
		}
	}()

	list, _ := videoSvc.ListVideos(ctx, 100, 0)
	var v1, v2 *video.Video
	for i := range list {
		if list[i].SourcePath != nil && *list[i].SourcePath == p1 {
			v1 = &list[i]
		}
		if list[i].SourcePath != nil && *list[i].SourcePath == p2 {
			v2 = &list[i]
		}
	}
	if v1 == nil || v2 == nil {
		t.Fatalf("both videos not found")
	}
	if v1.ContentHash != v2.ContentHash {
		t.Fatalf("same bytes should have same hash")
	}
	if v1.ID == v2.ID {
		t.Fatalf("should be separate records")
	}
}

func TestWatcherCreatesFile(t *testing.T) {
	svc, videoSvc, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	src, err := svc.Create(ctx, "watch", dir)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer svc.Delete(ctx, src.ID)

	time.Sleep(500 * time.Millisecond)
	newFile := filepath.Join(dir, "new.mp4")
	os.WriteFile(newFile, []byte("new content"), 0644)

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		list, _ := videoSvc.ListVideos(ctx, 100, 0)
		for _, v := range list {
			if v.SourcePath != nil && *v.SourcePath == newFile {
				if v.Status != video.StatusUploaded {
					t.Fatalf("expected UPLOADED")
				}
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("watcher did not create video for new file")
}

func TestDeleteSourceDoesNotDeleteVideosOrFiles(t *testing.T) {
	svc, videoSvc, cleanup := newTestServices(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.mp4"), []byte("x"), 0644)
	src, _ := svc.Create(ctx, "todel", dir)
	list, _ := videoSvc.ListVideos(ctx, 100, 0)
	var vid *video.Video
	for i := range list {
		if list[i].SourcePath != nil && *list[i].SourcePath == filepath.Join(dir, "x.mp4") {
			vid = &list[i]
			break
		}
	}
	if vid == nil {
		t.Fatalf("video not found")
	}
	if err := svc.Delete(ctx, src.ID); err != nil {
		t.Fatalf("delete source failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "x.mp4")); err != nil {
		t.Fatalf("file should not be deleted")
	}
	fetched, err := videoSvc.GetVideo(ctx, vid.ID)
	if err != nil {
		t.Fatalf("video row should remain")
	}
	if fetched.ID != vid.ID {
		t.Fatalf("mismatch")
	}
	videoSvc.DeleteVideo(ctx, vid.ID)
}
