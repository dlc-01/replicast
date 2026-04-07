package search

import (
	"context"
	"fmt"
	"strings"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
)

type localUserRepo interface {
	GetByGlobalID(ctx context.Context, globalID string) (*port.User, error)
	GetByUsername(ctx context.Context, username string) (*port.User, error)
}

type remoteUserFetcher interface {
	FetchRemoteUser(ctx context.Context, globalID string) (*port.User, error)
}

type Service struct {
	users  localUserRepo
	remote remoteUserFetcher
	cfg    *config.Config
}

func NewService(users localUserRepo, remote remoteUserFetcher, cfg *config.Config) *Service {
	return &Service{users: users, remote: remote, cfg: cfg}
}

// Search ищет пользователя по q.
//
// Поддерживаемые форматы:
//
//	"alice"              — локальный пользователь на этом узле
//	"alice@node-a"       — если node-a это мы, локально; иначе — удалённый запрос
//	"alice@example.com"  — идём на https://example.com и ищем alice
func (s *Service) Search(ctx context.Context, q string) ([]*port.User, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, apperr.BadRequest("empty_query", "query is empty")
	}

	if strings.Contains(q, "@") {
		return s.searchByGlobalID(ctx, q)
	}

	// Только username — ищем локально
	return s.searchLocal(ctx, q)
}

func (s *Service) searchByGlobalID(ctx context.Context, globalID string) ([]*port.User, error) {
	parts := strings.SplitN(globalID, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, apperr.BadRequest("invalid_query", "format: username@node or username@domain.com")
	}
	node := parts[1]

	// Пользователь на нашем узле
	if node == s.cfg.NodeName {
		return s.searchLocal(ctx, parts[0])
	}

	// Сначала проверяем локальный кэш
	if u, err := s.users.GetByGlobalID(ctx, globalID); err == nil {
		return []*port.User{u}, nil
	}

	// Идём на удалённый узел
	// nodeNameToBaseURL внутри FetchRemoteUser автоматически определяет:
	// - example.com       → https://example.com
	// - node-b            → http://node-b:8080  (Docker)
	// - localhost:8082     → http://localhost:8082
	u, err := s.remote.FetchRemoteUser(ctx, globalID)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	return []*port.User{u}, nil
}

func (s *Service) searchLocal(ctx context.Context, username string) ([]*port.User, error) {
	u, err := s.users.GetByUsername(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("search: local user %q: %w", username, err)
	}
	return []*port.User{u}, nil
}
