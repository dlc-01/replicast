package auth_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// — Хелперы ──────────────────────────────────────────────────────────

func authTestCfg() *config.Config {
	return &config.Config{
		NodeName:  "node-a",
		JWTSecret: "test-secret-key-long-enough-32chars!",
	}
}

// mockAuthRepo реализует только authRepository интерфейс (4 метода).
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

// — Тесты ────────────────────────────────────────────────────────────

func TestAuthService_Register(t *testing.T) {
	svc := auth.NewService(newMockAuthRepo(), logger.Nop(), authTestCfg())

	result, err := svc.Register(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Token == "" {
		t.Error("expected non-empty token")
	}
	if result.User.GlobalID != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", result.User.GlobalID)
	}
}

func TestAuthService_Register_Duplicate(t *testing.T) {
	svc := auth.NewService(newMockAuthRepo(), logger.Nop(), authTestCfg())

	_, err := svc.Register(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	_, err = svc.Register(context.Background(), "alice", "password123")
	if !errors.Is(err, apperr.ErrUserExists) {
		t.Errorf("expected ErrUserExists, got %v", err)
	}
}

func TestAuthService_Login(t *testing.T) {
	repo := newMockAuthRepo()

	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	repo.users["alice@node-a"] = &port.User{
		ID:            "test-uuid-alice",
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		PasswordHash:  string(hash),
	}

	svc := auth.NewService(repo, logger.Nop(), authTestCfg())

	tests := []struct {
		name     string
		username string
		password string
		wantErr  error
	}{
		{"success", "alice", "password123", nil},
		{"wrong password", "alice", "wrongpass", apperr.ErrInvalidPassword},
		{"unknown user", "nobody", "password123", apperr.ErrInvalidPassword},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := svc.Login(context.Background(), tt.username, tt.password)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if token == "" {
				t.Error("expected non-empty token")
			}
		})
	}
}
