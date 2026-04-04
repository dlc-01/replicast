package comments

import (
	"context"
	"fmt"

	"github.com/oklog/ulid/v2"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type commentRepo interface {
	Create(ctx context.Context, c port.Comment) error
	GetByPost(ctx context.Context, postGlobalID string, limit int) ([]port.Comment, error)
	GetByGlobalID(ctx context.Context, globalID string) (*port.Comment, error)
	Delete(ctx context.Context, globalID string) error
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
}

type fedEnqueuer interface {
	EnqueueEvent(ctx context.Context, e port.OutboxEvent) error
}

type postGetter interface {
	GetByGlobalID(ctx context.Context, globalID string) (*port.Post, error)
}

type Service struct {
	repo    commentRepo
	fedRepo fedEnqueuer
	posts   postGetter
	log     logger.Logger
	cfg     *config.Config
}

func NewService(repo commentRepo, fedRepo fedEnqueuer, posts postGetter, log logger.Logger, cfg *config.Config) *Service {
	return &Service{repo: repo, fedRepo: fedRepo, posts: posts, log: log, cfg: cfg}
}

func (s *Service) Create(ctx context.Context, authorGlobalID, postGlobalID, content string) (*port.Comment, error) {
	authorUUID, err := s.repo.GetUUIDByGlobalID(ctx, authorGlobalID)
	if err != nil {
		return nil, fmt.Errorf("comments.Create resolve author: %w", err)
	}

	globalID := fmt.Sprintf("comment:%s:%s", authorGlobalID, ulid.Make().String())

	c := port.Comment{
		GlobalID:     globalID,
		PostGlobalID: postGlobalID,
		AuthorID:     authorUUID,
		OriginNode:   s.cfg.NodeName,
		Content:      content,
	}
	if err := s.repo.Create(ctx, c); err != nil {
		return nil, fmt.Errorf("comments.Create: %w", err)
	}

	// Уведомляем origin node поста
	post, _ := s.posts.GetByGlobalID(ctx, postGlobalID)
	if post != nil && post.OriginNode != s.cfg.NodeName {
		_ = s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: post.OriginNode,
			EventType:  "comment.created",
			Payload: map[string]any{
				"global_id":      globalID,
				"post_global_id": postGlobalID,
				"author_id":      authorGlobalID,
				"content":        content,
				"origin_node":    s.cfg.NodeName,
			},
		})
	}

	s.log.Info("comment created", "global_id", globalID, "post", postGlobalID)
	return s.repo.GetByGlobalID(ctx, globalID)
}

func (s *Service) GetByPost(ctx context.Context, postGlobalID string, limit int) ([]port.Comment, error) {
	return s.repo.GetByPost(ctx, postGlobalID, limit)
}

func (s *Service) Delete(ctx context.Context, globalID, authorGlobalID string) error {
	c, err := s.repo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return err
	}

	authorUUID, err := s.repo.GetUUIDByGlobalID(ctx, authorGlobalID)
	if err != nil {
		return fmt.Errorf("comments.Delete resolve author: %w", err)
	}
	if c.AuthorID != authorUUID {
		return apperr.ErrPostForbidden
	}

	return s.repo.Delete(ctx, globalID)
}
