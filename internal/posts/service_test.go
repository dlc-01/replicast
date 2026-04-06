package posts_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/posts"
)

// — Моки ──────────────────────────────────────────────────────────────

type mockPostRepo struct {
	data     map[string]*port.Post
	createFn func(port.Post) error
}

func newMockPostRepo() *mockPostRepo {
	return &mockPostRepo{data: make(map[string]*port.Post)}
}

func (m *mockPostRepo) Create(_ context.Context, p port.Post) error {
	if m.createFn != nil {
		return m.createFn(p)
	}
	m.data[p.GlobalID] = &p
	return nil
}
func (m *mockPostRepo) GetByID(_ context.Context, _ string) (*port.Post, error) { return nil, nil }
func (m *mockPostRepo) GetByGlobalID(_ context.Context, globalID string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, nil
	}
	return p, nil
}
func (m *mockPostRepo) Update(_ context.Context, globalID, content string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrPostNotFound
	}
	p.Content = content
	p.Version++
	return p, nil
}
func (m *mockPostRepo) Delete(_ context.Context, globalID string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrPostNotFound
	}
	delete(m.data, globalID)
	return p, nil
}
func (m *mockPostRepo) GetFollowerNodes(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

type mockFeedRepo struct{}

func (m *mockFeedRepo) AddItem(_ context.Context, _ port.FeedItem) error { return nil }
func (m *mockFeedRepo) RemoveItem(_ context.Context, _, _ string) error  { return nil }
func (m *mockFeedRepo) GetFeed(_ context.Context, _ string, _ int) ([]port.FeedPost, error) {
	return nil, nil
}
func (m *mockFeedRepo) GetFollowerUserIDs(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

type mockFedRepo struct{}

func (m *mockFedRepo) EnqueueEvent(_ context.Context, _ port.OutboxEvent) error { return nil }
func (m *mockFedRepo) GetPendingEvents(_ context.Context, _ int) ([]port.OutboxRow, error) {
	return nil, nil
}
func (m *mockFedRepo) MarkDelivered(_ context.Context, _ string) error               { return nil }
func (m *mockFedRepo) MarkFailed(_ context.Context, _ string, _ int) error           { return nil }
func (m *mockFedRepo) IsProcessed(_ context.Context, _ string) (bool, error)         { return false, nil }
func (m *mockFedRepo) MarkProcessed(_ context.Context, _, _ string) error            { return nil }
func (m *mockFedRepo) GetNodeByName(_ context.Context, _ string) (*port.Node, error) { return nil, nil }
func (m *mockFedRepo) UpsertNode(_ context.Context, _ port.Node) error               { return nil }

type mockUserResolver struct {
	uuids map[string]string
}

func (m *mockUserResolver) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	uuid, ok := m.uuids[globalID]
	if !ok {
		return "", apperr.ErrUserNotFound
	}
	return uuid, nil
}

// — Хелперы ────────────────────────────────────────────────────────────

func postCfg() *config.Config { return &config.Config{NodeName: "node-a"} }

func newPostSvc(postRepo *mockPostRepo, userRepo *mockUserResolver) *posts.Service {
	return posts.NewService(postRepo, &mockFeedRepo{}, &mockFedRepo{}, userRepo, logger.Nop(), postCfg())
}

// — Тесты Create ───────────────────────────────────────────────────────

func TestPostService_Create_Success(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	p, err := svc.Create(context.Background(), "alice@node-a", "hello world", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Content != "hello world" {
		t.Errorf("content = %q, want hello world", p.Content)
	}
	if p.AuthorID != "uuid-alice" {
		t.Errorf("author_id = %q, want uuid-alice", p.AuthorID)
	}
	if p.OriginNode != "node-a" {
		t.Errorf("origin_node = %q, want node-a", p.OriginNode)
	}
}

func TestPostService_Create_UserNotFound(t *testing.T) {
	svc := newPostSvc(newMockPostRepo(), &mockUserResolver{uuids: map[string]string{}})

	_, err := svc.Create(context.Background(), "nobody@node-a", "hello", false)
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestPostService_Create_GlobalIDFormat(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	p, _ := svc.Create(context.Background(), "alice@node-a", "test", false)
	prefix := "post:alice@node-a:"
	if !strings.HasPrefix(p.GlobalID, prefix) {
		t.Errorf("global_id = %q, want prefix %q", p.GlobalID, prefix)
	}
}

func TestPostService_Create_HideLikes_True(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	p, err := svc.Create(context.Background(), "alice@node-a", "secret post", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.HideLikes {
		t.Error("HideLikes should be true")
	}
}

func TestPostService_Create_HideLikes_False(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	p, err := svc.Create(context.Background(), "alice@node-a", "public post", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.HideLikes {
		t.Error("HideLikes should be false")
	}
}

// — Тесты Get ──────────────────────────────────────────────────────────

func TestPostService_Get_Found(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		Content:  "hello",
	}
	svc := newPostSvc(postRepo, &mockUserResolver{})

	p, err := svc.Get(context.Background(), "post:alice@node-a:001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Content != "hello" {
		t.Errorf("content = %q, want hello", p.Content)
	}
}

func TestPostService_Get_NotFound(t *testing.T) {
	svc := newPostSvc(newMockPostRepo(), &mockUserResolver{})

	_, err := svc.Get(context.Background(), "post:nobody:999")
	if !errors.Is(err, apperr.ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}
}

// — Тесты Update ───────────────────────────────────────────────────────

func TestPostService_Update_Success(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
		Content:  "original",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(postRepo, userRepo)

	p, err := svc.Update(context.Background(), "post:alice@node-a:001", "alice@node-a", "updated")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Content != "updated" {
		t.Errorf("content = %q, want updated", p.Content)
	}
	if p.Version != 1 {
		t.Errorf("version = %d, want 1 after update", p.Version)
	}
}

func TestPostService_Update_Forbidden(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
		Content:  "original",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{
		"alice@node-a": "uuid-alice",
		"bob@node-a":   "uuid-bob",
	}}
	svc := newPostSvc(postRepo, userRepo)

	_, err := svc.Update(context.Background(), "post:alice@node-a:001", "bob@node-a", "hacked")
	if !errors.Is(err, apperr.ErrPostForbidden) {
		t.Errorf("expected ErrPostForbidden, got %v", err)
	}
}

func TestPostService_Update_NotFound(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	_, err := svc.Update(context.Background(), "post:nobody:999", "alice@node-a", "text")
	if !errors.Is(err, apperr.ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}
}

// — Тесты Delete ───────────────────────────────────────────────────────

func TestPostService_Delete_Success(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(postRepo, userRepo)

	if err := svc.Delete(context.Background(), "post:alice@node-a:001", "alice@node-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := postRepo.data["post:alice@node-a:001"]; ok {
		t.Error("post should be deleted")
	}
}

func TestPostService_Delete_Forbidden(t *testing.T) {
	postRepo := newMockPostRepo()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		AuthorID: "uuid-alice",
	}
	userRepo := &mockUserResolver{uuids: map[string]string{
		"alice@node-a": "uuid-alice",
		"bob@node-a":   "uuid-bob",
	}}
	svc := newPostSvc(postRepo, userRepo)

	err := svc.Delete(context.Background(), "post:alice@node-a:001", "bob@node-a")
	if !errors.Is(err, apperr.ErrPostForbidden) {
		t.Errorf("expected ErrPostForbidden, got %v", err)
	}
}

func TestPostService_Delete_NotFound(t *testing.T) {
	userRepo := &mockUserResolver{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newPostSvc(newMockPostRepo(), userRepo)

	err := svc.Delete(context.Background(), "post:nobody:999", "alice@node-a")
	if !errors.Is(err, apperr.ErrPostNotFound) {
		t.Errorf("expected ErrPostNotFound, got %v", err)
	}
}
