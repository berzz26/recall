package video

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/berzz26/recall/services/api/internal/storage"
)

type Service struct {
	repo    *Repository
	storage storage.Storage
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo, storage: nil}
}

func NewServiceWithStorage(repo *Repository, store storage.Storage) *Service {
	return &Service{repo: repo, storage: store}
}

func (s *Service) SetStorage(store storage.Storage) {
	s.storage = store
}

var allowedExtensions = map[string]bool{
	".mp4": true,
	".mkv": true,
	".avi": true,
	".mov": true,
}

func isAllowedExtension(ext string) bool {
	return allowedExtensions[strings.ToLower(ext)]
}

func mimeFromExtension(ext string) string {
	switch strings.ToLower(ext) {
	case ".mp4":
		return "video/mp4"
	case ".mkv":
		return "video/x-matroska"
	case ".avi":
		return "video/x-msvideo"
	case ".mov":
		return "video/quicktime"
	default:
		return "application/octet-stream"
	}
}

func (s *Service) CreateVideo(ctx context.Context, filename string, contentHash *string, mimeType *string, sizeBytes *int64, sourceType *SourceType, sourcePath *string) (*Video, error) {
	if filename == "" {
		return nil, errors.New("filename is required")
	}

	st := SourceTypeUpload
	if sourceType != nil {
		st = *sourceType
	}
	switch st {
	case SourceTypeLocal, SourceTypeUpload:
	default:
		return nil, fmt.Errorf("invalid source_type %s", st)
	}

	if st == SourceTypeLocal && (sourcePath == nil || *sourcePath == "") {
		return nil, errors.New("source_path is required for LOCAL source_type")
	}
	if st == SourceTypeUpload {
		sourcePath = nil
	}

	if st == SourceTypeLocal {
		ext := filepath.Ext(filename)
		if !isAllowedExtension(ext) {
			return nil, fmt.Errorf("unsupported file type %s", ext)
		}
	}

	ch := ""
	if contentHash != nil {
		ch = *contentHash
	}
	mt := ""
	if mimeType != nil {
		mt = *mimeType
		if mt == "" {
			mt = "application/octet-stream"
		}
	}
	sb := int64(0)
	if sizeBytes != nil {
		sb = *sizeBytes
		if sb < 0 {
			return nil, errors.New("size_bytes must be >= 0")
		}
	}

	v, err := s.repo.Create(ctx, filename, ch, mt, sb, st, sourcePath)
	if err != nil {
		return nil, fmt.Errorf("create video: %w", err)
	}
	return v, nil
}

func (s *Service) UploadVideo(ctx context.Context, filename string, reader io.Reader, mimeType string) (*Video, error) {
	if filename == "" {
		return nil, errors.New("filename is required")
	}
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp4"
	}
	if !isAllowedExtension(ext) {
		return nil, fmt.Errorf("unsupported file type %s", ext)
	}
	if reader == nil {
		return nil, errors.New("reader is required")
	}
	if s.storage == nil {
		return nil, errors.New("storage not configured")
	}

	if mimeType == "" {
		mimeType = mimeFromExtension(ext)
	}

	v, err := s.repo.Create(ctx, filename, "", mimeType, 0, SourceTypeUpload, nil)
	if err != nil {
		return nil, fmt.Errorf("create video: %w", err)
	}

	storageKey := fmt.Sprintf("videos/%s/original%s", v.ID.String(), strings.ToLower(ext))

	pr, pw := io.Pipe()
	hash := sha256.New()
	tee := io.TeeReader(reader, hash)
	var written int64
	var copyErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		n, err := io.Copy(pw, tee)
		written = n
		copyErr = err
		pw.CloseWithError(err)
	}()

	err = s.storage.Save(ctx, storageKey, pr)
	<-done
	if copyErr != nil && err == nil {
		err = copyErr
	}
	if err != nil {
		s.storage.Delete(ctx, storageKey)
		s.repo.UpdateStatus(ctx, v.ID, StatusFailed)
		return nil, fmt.Errorf("save file: %w", err)
	}

	hashStr := hex.EncodeToString(hash.Sum(nil))
	size := written

	updated, err := s.repo.UpdateUpload(ctx, v.ID, storageKey, hashStr, mimeType, size, StatusUploaded)
	if err != nil {
		s.storage.Delete(ctx, storageKey)
		s.repo.UpdateStatus(ctx, v.ID, StatusFailed)
		return nil, fmt.Errorf("update video: %w", err)
	}
	return updated, nil
}

func (s *Service) GetVideo(ctx context.Context, id uuid.UUID) (*Video, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid id")
	}

	v, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get video %s: %w", id.String(), err)
	}
	return v, nil
}

func (s *Service) ListVideos(ctx context.Context, limit, offset int) ([]Video, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	videos, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list videos: %w", err)
	}
	if videos == nil {
		videos = []Video{}
	}
	return videos, nil
}

func (s *Service) DeleteVideo(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.New("invalid id")
	}

	v, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get video %s: %w", id.String(), err)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete video %s: %w", id.String(), err)
	}

	if v.SourceType == SourceTypeUpload && v.StorageKey != nil && *v.StorageKey != "" && s.storage != nil {
		_ = s.storage.Delete(ctx, *v.StorageKey)
	}

	return nil
}

func (s *Service) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) (*Video, error) {
	if id == uuid.Nil {
		return nil, errors.New("invalid id")
	}
	switch status {
	case StatusUploading, StatusUploaded, StatusProcessing, StatusReady, StatusFailed:
	default:
		return nil, fmt.Errorf("invalid status %s", status)
	}

	v, err := s.repo.UpdateStatus(ctx, id, status)
	if err != nil {
		return nil, fmt.Errorf("update video %s status: %w", id.String(), err)
	}
	return v, nil
}

func (s *Service) IngestLocalDirectory(ctx context.Context, dir string) (int, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() {
		return 0, fmt.Errorf("not a directory")
	}

	var count int
	err = filepath.WalkDir(abs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if !isAllowedExtension(ext) {
			return nil
		}
		exists, err := s.repo.ExistsBySourcePath(ctx, path)
		if err != nil {
			return nil
		}
		if exists {
			return nil
		}
		filename := filepath.Base(path)
		var size int64
		if fi, err := d.Info(); err == nil {
			size = fi.Size()
		}
		mime := mimeFromExtension(ext)
		_, err = s.repo.Create(ctx, filename, "", mime, size, SourceTypeLocal, &path)
		if err != nil {
			return nil
		}
		count++
		return nil
	})
	if err != nil {
		return count, err
	}
	return count, nil
}
