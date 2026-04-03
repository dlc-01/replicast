package federation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// followWriter — минимальный интерфейс для записи подписки.
type followWriter interface {
	Create(ctx context.Context, f port.Follow) error
}

type Service struct {
	fedRepo    port.FederationRepository
	postRepo   port.PostRepository
	userRepo   port.UserRepository
	feedRepo   port.FeedRepository
	followRepo followWriter
	log        logger.Logger
	cfg        *config.Config
}

func NewService(
	fedRepo port.FederationRepository,
	postRepo port.PostRepository,
	userRepo port.UserRepository,
	feedRepo port.FeedRepository,
	followRepo followWriter,
	log logger.Logger,
	cfg *config.Config,
) *Service {
	return &Service{
		fedRepo:    fedRepo,
		postRepo:   postRepo,
		userRepo:   userRepo,
		feedRepo:   feedRepo,
		followRepo: followRepo,
		log:        log,
		cfg:        cfg,
	}
}

func (s *Service) NodeInfo() map[string]string {
	return map[string]string{
		"node":       s.cfg.NodeName,
		"base_url":   s.cfg.InternalBaseURL,
		"public_url": s.cfg.BaseURL,
		"version":    "1",
	}
}

func (s *Service) Handshake(ctx context.Context, nodeName, baseURL, secret string) error {
	if err := s.fedRepo.UpsertNode(ctx, port.Node{
		Name:         nodeName,
		BaseURL:      baseURL,
		SharedSecret: secret,
		Status:       "active",
	}); err != nil {
		return fmt.Errorf("federation.Handshake: %w", err)
	}
	s.log.Info("node registered via handshake", "node", nodeName, "base_url", baseURL)
	return nil
}

func (s *Service) ReceiveEvent(ctx context.Context, eventID, sourceNode, eventType string, payload json.RawMessage) error {
	processed, err := s.fedRepo.IsProcessed(ctx, eventID)
	if err != nil {
		return fmt.Errorf("federation.ReceiveEvent check processed: %w", err)
	}
	if processed {
		s.log.Info("event already processed, skipping", "event_id", eventID)
		return nil
	}

	switch eventType {
	case "user.followed":
		if err := s.handleUserFollowed(ctx, payload, sourceNode); err != nil {
			return err
		}
	case "post.created":
		if err := s.handlePostCreated(ctx, payload); err != nil {
			return err
		}
	case "post.updated":
		if err := s.handlePostUpdated(ctx, payload); err != nil {
			return err
		}
	case "post.deleted":
		if err := s.handlePostDeleted(ctx, payload); err != nil {
			return err
		}
	default:
		s.log.Warn("unknown event type, ignoring", "type", eventType, "event_id", eventID)
	}

	if err := s.fedRepo.MarkProcessed(ctx, eventID, sourceNode); err != nil {
		s.log.Warn("failed to mark event processed", "event_id", eventID, "err", err)
	}

	s.log.Info("event processed", "event_id", eventID, "type", eventType, "source", sourceNode)
	return nil
}

