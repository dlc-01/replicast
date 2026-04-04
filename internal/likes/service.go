package likes

import (
	"context"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type likeRepo interface {
	Like(ctx context.Context, userID, postGlobalID string) error
	Unlike(ctx context.Context, userID, postGlobalID string) error
	Count(ctx context.Context, postGlobalID string) (int, error)
	IsLiked(ctx context.Context, userID, postGlobalID string) (bool, error)
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
}

type fedEnqueuer interface {
	EnqueueEvent(ctx context.Context, e port.OutboxEvent) error
}

type postGetter interface {
	GetByGlobalID(ctx context.Context, globalID string) (*port.Post, error)
}

type Service struct {
	repo    likeRepo
	fedRepo fedEnqueuer
	posts   postGetter
	log     logger.Logger
	cfg     *config.Config
}

func NewService(repo likeRepo, fedRepo fedEnqueuer, posts postGetter, log logger.Logger, cfg *config.Config) *Service {
	return &Service{repo: repo, fedRepo: fedRepo, posts: posts, log: log, cfg: cfg}
}

func (s *Service) Like(ctx context.Context, userGlobalID, postGlobalID string) error {
	userUUID, err := s.repo.GetUUIDByGlobalID(ctx, userGlobalID)
	if err != nil {
		return fmt.Errorf("likes.Like: %w", err)
	}
	if err := s.repo.Like(ctx, userUUID, postGlobalID); err != nil {
		return fmt.Errorf("likes.Like: %w", err)
	}

	// Уведомляем origin node если пост с другого узла
	post, _ := s.posts.GetByGlobalID(ctx, postGlobalID)
	if post != nil && post.OriginNode != s.cfg.NodeName {
		_ = s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: post.OriginNode,
			EventType:  "post.liked",
			Payload: map[string]any{
				"event_id":       ulid.Make().String(),
				"post_global_id": postGlobalID,
				"user_global_id": userGlobalID,
				"origin_node":    s.cfg.NodeName,
			},
		})
	}

	s.log.Info("post liked", "user", userGlobalID, "post", postGlobalID)
	return nil
}

func (s *Service) Unlike(ctx context.Context, userGlobalID, postGlobalID string) error {
	userUUID, err := s.repo.GetUUIDByGlobalID(ctx, userGlobalID)
	if err != nil {
		return fmt.Errorf("likes.Unlike: %w", err)
	}
	if err := s.repo.Unlike(ctx, userUUID, postGlobalID); err != nil {
		return fmt.Errorf("likes.Unlike: %w", err)
	}
	s.log.Info("post unliked", "user", userGlobalID, "post", postGlobalID)
	return nil
}

func (s *Service) GetCount(ctx context.Context, postGlobalID string) (int, error) {
	return s.repo.Count(ctx, postGlobalID)
}

func (s *Service) IsLiked(ctx context.Context, userGlobalID, postGlobalID string) (bool, error) {
	userUUID, err := s.repo.GetUUIDByGlobalID(ctx, userGlobalID)
	if err != nil {
		return false, nil
	}
	return s.repo.IsLiked(ctx, userUUID, postGlobalID)
}
