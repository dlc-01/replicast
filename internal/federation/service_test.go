package federation_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// — Моки ──────────────────────────────────────────────────────────────

type mockFedRepoFull struct {
	nodes     map[string]*port.Node
	processed map[string]bool
	outbox    []port.OutboxEvent
	markErr   error
}

func newMockFedRepoFull() *mockFedRepoFull {
	return &mockFedRepoFull{
		nodes:     make(map[string]*port.Node),
		processed: make(map[string]bool),
	}
}

func (m *mockFedRepoFull) EnqueueEvent(_ context.Context, e port.OutboxEvent) error {
	m.outbox = append(m.outbox, e)
	return nil
}
func (m *mockFedRepoFull) GetPendingEvents(_ context.Context, _ int) ([]port.OutboxRow, error) {
	return nil, nil
}
func (m *mockFedRepoFull) MarkDelivered(_ context.Context, _ string) error     { return nil }
func (m *mockFedRepoFull) MarkFailed(_ context.Context, _ string, _ int) error { return nil }
func (m *mockFedRepoFull) IsProcessed(_ context.Context, eventID string) (bool, error) {
	return m.processed[eventID], nil
}
func (m *mockFedRepoFull) MarkProcessed(_ context.Context, eventID, _ string) error {
	m.processed[eventID] = true
	return m.markErr
}
func (m *mockFedRepoFull) GetNodeByName(_ context.Context, name string) (*port.Node, error) {
	n, ok := m.nodes[name]
	if !ok {
		return nil, apperr.NotFound("node_not_found", "node not found")
	}
	return n, nil
}
func (m *mockFedRepoFull) UpsertNode(_ context.Context, n port.Node) error {
	m.nodes[n.Name] = &n
	return nil
}

type mockPostRepoFed struct {
	data map[string]*port.Post
}

func newMockPostRepoFed() *mockPostRepoFed {
	return &mockPostRepoFed{data: make(map[string]*port.Post)}
}

func (m *mockPostRepoFed) Create(_ context.Context, p port.Post) error {
	m.data[p.GlobalID] = &p
	return nil
}
func (m *mockPostRepoFed) GetByID(_ context.Context, _ string) (*port.Post, error) { return nil, nil }
func (m *mockPostRepoFed) GetByGlobalID(_ context.Context, globalID string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, nil
	}
	return p, nil
}
func (m *mockPostRepoFed) Update(_ context.Context, globalID, content string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrPostNotFound
	}
	p.Content = content
	p.Version++
	return p, nil
}
func (m *mockPostRepoFed) Delete(_ context.Context, globalID string) (*port.Post, error) {
	p, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrPostNotFound
	}
	delete(m.data, globalID)
	return p, nil
}
func (m *mockPostRepoFed) GetFollowerNodes(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

type mockUserRepoFed struct {
	data map[string]*port.User
}

func newMockUserRepoFed() *mockUserRepoFed {
	return &mockUserRepoFed{data: make(map[string]*port.User)}
}

func (m *mockUserRepoFed) Create(_ context.Context, u port.User) error {
	m.data[u.GlobalID] = &u
	return nil
}
func (m *mockUserRepoFed) GetByID(_ context.Context, id string) (*port.User, error) {
	for _, u := range m.data {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, apperr.ErrUserNotFound
}
func (m *mockUserRepoFed) GetByGlobalID(_ context.Context, globalID string) (*port.User, error) {
	u, ok := m.data[globalID]
	if !ok {
		return nil, apperr.ErrUserNotFound
	}
	return u, nil
}
func (m *mockUserRepoFed) GetByUsername(_ context.Context, _ string) (*port.User, error) {
	return nil, apperr.ErrUserNotFound
}
func (m *mockUserRepoFed) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	u, ok := m.data[globalID]
	if !ok {
		return "", apperr.ErrUserNotFound
	}
	return u.ID, nil
}
func (m *mockUserRepoFed) UpdateProfile(_ context.Context, _, _, _ string) error { return nil }
func (m *mockUserRepoFed) UpsertRemote(_ context.Context, u port.User) error {
	m.data[u.GlobalID] = &u
	return nil
}
func (m *mockUserRepoFed) UsernameExists(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *mockUserRepoFed) GetPasswordHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

type mockFeedRepoFed struct {
	items     []port.FeedItem
	followers map[string][]string
}

func newMockFeedRepoFed() *mockFeedRepoFed {
	return &mockFeedRepoFed{followers: make(map[string][]string)}
}

func (m *mockFeedRepoFed) AddItem(_ context.Context, item port.FeedItem) error {
	m.items = append(m.items, item)
	return nil
}
func (m *mockFeedRepoFed) RemoveItem(_ context.Context, _, _ string) error { return nil }
func (m *mockFeedRepoFed) GetFeed(_ context.Context, _ string, _ int) ([]port.FeedPost, error) {
	return nil, nil
}
func (m *mockFeedRepoFed) GetFollowerUserIDs(_ context.Context, authorGlobalID string) ([]string, error) {
	return m.followers[authorGlobalID], nil
}
func (m *mockFeedRepoFed) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	return "uuid-" + globalID, nil
}

