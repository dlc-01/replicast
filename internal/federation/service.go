package federation

import (
	"context"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
)

// Service — заглушка для Фазы 1.
// Фаза 2: приём/отправка событий, handshake, remote users.
type Service struct {
	fedRepo  port.FederationRepository
	postRepo port.PostRepository
	userRepo port.UserRepository
	feedRepo port.FeedRepository
	cfg      *config.Config
}

func NewService(
	fedRepo port.FederationRepository,
	postRepo port.PostRepository,
	userRepo port.UserRepository,
	feedRepo port.FeedRepository,
	cfg *config.Config,
) *Service {
	return &Service{
		fedRepo:  fedRepo,
		postRepo: postRepo,
		userRepo: userRepo,
		feedRepo: feedRepo,
		cfg:      cfg,
	}
}

// ReceiveEvent — TODO Фаза 2: обработка входящих событий от других узлов.
func (s *Service) ReceiveEvent(ctx context.Context, sourceNode, eventType string, payload map[string]any) error {
	return nil
}

// Handshake — TODO Фаза 2: знакомство с новым узлом.
func (s *Service) Handshake(ctx context.Context, nodeName, baseURL, secret string) error {
	return nil
}

// NodeInfo возвращает метаданные этого узла — используется в WellKnown и Handshake.
func (s *Service) NodeInfo() map[string]string {
	return map[string]string{
		"node":     s.cfg.NodeName,
		"base_url": s.cfg.BaseURL,
		"version":  "1",
	}
}
