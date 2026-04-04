package posts

import (
	"context"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type userResolver interface {
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
}

type Service struct {
	postRepo port.PostRepository
	feedRepo port.FeedRepository
	fedRepo  port.FederationRepository
	userRepo userResolver
	log      logger.Logger
	cfg      *config.Config
}

func NewService(
	postRepo port.PostRepository,
	feedRepo port.FeedRepository,
	fedRepo port.FederationRepository,
	userRepo userResolver,
	log logger.Logger,
	cfg *config.Config,
) *Service {
	return &Service{
		postRepo: postRepo,
		feedRepo: feedRepo,
		fedRepo:  fedRepo,
		userRepo: userRepo,
		log:      log,
		cfg:      cfg,
	}
}

func (s *Service) Create(ctx context.Context, authorGlobalID, content string) (*port.Post, error) {
	authorUUID, err := s.userRepo.GetUUIDByGlobalID(ctx, authorGlobalID)
	if err != nil {
		return nil, fmt.Errorf("posts.Create resolve author: %w", err)
	}

	globalID := fmt.Sprintf("post:%s:%s", authorGlobalID, ulid.Make().String())

	p := port.Post{
		GlobalID:   globalID,
		AuthorID:   authorUUID,
		OriginNode: s.cfg.NodeName,
		Content:    content,
		Visibility: "public",
	}
	if err := s.postRepo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("posts.Create: %w", err)
	}

	if err := s.feedRepo.AddItem(ctx, port.FeedItem{
		OwnerUserID:  authorUUID,
		PostGlobalID: globalID,
		SourceNode:   s.cfg.NodeName,
	}); err != nil {
		s.log.Warn("posts.Create: add to author feed", "err", err)
	}

	followerIDs, err := s.feedRepo.GetFollowerUserIDs(ctx, authorGlobalID)
	if err != nil {
		s.log.Warn("posts.Create: get follower ids", "err", err)
	}
	for _, fid := range followerIDs {
		if err := s.feedRepo.AddItem(ctx, port.FeedItem{
			OwnerUserID:  fid,
			PostGlobalID: globalID,
			SourceNode:   s.cfg.NodeName,
		}); err != nil {
			s.log.Warn("posts.Create: add to follower feed", "follower", fid, "err", err)
		}
	}

	remoteNodes, err := s.postRepo.GetFollowerNodes(ctx, authorUUID)
	if err != nil {
		s.log.Warn("posts.Create: get follower nodes", "err", err)
	}
	for _, node := range remoteNodes {
		if err := s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: node,
			EventType:  "post.created",
			Payload: map[string]any{
				"global_id":   globalID,
				"author_id":   authorGlobalID,
				"content":     content,
				"origin_node": s.cfg.NodeName,
				"created_at":  time.Now().UTC().Format(time.RFC3339),
			},
		}); err != nil {
			s.log.Warn("posts.Create: enqueue event", "node", node, "err", err)
		}
	}

	s.log.Info("post created", "global_id", globalID, "author", authorGlobalID)
	return s.postRepo.GetByGlobalID(ctx, globalID)
}

func (s *Service) Get(ctx context.Context, globalID string) (*port.Post, error) {
	p, err := s.postRepo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("posts.Get: %w", err)
	}
	if p == nil {
		return nil, apperr.ErrPostNotFound
	}
	return p, nil
}

func (s *Service) Update(ctx context.Context, globalID, authorGlobalID, content string) (*port.Post, error) {
	p, err := s.postRepo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("posts.Update: %w", err)
	}
	if p == nil {
		return nil, apperr.ErrPostNotFound
	}

	authorUUID, err := s.userRepo.GetUUIDByGlobalID(ctx, authorGlobalID)
	if err != nil {
		return nil, fmt.Errorf("posts.Update resolve author: %w", err)
	}
	if p.AuthorID != authorUUID {
		return nil, apperr.ErrPostForbidden
	}

	updated, err := s.postRepo.Update(ctx, globalID, content)
	if err != nil {
		return nil, fmt.Errorf("posts.Update: %w", err)
	}

	// Рассылаем post.updated на все узлы где есть подписчики
	remoteNodes, err := s.postRepo.GetFollowerNodes(ctx, authorUUID)
	if err != nil {
		s.log.Warn("posts.Update: get follower nodes", "err", err)
	}
	for _, node := range remoteNodes {
		if err := s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: node,
			EventType:  "post.updated",
			Payload: map[string]any{
				"global_id": globalID,
				"content":   content,
				"version":   updated.Version,
			},
		}); err != nil {
			s.log.Warn("posts.Update: enqueue event", "node", node, "err", err)
		}
	}

	s.log.Info("post updated", "global_id", globalID)
	return updated, nil
}

func (s *Service) Delete(ctx context.Context, globalID, authorGlobalID string) error {
	p, err := s.postRepo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return fmt.Errorf("posts.Delete: %w", err)
	}
	if p == nil {
		return apperr.ErrPostNotFound
	}

	authorUUID, err := s.userRepo.GetUUIDByGlobalID(ctx, authorGlobalID)
	if err != nil {
		return fmt.Errorf("posts.Delete resolve author: %w", err)
	}
	if p.AuthorID != authorUUID {
		return apperr.ErrPostForbidden
	}

	if _, err := s.postRepo.Delete(ctx, globalID); err != nil {
		return fmt.Errorf("posts.Delete: %w", err)
	}

	// Рассылаем post.deleted на все узлы где есть подписчики
	remoteNodes, err := s.postRepo.GetFollowerNodes(ctx, authorUUID)
	if err != nil {
		s.log.Warn("posts.Delete: get follower nodes", "err", err)
	}
	for _, node := range remoteNodes {
		if err := s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: node,
			EventType:  "post.deleted",
			Payload: map[string]any{
				"global_id": globalID,
			},
		}); err != nil {
			s.log.Warn("posts.Delete: enqueue event", "node", node, "err", err)
		}
	}

	s.log.Info("post deleted", "global_id", globalID)
	return nil
}
