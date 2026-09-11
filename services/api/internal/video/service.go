package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
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

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete video %s: %w", id.String(), err)
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
