package local_source

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/video"
)

type Service struct {
	repo            *Repository
	videoService    *video.Service
	stability       time.Duration
	mu              sync.Mutex
	watchers        map[uuid.UUID]*watcher
}

type watcher struct {
	source  LocalSource
	watcher *fsnotify.Watcher
	done    chan struct{}
}

func NewService(repo *Repository, videoService *video.Service, stability time.Duration) *Service {
	if stability <= 0 {
		stability = 5 * time.Second
	}
	return &Service{
		repo:         repo,
		videoService: videoService,
		stability:    stability,
		watchers:     make(map[uuid.UUID]*watcher),
	}
}

func (s *Service) Create(ctx context.Context, name, path string) (*LocalSource, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("path must be absolute")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid path")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("path does not exist")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path must be a directory")
	}
	if _, err := os.Open(abs); err != nil {
		return nil, fmt.Errorf("path not readable")
	}

	existing, err := s.repo.GetByPath(ctx, abs)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("path already exists")
	}

	src, err := s.repo.Create(ctx, name, abs)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			return nil, fmt.Errorf("path already exists")
		}
		return nil, err
	}

	slog.Info("local source registered", "id", src.ID, "path", src.Path)

	count, err := s.videoService.IngestLocalDirectory(ctx, abs)
	if err != nil {
		slog.Error("initial scan failed", "path", abs, "error", err)
	} else {
		slog.Info("initial scan completed", "path", abs, "ingested", count)
	}

	if err := s.StartWatcher(ctx, *src); err != nil {
		slog.Error("failed to start watcher", "path", abs, "error", err)
	}

	return src, nil
}

func (s *Service) List(ctx context.Context) ([]LocalSource, error) {
	return s.repo.List(ctx)
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	src, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.StopWatcher(id); err != nil {
		slog.Error("failed to stop watcher", "id", id, "error", err)
	}
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	slog.Info("local source deleted", "id", id, "path", src.Path)
	return nil
}

func (s *Service) StartAllWatchers(ctx context.Context) error {
	sources, err := s.repo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	for _, src := range sources {
		if err := s.StartWatcher(ctx, src); err != nil {
			slog.Error("failed to start watcher", "path", src.Path, "error", err)
		}
	}
	return nil
}

func (s *Service) StopAllWatchers() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, w := range s.watchers {
		close(w.done)
		w.watcher.Close()
		delete(s.watchers, id)
		slog.Info("watcher stopped", "id", id)
	}
}

func (s *Service) StartWatcher(ctx context.Context, src LocalSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.watchers[src.ID]; ok {
		return nil
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := s.addRecursiveWatch(w, src.Path); err != nil {
		w.Close()
		return err
	}
	done := make(chan struct{})
	ws := &watcher{source: src, watcher: w, done: done}
	s.watchers[src.ID] = ws
	go s.watchLoop(ws)
	slog.Info("watcher started", "id", src.ID, "path", src.Path)
	return nil
}

func (s *Service) StopWatcher(id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.watchers[id]
	if !ok {
		return nil
	}
	close(w.done)
	w.watcher.Close()
	delete(s.watchers, id)
	slog.Info("watcher stopped", "id", id)
	return nil
}

func (s *Service) addRecursiveWatch(w *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if err := w.Add(path); err != nil {
				slog.Error("failed to watch dir", "path", path, "error", err)
			}
		}
		return nil
	})
}

func (s *Service) watchLoop(ws *watcher) {
	debounce := make(map[string]time.Time)
	mu := sync.Mutex{}
	for {
		select {
		case <-ws.done:
			return
		case event, ok := <-ws.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			path := event.Name
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.IsDir() {
				ws.watcher.Add(path)
				continue
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".mp4" && ext != ".mkv" && ext != ".avi" && ext != ".mov" {
				continue
			}
			mu.Lock()
			last, ok := debounce[path]
			if ok && time.Since(last) < s.stability {
				mu.Unlock()
				continue
			}
			debounce[path] = time.Now()
			mu.Unlock()

			go func(p string) {
				time.Sleep(s.stability)
				if err := s.handleStableFile(p); err != nil {
					if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "already exists") {
						slog.Error("ingestion error", "path", p, "error", err)
					} else {
						slog.Info("duplicate skipped", "path", p)
					}
				} else {
					slog.Info("video discovered", "path", p)
				}
			}(path)

		case err, ok := <-ws.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("watcher error", "path", ws.source.Path, "error", err)
		}
	}
}

func (s *Service) handleStableFile(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info1, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info1.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	size1 := info1.Size()
	mtime1 := info1.ModTime()
	time.Sleep(s.stability)
	info2, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if info2.Size() != size1 || !info2.ModTime().Equal(mtime1) {
		return fmt.Errorf("file unstable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exists, err := s.videoService.ExistsBySourcePath(ctx, abs)
	if err == nil && exists {
		return fmt.Errorf("already exists")
	}
	_, err = s.videoService.CreateLocalFromPath(ctx, abs)
	return err
}
