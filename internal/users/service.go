package users

import (
	"context"
	"fmt"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type Service struct {
	repo port.UserRepository
	log  logger.Logger
	cfg  *config.Config
}

func NewService(repo port.UserRepository, cfg *config.Config) *Service {
	return &Service{repo: repo, log: logger.Nop(), cfg: cfg}
}

// NewServiceWithLogger создаёт сервис с явным логгером — используется в app.go.
func NewServiceWithLogger(repo port.UserRepository, log logger.Logger, cfg *config.Config) *Service {
	return &Service{repo: repo, log: log, cfg: cfg}
}

func (s *Service) GetProfile(ctx context.Context, username string) (*port.User, error) {
	u, err := s.repo.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("users.GetProfile: %w", err)
	}
	return u, nil
}

func (s *Service) UpdateProfile(ctx context.Context, globalID, displayName, bio string) (*port.User, error) {
	uuid, err := s.repo.GetUUIDByGlobalID(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("users.UpdateProfile: %w", err)
	}
	if err := s.repo.UpdateProfile(ctx, uuid, displayName, bio); err != nil {
		return nil, fmt.Errorf("users.UpdateProfile: %w", err)
	}
	u, err := s.repo.GetByID(ctx, uuid)
	if err != nil {
		return nil, fmt.Errorf("users.UpdateProfile get: %w", err)
	}
	s.log.Info("profile updated", "global_id", globalID)
	return u, nil
}

// GetByGlobalID возвращает профиль по global_id — используется federation сервисом.
func (s *Service) GetByGlobalID(ctx context.Context, globalID string) (*port.User, error) {
	u, err := s.repo.GetByGlobalID(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("users.GetByGlobalID: %w", err)
	}
	return u, nil
}

// UpsertRemote сохраняет профиль удалённого пользователя — используется federation.
func (s *Service) UpsertRemote(ctx context.Context, u port.User) error {
	if err := s.repo.UpsertRemote(ctx, u); err != nil {
		return fmt.Errorf("users.UpsertRemote: %w", err)
	}
	return nil
}

// GetUUIDByGlobalID резолвит global_id → UUID для сервисов которым нужен внутренний ID.
func (s *Service) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	uuid, err := s.repo.GetUUIDByGlobalID(ctx, globalID)
	if err != nil {
		return "", fmt.Errorf("users.GetUUIDByGlobalID: %w", err)
	}
	return uuid, nil
}

// CreateRemoteStub создаёт заглушку для пользователя с другого узла если его ещё нет локально.
func (s *Service) CreateRemoteStub(ctx context.Context, globalID, homeNode string) error {
	_, err := s.repo.GetByGlobalID(ctx, globalID)
	if err == nil {
		return nil // уже существует
	}
	if !isNotFound(err) {
		return fmt.Errorf("users.CreateRemoteStub check: %w", err)
	}
	return s.repo.UpsertRemote(ctx, port.User{
		GlobalID: globalID,
		HomeNode: homeNode,
		IsLocal:  false,
	})
}

func isNotFound(err error) bool {
	appErr, ok := apperr.As(err)
	return ok && appErr.Status == 404
}
