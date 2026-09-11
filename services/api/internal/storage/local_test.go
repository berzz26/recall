package storage

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorageSaveAndDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := NewLocalStorage(dir)
	if err != nil {
		t.Fatalf("NewLocalStorage failed: %v", err)
	}
	ctx := context.Background()
	key := "videos/test123/original.mp4"
	data := []byte("hello video")
	if err := s.Save(ctx, key, bytes.NewReader(data)); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	exists, _ := s.Exists(ctx, key)
	if !exists {
		t.Fatalf("Exists should be true")
	}
	rc, err := s.Open(ctx, key)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(rc)
	rc.Close()
	if buf.String() != string(data) {
		t.Fatalf("data mismatch")
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	exists, _ = s.Exists(ctx, key)
	if exists {
		t.Fatalf("Exists should be false after delete")
	}
}

func TestLocalStoragePathTraversal(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStorage(dir)
	ctx := context.Background()
	badKeys := []string{
		"../../etc/passwd",
		"/absolute/path.mp4",
		"videos/../etc/passwd",
		"..",
		"videos/../../secret",
	}
	for _, k := range badKeys {
		if err := s.Save(ctx, k, bytes.NewReader([]byte("x"))); err == nil {
			t.Fatalf("expected error for key %q", k)
		}
	}
}

func TestLocalStoragePreventEscape(t *testing.T) {
	dir := t.TempDir()
	s, _ := NewLocalStorage(dir)
	ctx := context.Background()
	key := "videos/abc/original.mp4"
	s.Save(ctx, key, bytes.NewReader([]byte("data")))
	if _, err := os.Stat(filepath.Join(dir, "etc")); err == nil {
		t.Fatalf("traversal created file outside root")
	}
}
