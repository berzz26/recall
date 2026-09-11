package video

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/berzz26/recall/pkg/database"
	"github.com/berzz26/recall/services/api/internal/storage"
)

func newL14Service(t *testing.T) (*Service, func()) {
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
	repo := NewRepository(db.DB)
	svc := NewServiceWithConfig(repo, store, 1<<30)
	cleanup := func() {
		db.Close()
		os.RemoveAll(dir)
	}
	return svc, cleanup
}

func TestLocalValidGetsHashSizeMime(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()

	tmp, _ := os.CreateTemp("", "local-valid*.mp4")
	content := []byte("local file content for hash")
	tmp.Write(content)
	tmp.Close()
	path := tmp.Name()
	defer os.Remove(path)

	hash := sha256.Sum256(content)
	expectedHash := hex.EncodeToString(hash[:])

	v, err := svc.CreateVideo(ctx, "valid.mp4", nil, nil, nil, sourcePtr(SourceTypeLocal), &path)
	if err != nil {
		t.Fatalf("CreateVideo LOCAL failed: %v", err)
	}
	if v.ContentHash != expectedHash {
		t.Fatalf("hash mismatch expected %s got %s", expectedHash, v.ContentHash)
	}
	if v.SizeBytes != int64(len(content)) {
		t.Fatalf("size mismatch")
	}
	if v.MimeType != "video/mp4" {
		t.Fatalf("mime mismatch expected video/mp4 got %s", v.MimeType)
	}
	if v.SourcePath == nil || *v.SourcePath != path {
		t.Fatalf("source_path not preserved")
	}
	if v.StorageKey != nil {
		t.Fatalf("storage_key should be null")
	}
	if v.Status != StatusUploaded {
		t.Fatalf("expected UPLOADED")
	}
	if v.SourceMtime == nil {
		t.Fatalf("source_mtime should be set")
	}
	svc.DeleteVideo(ctx, v.ID)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("source file should not be deleted")
	}
}

func TestLocalNonexistentPath(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	path := "/tmp/does-not-exist-12345.mp4"
	_, err := svc.CreateVideo(ctx, "bad.mp4", nil, nil, nil, sourcePtr(SourceTypeLocal), &path)
	if err == nil {
		t.Fatalf("expected error for nonexistent path")
	}
	if !contains(err.Error(), "does not exist") {
		t.Fatalf("expected not exist error got %v", err)
	}
}

func TestLocalDirectoryInsteadOfFile(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	_, err := svc.CreateVideo(ctx, "dir.mp4", nil, nil, nil, sourcePtr(SourceTypeLocal), &dir)
	if err == nil {
		t.Fatalf("expected error for directory")
	}
}

func TestLocalUnsupportedExtension(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	tmp, _ := os.CreateTemp("", "bad*.txt")
	tmp.Write([]byte("x"))
	tmp.Close()
	path := tmp.Name()
	defer os.Remove(path)
	_, err := svc.CreateVideo(ctx, "bad.txt", nil, nil, nil, sourcePtr(SourceTypeLocal), &path)
	if err == nil {
		t.Fatalf("expected error for unsupported extension")
	}
}

func TestUploadEmptyRejected(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	_, err := svc.UploadVideo(ctx, "empty.mp4", bytes.NewReader([]byte{}), "video/mp4")
	if err == nil {
		t.Fatalf("expected error for empty file")
	}
}

func TestUploadMimeNormalized(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	data := []byte("mime test")
	v, err := svc.UploadVideo(ctx, "test.mkv", bytes.NewReader(data), "application/octet-stream")
	if err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if v.MimeType != "video/x-matroska" {
		t.Fatalf("expected normalized mime video/x-matroska got %s", v.MimeType)
	}
	svc.DeleteVideo(ctx, v.ID)
}

func TestUploadMaxSizeEnforced(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	svc.SetMaxUploadSize(10)
	ctx := context.Background()
	data := bytes.Repeat([]byte("a"), 20)
	_, err := svc.UploadVideo(ctx, "big.mp4", bytes.NewReader(data), "video/mp4")
	if err == nil {
		t.Fatalf("expected error for too large")
	}
	if !contains(err.Error(), "too large") {
		t.Fatalf("expected too large error got %v", err)
	}
}

func TestDeduplicationSameHashDifferentPaths(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()

	dir := t.TempDir()
	content := []byte("identical content")
	p1 := filepath.Join(dir, "a.mp4")
	p2 := filepath.Join(dir, "b.mp4")
	os.WriteFile(p1, content, 0644)
	os.WriteFile(p2, content, 0644)

	v1, err := svc.CreateVideo(ctx, "a.mp4", nil, nil, nil, sourcePtr(SourceTypeLocal), &p1)
	if err != nil {
		t.Fatalf("Create a failed: %v", err)
	}
	v2, err := svc.CreateVideo(ctx, "b.mp4", nil, nil, nil, sourcePtr(SourceTypeLocal), &p2)
	if err != nil {
		t.Fatalf("Create b failed: %v", err)
	}
	if v1.ContentHash != v2.ContentHash {
		t.Fatalf("expected same hash for identical bytes")
	}
	if v1.ID == v2.ID {
		t.Fatalf("should be separate records")
	}
	list, err := svc.GetByContentHash(ctx, v1.ContentHash)
	if err != nil {
		t.Fatalf("GetByContentHash failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 with same hash got %d", len(list))
	}
	svc.DeleteVideo(ctx, v1.ID)
	svc.DeleteVideo(ctx, v2.ID)
}

func TestDuplicateContentNotRejected(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	data := []byte("dup content")
	v1, _ := svc.UploadVideo(ctx, "dup1.mp4", bytes.NewReader(data), "video/mp4")
	v2, err := svc.UploadVideo(ctx, "dup2.mp4", bytes.NewReader(data), "video/mp4")
	if err != nil {
		t.Fatalf("second upload with same hash should not be rejected: %v", err)
	}
	if v1.ContentHash != v2.ContentHash {
		t.Fatalf("hash should be same")
	}
	svc.DeleteVideo(ctx, v1.ID)
	svc.DeleteVideo(ctx, v2.ID)
}

func TestScannerPopulatesMetadata(t *testing.T) {
	svc, cleanup := newL14Service(t)
	defer cleanup()
	ctx := context.Background()
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "x.mp4"), []byte("x content"), 0644)
	os.WriteFile(filepath.Join(dir, "y.txt"), []byte("ignore"), 0644)
	count, err := svc.IngestLocalDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 got %d", count)
	}
	list, _ := svc.ListVideos(ctx, 10, 0)
	found := false
	for _, v := range list {
		if v.SourcePath != nil && *v.SourcePath == filepath.Join(dir, "x.mp4") {
			if v.ContentHash == "" || v.SizeBytes == 0 || v.MimeType != "video/mp4" || v.SourceMtime == nil {
				t.Fatalf("metadata not populated")
			}
			found = true
			svc.DeleteVideo(ctx, v.ID)
		}
	}
	if !found {
		t.Fatalf("ingested video not found")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i <= len(s)-len(substr); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
