package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/search"
)

// — Моки ──────────────────────────────────────────────────────────────

type mockUserRepo struct {
	byGlobalID map[string]*port.User
	byUsername map[string]*port.User
}

func (m *mockUserRepo) GetByGlobalID(_ context.Context, globalID string) (*port.User, error) {
	u, ok := m.byGlobalID[globalID]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}

func (m *mockUserRepo) GetByUsername(_ context.Context, username string) (*port.User, error) {
	u, ok := m.byUsername[username]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}

type mockRemote struct {
	users map[string]*port.User
	err   error
}

func (m *mockRemote) FetchRemoteUser(_ context.Context, globalID string) (*port.User, error) {
	if m.err != nil {
		return nil, m.err
	}
	u, ok := m.users[globalID]
	if !ok {
		return nil, errors.New("not found on remote")
	}
	return u, nil
}

func testCfg() *config.Config {
	return &config.Config{NodeName: "node-a"}
}

// — Тесты ─────────────────────────────────────────────────────────────

func TestSearch_LocalUsername(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{
			"alice": {GlobalID: "alice@node-a", LocalUsername: "alice"},
		},
		byGlobalID: map[string]*port.User{},
	}
	svc := search.NewService(repo, &mockRemote{users: map[string]*port.User{}}, testCfg())

	results, err := svc.Search(context.Background(), "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].GlobalID != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", results[0].GlobalID)
	}
}

func TestSearch_LocalGlobalID(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{
			"alice": {GlobalID: "alice@node-a", LocalUsername: "alice"},
		},
		byGlobalID: map[string]*port.User{},
	}
	svc := search.NewService(repo, &mockRemote{users: map[string]*port.User{}}, testCfg())

	// alice@node-a — наш узел, ищем локально
	results, err := svc.Search(context.Background(), "alice@node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].GlobalID != "alice@node-a" {
		t.Errorf("want alice@node-a, got %+v", results)
	}
}

func TestSearch_RemoteUser_FromCache(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{},
		byGlobalID: map[string]*port.User{
			"bob@node-b": {GlobalID: "bob@node-b", HomeNode: "node-b"},
		},
	}
	svc := search.NewService(repo, &mockRemote{users: map[string]*port.User{}}, testCfg())

	results, err := svc.Search(context.Background(), "bob@node-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].GlobalID != "bob@node-b" {
		t.Errorf("want bob@node-b from cache, got %+v", results)
	}
}

func TestSearch_RemoteUser_FetchesFromRemote(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{},
		byGlobalID: map[string]*port.User{}, // нет в кэше
	}
	remote := &mockRemote{users: map[string]*port.User{
		"bob@node-b": {GlobalID: "bob@node-b", HomeNode: "node-b"},
	}}
	svc := search.NewService(repo, remote, testCfg())

	results, err := svc.Search(context.Background(), "bob@node-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].GlobalID != "bob@node-b" {
		t.Errorf("want bob@node-b from remote, got %+v", results)
	}
}

func TestSearch_RemoteDomain(t *testing.T) {
	// alice@social.example.com — домен, идём туда
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{},
		byGlobalID: map[string]*port.User{},
	}
	remote := &mockRemote{users: map[string]*port.User{
		"alice@social.example.com": {GlobalID: "alice@social.example.com", HomeNode: "social.example.com"},
	}}
	svc := search.NewService(repo, remote, testCfg())

	results, err := svc.Search(context.Background(), "alice@social.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	svc := search.NewService(
		&mockUserRepo{byUsername: map[string]*port.User{}, byGlobalID: map[string]*port.User{}},
		&mockRemote{users: map[string]*port.User{}},
		testCfg(),
	)

	_, err := svc.Search(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestSearch_InvalidFormat(t *testing.T) {
	svc := search.NewService(
		&mockUserRepo{byUsername: map[string]*port.User{}, byGlobalID: map[string]*port.User{}},
		&mockRemote{users: map[string]*port.User{}},
		testCfg(),
	)

	_, err := svc.Search(context.Background(), "@")
	if err == nil {
		t.Error("expected error for invalid format @")
	}
}

func TestSearch_RemoteUnavailable(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{},
		byGlobalID: map[string]*port.User{},
	}
	remote := &mockRemote{err: errors.New("connection refused")}
	svc := search.NewService(repo, remote, testCfg())

	_, err := svc.Search(context.Background(), "bob@node-b")
	if err == nil {
		t.Error("expected error when remote unavailable")
	}
}

func TestSearch_LocalNotFound(t *testing.T) {
	repo := &mockUserRepo{
		byUsername: map[string]*port.User{},
		byGlobalID: map[string]*port.User{},
	}
	svc := search.NewService(repo, &mockRemote{users: map[string]*port.User{}}, testCfg())

	_, err := svc.Search(context.Background(), "nobody")
	if err == nil {
		t.Error("expected error for unknown local user")
	}
}
