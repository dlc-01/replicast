package follows

import (
	"context"
	"fmt"
	"strings"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// Локальные интерфейсы — минимальные контракты для зависимостей.
// Каждый содержит только то что реально нужно этому сервису.

type followRepository interface {
	Create(ctx context.Context, f port.Follow) error
	Delete(ctx context.Context, followerUserID, targetGlobalID string) error
	Exists(ctx context.Context, followerUserID, targetGlobalID string) (bool, error)
}

type userLookup interface {
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
}

type fedEnqueuer interface {
	EnqueueEvent(ctx context.Context, e port.OutboxEvent) error
}

type Service struct {
	followRepo followRepository
	userRepo   userLookup
	fedRepo    fedEnqueuer
	log        logger.Logger
	cfg        *config.Config
}

func NewService(
	followRepo followRepository,
	userRepo userLookup,
	fedRepo fedEnqueuer,
	log logger.Logger,
	cfg *config.Config,
) *Service {
	return &Service{
		followRepo: followRepo,
		userRepo:   userRepo,
		fedRepo:    fedRepo,
		log:        log,
		cfg:        cfg,
	}
}

func (s *Service) Follow(ctx context.Context, followerGlobalID, targetGlobalID string) error {
	if followerGlobalID == targetGlobalID {
		return apperr.BadRequest("self_follow", "cannot follow yourself")
	}

	followerUUID, err := s.userRepo.GetUUIDByGlobalID(ctx, followerGlobalID)
	if err != nil {
		return fmt.Errorf("follows.Follow resolve follower: %w", err)
	}

	exists, err := s.followRepo.Exists(ctx, followerUUID, targetGlobalID)
	if err != nil {
		return fmt.Errorf("follows.Follow check exists: %w", err)
	}
	if exists {
		return apperr.Conflict("already_following", "already following this user")
	}

	targetNode := nodeFromGlobalID(targetGlobalID)

	if err := s.followRepo.Create(ctx, port.Follow{
		FollowerUserID:     followerUUID,
		TargetGlobalUserID: targetGlobalID,
		TargetNode:         targetNode,
	}); err != nil {
		return fmt.Errorf("follows.Follow create: %w", err)
	}

	// Удалённая подписка — уведомляем целевой узел через outbox
	if targetNode != s.cfg.NodeName {
		_ = s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: targetNode,
			EventType:  "user.followed",
			Payload: map[string]any{
				"follower_global_id": followerGlobalID,
				"target_global_id":   targetGlobalID,
				"follower_node":      s.cfg.NodeName,
			},
		})
	}

	s.log.Info("follow created",
		"follower", followerGlobalID,
		"target", targetGlobalID,
		"remote", targetNode != s.cfg.NodeName,
	)
	return nil
}

func (s *Service) Unfollow(ctx context.Context, followerGlobalID, targetGlobalID string) error {
	followerUUID, err := s.userRepo.GetUUIDByGlobalID(ctx, followerGlobalID)
	if err != nil {
		return fmt.Errorf("follows.Unfollow resolve follower: %w", err)
	}

	exists, err := s.followRepo.Exists(ctx, followerUUID, targetGlobalID)
	if err != nil {
		return fmt.Errorf("follows.Unfollow check exists: %w", err)
	}
	if !exists {
		return apperr.NotFound("not_following", "not following this user")
	}

	if err := s.followRepo.Delete(ctx, followerUUID, targetGlobalID); err != nil {
		return fmt.Errorf("follows.Unfollow delete: %w", err)
	}

	s.log.Info("follow removed",
		"follower", followerGlobalID,
		"target", targetGlobalID,
	)
	return nil
}

// nodeFromGlobalID извлекает имя узла из "alice@node-a" → "node-a".
func nodeFromGlobalID(globalID string) string {
	if i := strings.LastIndex(globalID, "@"); i != -1 {
		return globalID[i+1:]
	}
	return ""
}
