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

func (s *Service) CreateVideo(ctx context.Context, filename, contentHash, mimeType string, sizeBytes int64) (*Video, error) {
	if filename == "" {
		return nil, errors.New("filename is required")
	}
	if contentHash == "" {
		return nil, errors.New("content_hash is required")
	}
	if mimeType == "" {
		return nil, errors.New("mime_type is required")
	}
	if sizeBytes <= 0 {
		return nil, errors.New("size_bytes must be greater than 0")
	}

	v, err := s.repo.Create(ctx, filename, contentHash, mimeType, sizeBytes)
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
