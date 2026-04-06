package auth_test

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/auth"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

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

func TestAuthService_Register_ReturnsE2EKeys(t *testing.T) {
	svc := auth.NewService(newMockAuthRepo(), logger.Nop(), authTestCfg())

	result, err := svc.Register(context.Background(), "alice", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Приватный ключ возвращается клиенту один раз
	if result.PrivateKey == "" {
		t.Error("private_key should be returned on registration")
	}
	// Публичный ключ сохранён в пользователе
	if result.User.PublicKey == "" {
		t.Error("public_key should be stored in user")
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
