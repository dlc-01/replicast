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

type nodeRegistry interface {
	GetNodeByName(ctx context.Context, name string) (*port.Node, error)
	UpsertNode(ctx context.Context, n port.Node) error
}

// WellKnownInfo — результат discovery узла.
type WellKnownInfo struct {
	Node    string
	BaseURL string
}

type nodeDiscoverer interface {
	FetchWellKnown(ctx context.Context, nodeName string) (*WellKnownInfo, error)
}

// discovererAdapter оборачивает federation.Client чтобы вернуть локальный тип.
type discovererAdapter struct {
	inner interface {
		FetchWellKnown(ctx context.Context, nodeName string) (interface {
			GetNode() string
			GetBaseURL() string
		}, error)
	}
}

// FedDiscoverer — интерфейс который реализует federation.Client.
// Используем его напрямую чтобы избежать import cycle.
type FedDiscoverer interface {
	FetchWellKnownInfo(ctx context.Context, nodeName string) (node, baseURL string, err error)
}

type Service struct {
	followRepo followRepository
	userRepo   userLookup
	fedRepo    fedEnqueuer
	nodeRepo   nodeRegistry
	discoverer FedDiscoverer
	log        logger.Logger
	cfg        *config.Config
}

func NewService(
	followRepo followRepository,
	userRepo userLookup,
	fedRepo fedEnqueuer,
	nodeRepo nodeRegistry,
	discoverer FedDiscoverer,
	log logger.Logger,
	cfg *config.Config,
) *Service {
	return &Service{
		followRepo: followRepo,
		userRepo:   userRepo,
		fedRepo:    fedRepo,
		nodeRepo:   nodeRepo,
		discoverer: discoverer,
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

	// Удалённый узел — убеждаемся что знаем его через /.well-known/replicast
	if targetNode != s.cfg.NodeName {
		if err := s.ensureNodeKnown(ctx, targetNode); err != nil {
			return fmt.Errorf("follows.Follow discover node: %w", err)
		}
	}

	if err := s.followRepo.Create(ctx, port.Follow{
		FollowerUserID:     followerUUID,
		TargetGlobalUserID: targetGlobalID,
		TargetNode:         targetNode,
	}); err != nil {
		return fmt.Errorf("follows.Follow create: %w", err)
	}

	// Уведомляем удалённый узел через outbox
	if targetNode != s.cfg.NodeName {
		_ = s.fedRepo.EnqueueEvent(ctx, port.OutboxEvent{
			TargetNode: targetNode,
			EventType:  "user.followed",
			Payload: map[string]any{
				"follower_global_id": followerGlobalID,
				"target_global_id":   targetGlobalID,
				"follower_node":      s.cfg.NodeName,
				"follower_base_url":  s.cfg.BaseURL,
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

	s.log.Info("follow removed", "follower", followerGlobalID, "target", targetGlobalID)
	return nil
}

// ensureNodeKnown проверяет таблицу nodes.
// Если узел неизвестен — делает discovery через /.well-known/replicast.
func (s *Service) ensureNodeKnown(ctx context.Context, nodeName string) error {
	_, err := s.nodeRepo.GetNodeByName(ctx, nodeName)
	if err == nil {
		return nil // узел уже известен
	}

	s.log.Info("discovering new node via well-known", "node", nodeName)

	node, baseURL, err := s.discoverer.FetchWellKnownInfo(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("ensureNodeKnown: %w", err)
	}

	if err := s.nodeRepo.UpsertNode(ctx, port.Node{
		Name:         node,
		BaseURL:      baseURL,
		SharedSecret: s.cfg.SharedSecret,
		Status:       "active",
	}); err != nil {
		return fmt.Errorf("ensureNodeKnown upsert: %w", err)
	}

	s.log.Info("node discovered and saved", "node", node, "base_url", baseURL)
	return nil
}

func nodeFromGlobalID(globalID string) string {
	if i := strings.LastIndex(globalID, "@"); i != -1 {
		return globalID[i+1:]
	}
	return ""
}
