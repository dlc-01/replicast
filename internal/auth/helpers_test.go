package auth_test

import (
	"context"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
)

// authTestCfg — общий конфиг для всех тестов auth пакета.
func authTestCfg() *config.Config {
	return &config.Config{
		NodeName:  "node-a",
		JWTSecret: "test-secret-key-long-enough-32chars!",
	}
}

// mockAuthRepo — общий мок репозитория для service_test и handler_test.
type mockAuthRepo struct {
	users map[string]*port.User
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{users: make(map[string]*port.User)}
}

func (m *mockAuthRepo) CreateUser(_ context.Context, u port.User) error {
	m.users[u.GlobalID] = &u
	return nil
}

func (m *mockAuthRepo) GetByGlobalID(_ context.Context, globalID string) (*port.User, error) {
	u, ok := m.users[globalID]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}

func (m *mockAuthRepo) UsernameExists(_ context.Context, username string) (bool, error) {
	for _, u := range m.users {
		if u.LocalUsername == username {
			return true, nil
		}
	}
	return false, nil
}

func (m *mockAuthRepo) GetPasswordHash(_ context.Context, globalID string) (string, error) {
	u, ok := m.users[globalID]
	if !ok {
		return "", apperr.ErrInvalidPassword
	}
	return u.PasswordHash, nil
}
