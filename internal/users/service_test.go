package users_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/users"
)

// mockUserRepo — in-memory реализация port.UserRepository.
type mockUserRepo struct {
	data     map[string]*port.User // global_id → user
	byName   map[string]*port.User // local_username → user
	createFn func(port.User) error
}

func newMockRepo() *mockUserRepo {
	return &mockUserRepo{
		data:   make(map[string]*port.User),
		byName: make(map[string]*port.User),
	}
}

func (m *mockUserRepo) Create(_ context.Context, u port.User) error {
	if m.createFn != nil {
		return m.createFn(u)
	}
	m.data[u.GlobalID] = &u
	if u.LocalUsername != "" {
		m.byName[u.LocalUsername] = &u
	}
	return nil
}

func (m *mockUserRepo) GetByID(_ context.Context, id string) (*port.User, error) {
	for _, u := range m.data {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, apperr.ErrUserNotFound
}

func (m *mockUserRepo) GetByGlobalID(_ context.Context, globalID string) (*port.User, error) {
	u, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*port.User, error) {
	u, ok := m.byName[username]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) UpdateProfile(_ context.Context, id, displayName, bio string) error {
	for _, u := range m.data {
		if u.ID == id {
			u.DisplayName = displayName
			u.Bio = bio
			return nil
		}
	}
	return apperr.ErrUserNotFound
}

func (m *mockUserRepo) UpsertRemote(_ context.Context, u port.User) error {
	m.data[u.GlobalID] = &u
	return nil
}

func (m *mockUserRepo) UsernameExists(_ context.Context, username string) (bool, error) {
	_, ok := m.byName[username]
	return ok, nil
}

func (m *mockUserRepo) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	u, ok := m.data[globalID]
	if !ok {
		return "", apperr.ErrUserNotFound
	}
	return u.ID, nil
}

func (m *mockUserRepo) GetPasswordHash(_ context.Context, globalID string) (string, error) {
	u, ok := m.data[globalID]
	if !ok {
		return "", apperr.ErrUserNotFound
	}
	return u.PasswordHash, nil
}

// — Хелперы ──────────────────────────────────────────────────────────

func testCfg() *config.Config {
	return &config.Config{
		NodeName:  "node-a",
		JWTSecret: "secret-long-enough-for-tests-32char",
	}
}

// — Тесты GetProfile ──────────────────────────────────────────────────

func TestService_GetProfile_Found(t *testing.T) {
	repo := newMockRepo()
	repo.byName["alice"] = &port.User{
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		HomeNode:      "node-a",
	}
	svc := users.NewService(repo, testCfg())

	u, err := svc.GetProfile(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.GlobalID != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", u.GlobalID)
	}
}

func TestService_GetProfile_NotFound(t *testing.T) {
	svc := users.NewService(newMockRepo(), testCfg())

	_, err := svc.GetProfile(context.Background(), "nobody")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// — Тесты UpdateProfile ───────────────────────────────────────────────

func TestService_UpdateProfile_Success(t *testing.T) {
	repo := newMockRepo()
	repo.data["alice@node-a"] = &port.User{
		ID:            "uuid-alice",
		GlobalID:      "alice@node-a",
		LocalUsername: "alice",
		HomeNode:      "node-a",
	}
	svc := users.NewService(repo, testCfg())

	u, err := svc.UpdateProfile(context.Background(), "alice@node-a", "Alice W", "bio text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.DisplayName != "Alice W" {
		t.Errorf("display_name = %q, want 'Alice W'", u.DisplayName)
	}
}

func TestService_UpdateProfile_UserNotFound(t *testing.T) {
	svc := users.NewService(newMockRepo(), testCfg())

	_, err := svc.UpdateProfile(context.Background(), "nobody@node-a", "name", "bio")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// — Тесты UpsertRemote ────────────────────────────────────────────────

func TestService_UpsertRemote(t *testing.T) {
	repo := newMockRepo()
	svc := users.NewService(repo, testCfg())

	u := port.User{
		GlobalID: "carol@node-b",
		HomeNode: "node-b",
		IsLocal:  false,
	}
	if err := svc.UpsertRemote(context.Background(), u); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, ok := repo.data["carol@node-b"]
	if !ok {
		t.Fatal("remote user not stored")
	}
	if stored.HomeNode != "node-b" {
		t.Errorf("home_node = %q, want node-b", stored.HomeNode)
	}
}