func (s *Service) handleUserFollowed(ctx context.Context, payload json.RawMessage, sourceNode string) error {
	var p struct {
		FollowerGlobalID string `json:"follower_global_id"`
		TargetGlobalID   string `json:"target_global_id"`
		FollowerNode     string `json:"follower_node"`
		FollowerBaseURL  string `json:"follower_base_url"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("handleUserFollowed decode: %w", err)
	}

	// Сохраняем узел источника
	if p.FollowerBaseURL != "" {
		_ = s.fedRepo.UpsertNode(ctx, port.Node{
			Name:         p.FollowerNode,
			BaseURL:      p.FollowerBaseURL,
			SharedSecret: s.cfg.SharedSecret,
			Status:       "active",
		})
	}

	// Создаём или обновляем заглушку remote follower
	if err := s.userRepo.UpsertRemote(ctx, port.User{
		GlobalID: p.FollowerGlobalID,
		HomeNode: p.FollowerNode,
		IsLocal:  false,
	}); err != nil {
		s.log.Warn("handleUserFollowed: upsert remote user", "err", err)
	}

	followerUser, err := s.userRepo.GetByGlobalID(ctx, p.FollowerGlobalID)
	if err != nil {
		s.log.Warn("handleUserFollowed: get follower after upsert", "follower", p.FollowerGlobalID, "err", err)
	}

	// Проверяем что цель существует локально
	targetUser, err := s.userRepo.GetByGlobalID(ctx, p.TargetGlobalID)
	if err != nil {
		return fmt.Errorf("handleUserFollowed target not found %s: %w", p.TargetGlobalID, err)
	}

	// Записываем подписку в follows — это ключевой шаг для GetFollowerNodes
	if followerUser != nil && targetUser != nil {
		s.log.Info("handleUserFollowed: creating follow record",
			"follower_id", followerUser.ID,
			"target", p.TargetGlobalID,
		)
		if err := s.followRepo.Create(ctx, port.Follow{
			FollowerUserID:     followerUser.ID,
			TargetGlobalUserID: p.TargetGlobalID,
			TargetNode:         p.FollowerNode,
		}); err != nil {
			s.log.Warn("handleUserFollowed: create follow record",
				"follower", p.FollowerGlobalID,
				"target", p.TargetGlobalID,
				"err", err,
			)
		}
	} else {
		s.log.Warn("handleUserFollowed: skipping follow creation",
			"follower_user_nil", followerUser == nil,
			"target_user_nil", targetUser == nil,
		)
	}

	s.log.Info("remote follow recorded",
		"follower", p.FollowerGlobalID,
		"target", p.TargetGlobalID,
	)
	return nil
}

func (s *Service) handlePostCreated(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		GlobalID   string `json:"global_id"`
		AuthorID   string `json:"author_id"`
		Content    string `json:"content"`
		OriginNode string `json:"origin_node"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("handlePostCreated decode: %w", err)
	}

	authorUser, err := s.userRepo.GetByGlobalID(ctx, p.AuthorID)
	if err != nil {
		if appErr, ok := apperr.As(err); ok && appErr.Status == 404 {
			_ = s.userRepo.UpsertRemote(ctx, port.User{
				GlobalID: p.AuthorID,
				HomeNode: p.OriginNode,
				IsLocal:  false,
			})
			authorUser, _ = s.userRepo.GetByGlobalID(ctx, p.AuthorID)
		}
	}
	if authorUser == nil {
		return fmt.Errorf("handlePostCreated: cannot resolve author %s", p.AuthorID)
	}

	if err := s.postRepo.Create(ctx, port.Post{
		GlobalID:   p.GlobalID,
		AuthorID:   authorUser.ID,
		OriginNode: p.OriginNode,
		Content:    p.Content,
		Visibility: "public",
	}); err != nil {
		s.log.Warn("handlePostCreated: post may already exist", "global_id", p.GlobalID, "err", err)
	}

	followerIDs, err := s.feedRepo.GetFollowerUserIDs(ctx, p.AuthorID)
	if err != nil {
		return fmt.Errorf("handlePostCreated get followers: %w", err)
	}

	for _, ownerID := range followerIDs {
		if err := s.feedRepo.AddItem(ctx, port.FeedItem{
			OwnerUserID:  ownerID,
			PostGlobalID: p.GlobalID,
			SourceNode:   p.OriginNode,
		}); err != nil {
			s.log.Warn("handlePostCreated add feed item", "owner", ownerID, "err", err)
		}
	}

	s.log.Info("post.created processed", "global_id", p.GlobalID, "author", p.AuthorID)
	return nil
}

func (s *Service) handlePostUpdated(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		GlobalID string `json:"global_id"`
		Content  string `json:"content"`
		Version  int    `json:"version"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("handlePostUpdated decode: %w", err)
	}

	existing, err := s.postRepo.GetByGlobalID(ctx, p.GlobalID)
	if err != nil || existing == nil {
		s.log.Warn("handlePostUpdated: post not found locally", "global_id", p.GlobalID)
		return nil
	}

	if p.Version > existing.Version {
		if _, err := s.postRepo.Update(ctx, p.GlobalID, p.Content); err != nil {
			return fmt.Errorf("handlePostUpdated: %w", err)
		}
		s.log.Info("post.updated processed", "global_id", p.GlobalID)
	}
	return nil
}

func (s *Service) handlePostDeleted(ctx context.Context, payload json.RawMessage) error {
	var p struct {
		GlobalID string `json:"global_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("handlePostDeleted decode: %w", err)
	}

	if _, err := s.postRepo.Delete(ctx, p.GlobalID); err != nil {
		s.log.Warn("handlePostDeleted: post not found", "global_id", p.GlobalID)
	}
	return nil
}

func (s *Service) GetRemoteUser(ctx context.Context, globalID string) (*port.User, error) {
	u, err := s.userRepo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("federation.GetRemoteUser: %w", err)
	}
	return u, nil
}
