package video_media

import (
	"context"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Upsert(ctx context.Context, m *MediaMetadata) (*MediaMetadata, error) {
	return s.repo.Upsert(ctx, m)
}

func (s *Service) GetByVideoID(ctx context.Context, videoID uuid.UUID) (*MediaMetadata, error) {
	return s.repo.GetByVideoID(ctx, videoID)
}

func (s *Service) DeleteByVideoID(ctx context.Context, videoID uuid.UUID) error {
	return s.repo.DeleteByVideoID(ctx, videoID)
}
