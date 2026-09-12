package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type LocalStorage struct {
	root string
}

func NewLocalStorage(root string) (*LocalStorage, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return nil, err
	}
	return &LocalStorage{root: abs}, nil
}

func (s *LocalStorage) resolve(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("empty key")
	}
	if strings.Contains(key, "..") {
		return "", fmt.Errorf("path traversal not allowed")
	}
	if filepath.IsAbs(key) {
		return "", fmt.Errorf("absolute path not allowed")
	}
	clean := filepath.Clean(key)
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute path not allowed")
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("path traversal not allowed")
	}
	if strings.Contains(clean, "..") {
		parts := strings.Split(clean, string(filepath.Separator))
		for _, p := range parts {
			if p == ".." {
				return "", fmt.Errorf("path traversal not allowed")
			}
		}
	}
	full := filepath.Join(s.root, clean)
	absRoot := s.root
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absFull, absRoot+string(filepath.Separator)) && absFull != absRoot {
		return "", fmt.Errorf("path traversal not allowed")
	}
	return absFull, nil
}

func (s *LocalStorage) Save(ctx context.Context, key string, r io.Reader) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		os.Remove(path)
		return err
	}
	return nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	path, err := s.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	// Prune empty parent directories up to storage root
	dir := filepath.Dir(path)
	for {
		if dir == s.root || dir == "." || dir == "/" {
			break
		}
		// Ensure dir is inside root
		if !strings.HasPrefix(dir, s.root) {
			break
		}
		// Stop if dir is the root itself
		if dir == s.root {
			break
		}
		err := os.Remove(dir)
		if err != nil {
			// Directory not empty or other error — stop pruning
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

func (s *LocalStorage) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	path, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	path, err := s.resolve(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) Root() string {
	return s.root
}
