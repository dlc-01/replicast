package feed

import (
	"context"
	"fmt"

	"github.com/dlc-01/replicast/internal/port"
)

const defaultFeedLimit = 50

// feedRepository — расширенный интерфейс с резолвом UUID.
type feedRepository interface {
	port.FeedRepository
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
}

type Service struct {
	repo feedRepository
}

func NewService(repo feedRepository) *Service {
	return &Service{repo: repo}
}

// GetFeed принимает ownerGlobalID (alice@node-a), резолвит UUID и возвращает ленту.
func (s *Service) GetFeed(ctx context.Context, ownerGlobalID string, limit int) ([]port.FeedPost, error) {
	if limit <= 0 || limit > 200 {
		limit = defaultFeedLimit
	}

	// Резолвим global_id → UUID для правильного JOIN в БД
	ownerUUID, err := s.repo.GetUUIDByGlobalID(ctx, ownerGlobalID)
	if err != nil {
		return nil, fmt.Errorf("feed.GetFeed resolve owner: %w", err)
	}

	items, err := s.repo.GetFeed(ctx, ownerUUID, limit)
	if err != nil {
		return nil, fmt.Errorf("feed.GetFeed: %w", err)
	}
	return items, nil
}