// — Хелпер ────────────────────────────────────────────────────────────

type mockFollowWriterS struct {
	created []port.Follow
}

func (m *mockFollowWriterS) Create(_ context.Context, f port.Follow) error {
	m.created = append(m.created, f)
	return nil
}

func newFedSvc(fedRepo *mockFedRepoFull, postRepo *mockPostRepoFed, userRepo *mockUserRepoFed, feedRepo *mockFeedRepoFed) *federation.Service {
	cfg := &config.Config{NodeName: "node-a", BaseURL: "http://node-a:8080", InternalBaseURL: "http://node-a:8080", SharedSecret: "secret"}
	return federation.NewService(fedRepo, postRepo, userRepo, feedRepo, &mockFollowWriterS{}, logger.Nop(), cfg)
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// — Тесты Handshake ───────────────────────────────────────────────────

func TestFedService_Handshake_SavesNode(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	svc := newFedSvc(fedRepo, newMockPostRepoFed(), newMockUserRepoFed(), newMockFeedRepoFed())

	err := svc.Handshake(context.Background(), "node-b", "http://node-b:8080", "secret")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	node, ok := fedRepo.nodes["node-b"]
	if !ok {
		t.Fatal("node-b should be saved")
	}
	if node.BaseURL != "http://node-b:8080" {
		t.Errorf("base_url = %q, want http://node-b:8080", node.BaseURL)
	}
}

// — Тесты ReceiveEvent — идемпотентность ─────────────────────────────

func TestFedService_ReceiveEvent_Idempotent(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	userRepo := newMockUserRepoFed()
	userRepo.data["alice@node-a"] = &port.User{ID: "uuid-alice", GlobalID: "alice@node-a", HomeNode: "node-a"}

	svc := newFedSvc(fedRepo, postRepo, userRepo, newMockFeedRepoFed())

	payload := mustJSON(map[string]any{
		"global_id":   "post:alice@node-a:001",
		"author_id":   "alice@node-a",
		"content":     "hello",
		"origin_node": "node-a",
	})

	// Первый раз — обрабатываем
	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.created", payload)
	if err != nil {
		t.Fatalf("first event: %v", err)
	}
	if len(postRepo.data) != 1 {
		t.Errorf("expected 1 post, got %d", len(postRepo.data))
	}

	// Второй раз — пропускаем (идемпотентность)
	err = svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.created", payload)
	if err != nil {
		t.Fatalf("second event: %v", err)
	}
	// Пост не дублируется — всё ещё 1
	if len(postRepo.data) != 1 {
		t.Errorf("expected 1 post after duplicate, got %d", len(postRepo.data))
	}
}

// — Тесты ReceiveEvent — post.created ────────────────────────────────

func TestFedService_ReceiveEvent_PostCreated_AddsToFeed(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	userRepo := newMockUserRepoFed()
	feedRepo := newMockFeedRepoFed()

	// Автор существует локально
	userRepo.data["alice@node-a"] = &port.User{
		ID: "uuid-alice", GlobalID: "alice@node-a", HomeNode: "node-a",
	}
	// bob подписан на alice
	feedRepo.followers["alice@node-a"] = []string{"uuid-bob"}

	svc := newFedSvc(fedRepo, postRepo, userRepo, feedRepo)

	payload := mustJSON(map[string]any{
		"global_id":   "post:alice@node-a:001",
		"author_id":   "alice@node-a",
		"content":     "hello from alice",
		"origin_node": "node-a",
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.created", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Пост сохранён
	if _, ok := postRepo.data["post:alice@node-a:001"]; !ok {
		t.Error("post should be saved")
	}

	// Пост добавлен в ленту bob'а
	if len(feedRepo.items) != 1 {
		t.Errorf("expected 1 feed item, got %d", len(feedRepo.items))
	}
	if feedRepo.items[0].OwnerUserID != "uuid-bob" {
		t.Errorf("owner = %q, want uuid-bob", feedRepo.items[0].OwnerUserID)
	}
}

func TestFedService_ReceiveEvent_PostCreated_CreatesRemoteUserStub(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	userRepo := newMockUserRepoFed()
	// Автор НЕ существует локально

	svc := newFedSvc(fedRepo, postRepo, userRepo, newMockFeedRepoFed())

	payload := mustJSON(map[string]any{
		"global_id":   "post:carol@node-c:001",
		"author_id":   "carol@node-c",
		"content":     "hello from carol",
		"origin_node": "node-c",
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-c", "post.created", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Заглушка автора создана
	if _, ok := userRepo.data["carol@node-c"]; !ok {
		t.Error("remote user stub should be created")
	}
}

// — Тесты ReceiveEvent — post.updated ────────────────────────────────

func TestFedService_ReceiveEvent_PostUpdated_NewerVersion(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		Content:  "original",
		Version:  1,
	}

	svc := newFedSvc(fedRepo, postRepo, newMockUserRepoFed(), newMockFeedRepoFed())

	payload := mustJSON(map[string]any{
		"global_id": "post:alice@node-a:001",
		"content":   "updated",
		"version":   2,
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.updated", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if postRepo.data["post:alice@node-a:001"].Content != "updated" {
		t.Error("post content should be updated")
	}
}

func TestFedService_ReceiveEvent_PostUpdated_OlderVersion_Ignored(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	postRepo.data["post:alice@node-a:001"] = &port.Post{
		GlobalID: "post:alice@node-a:001",
		Content:  "current",
		Version:  5,
	}

	svc := newFedSvc(fedRepo, postRepo, newMockUserRepoFed(), newMockFeedRepoFed())

	payload := mustJSON(map[string]any{
		"global_id": "post:alice@node-a:001",
		"content":   "old content",
		"version":   3, // старее чем 5
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.updated", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Контент не изменился — версия старее
	if postRepo.data["post:alice@node-a:001"].Content != "current" {
		t.Error("post content should NOT be updated with older version")
	}
}

// — Тесты ReceiveEvent — post.deleted ────────────────────────────────

func TestFedService_ReceiveEvent_PostDeleted(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	postRepo := newMockPostRepoFed()
	postRepo.data["post:alice@node-a:001"] = &port.Post{GlobalID: "post:alice@node-a:001"}

	svc := newFedSvc(fedRepo, postRepo, newMockUserRepoFed(), newMockFeedRepoFed())

	payload := mustJSON(map[string]any{"global_id": "post:alice@node-a:001"})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-a", "post.deleted", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := postRepo.data["post:alice@node-a:001"]; ok {
		t.Error("post should be deleted")
	}
}

// — Тесты ReceiveEvent — user.followed ───────────────────────────────

func TestFedService_ReceiveEvent_UserFollowed_SavesRemoteUser(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	userRepo := newMockUserRepoFed()
	userRepo.data["alice@node-a"] = &port.User{
		ID: "uuid-alice", GlobalID: "alice@node-a", HomeNode: "node-a",
	}

	svc := newFedSvc(fedRepo, newMockPostRepoFed(), userRepo, newMockFeedRepoFed())

	payload := mustJSON(map[string]any{
		"follower_global_id": "bob@node-b",
		"target_global_id":   "alice@node-a",
		"follower_node":      "node-b",
		"follower_base_url":  "http://node-b:8080",
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-b", "user.followed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Remote user stub создан
	if _, ok := userRepo.data["bob@node-b"]; !ok {
		t.Error("bob@node-b stub should be created")
	}

	// Узел node-b сохранён
	if _, ok := fedRepo.nodes["node-b"]; !ok {
		t.Error("node-b should be saved in nodes table")
	}
}

// — Тесты GetRemoteUser ───────────────────────────────────────────────

func TestFedService_GetRemoteUser_Found(t *testing.T) {
	userRepo := newMockUserRepoFed()
	userRepo.data["alice@node-a"] = &port.User{
		ID: "uuid-alice", GlobalID: "alice@node-a", HomeNode: "node-a",
	}
	svc := newFedSvc(newMockFedRepoFull(), newMockPostRepoFed(), userRepo, newMockFeedRepoFed())

	u, err := svc.GetRemoteUser(context.Background(), "alice@node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.GlobalID != "alice@node-a" {
		t.Errorf("global_id = %q, want alice@node-a", u.GlobalID)
	}
}

func TestFedService_GetRemoteUser_NotFound(t *testing.T) {
	svc := newFedSvc(newMockFedRepoFull(), newMockPostRepoFed(), newMockUserRepoFed(), newMockFeedRepoFed())

	_, err := svc.GetRemoteUser(context.Background(), "nobody@node-x")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// — Тесты NodeInfo ────────────────────────────────────────────────────

func TestFedService_NodeInfo(t *testing.T) {
	svc := newFedSvc(newMockFedRepoFull(), newMockPostRepoFed(), newMockUserRepoFed(), newMockFeedRepoFed())

	info := svc.NodeInfo()
	if info["node"] != "node-a" {
		t.Errorf("node = %q, want node-a", info["node"])
	}
	if info["version"] != "1" {
		t.Errorf("version = %q, want 1", info["version"])
	}
}

func TestFedService_NodeInfo_UsesInternalBaseURL(t *testing.T) {
	cfg := &config.Config{
		NodeName:        "node-a",
		BaseURL:         "http://localhost:8081", // внешний
		InternalBaseURL: "http://node-a:8080",    // внутренний
		SharedSecret:    "secret",
	}
	svc := federation.NewService(
		newMockFedRepoFull(), newMockPostRepoFed(), newMockUserRepoFed(), newMockFeedRepoFed(),
		&mockFollowWriterS{}, logger.Nop(), cfg,
	)

	info := svc.NodeInfo()

	// federation использует внутренний адрес
	if info["base_url"] != "http://node-a:8080" {
		t.Errorf("base_url = %q, want http://node-a:8080", info["base_url"])
	}
	// публичный адрес тоже присутствует
	if info["public_url"] != "http://localhost:8081" {
		t.Errorf("public_url = %q, want http://localhost:8081", info["public_url"])
	}
}

func TestFedService_NodeInfo_FallbackWhenNoInternal(t *testing.T) {
	cfg := &config.Config{
		NodeName:        "node-a",
		BaseURL:         "http://localhost:8081",
		InternalBaseURL: "http://localhost:8081",
		SharedSecret:    "secret",
	}
	svc := federation.NewService(
		newMockFedRepoFull(), newMockPostRepoFed(), newMockUserRepoFed(), newMockFeedRepoFed(),
		&mockFollowWriterS{}, logger.Nop(), cfg,
	)

	info := svc.NodeInfo()
	if info["base_url"] != info["public_url"] {
		t.Error("when InternalBaseURL == BaseURL, both should be equal")
	}
}

func TestFedService_UserFollowed_CreatesFollowRecord(t *testing.T) {
	fedRepo := newMockFedRepoFull()
	userRepo := newMockUserRepoFed()
	userRepo.data["alice@node-a"] = &port.User{
		ID: "uuid-alice", GlobalID: "alice@node-a", HomeNode: "node-a",
	}
	followWriter := &mockFollowWriterS{}

	cfg := &config.Config{NodeName: "node-a", BaseURL: "http://node-a:8080", InternalBaseURL: "http://node-a:8080", SharedSecret: "secret"}
	svc := federation.NewService(fedRepo, newMockPostRepoFed(), userRepo, newMockFeedRepoFed(), followWriter, logger.Nop(), cfg)

	payload := mustJSON(map[string]any{
		"follower_global_id": "bob@node-b",
		"target_global_id":   "alice@node-a",
		"follower_node":      "node-b",
		"follower_base_url":  "http://node-b:8080",
	})

	err := svc.ReceiveEvent(context.Background(), "evt-001", "node-b", "user.followed", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Запись в follows создана — это ключевое для GetFollowerNodes
	if len(followWriter.created) != 1 {
		t.Errorf("expected 1 follow record, got %d", len(followWriter.created))
	}
	if followWriter.created[0].TargetGlobalUserID != "alice@node-a" {
		t.Errorf("target = %q, want alice@node-a", followWriter.created[0].TargetGlobalUserID)
	}
}
