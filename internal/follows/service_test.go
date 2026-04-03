package follows_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/follows"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

// — Моки ─────────────────────────────────────────────────────────────

type mockFollowRepo struct {
	data map[string]port.Follow
}

func newMockFollowRepo() *mockFollowRepo {
	return &mockFollowRepo{data: make(map[string]port.Follow)}
}

func followKey(followerUUID, targetGlobalID string) string {
	return followerUUID + ":" + targetGlobalID
}

func (m *mockFollowRepo) Create(_ context.Context, f port.Follow) error {
	m.data[followKey(f.FollowerUserID, f.TargetGlobalUserID)] = f
	return nil
}
func (m *mockFollowRepo) Delete(_ context.Context, followerUUID, targetGlobalID string) error {
	delete(m.data, followKey(followerUUID, targetGlobalID))
	return nil
}
func (m *mockFollowRepo) Exists(_ context.Context, followerUUID, targetGlobalID string) (bool, error) {
	_, ok := m.data[followKey(followerUUID, targetGlobalID)]
	return ok, nil
}

type mockUserLookup struct {
	uuids map[string]string
}

func (m *mockUserLookup) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	uuid, ok := m.uuids[globalID]
	if !ok {
		return "", apperr.ErrUserNotFound
	}
	return uuid, nil
}

type mockFedEnqueuer struct {
	events []port.OutboxEvent
}

func (m *mockFedEnqueuer) EnqueueEvent(_ context.Context, e port.OutboxEvent) error {
	m.events = append(m.events, e)
	return nil
}

type mockNodeRegistry struct {
	nodes map[string]*port.Node
}

func newMockNodeRegistry() *mockNodeRegistry {
	return &mockNodeRegistry{nodes: make(map[string]*port.Node)}
}

func (m *mockNodeRegistry) GetNodeByName(_ context.Context, name string) (*port.Node, error) {
	n, ok := m.nodes[name]
	if !ok {
		return nil, apperr.NotFound("node_not_found", "node not found")
	}
	return n, nil
}
func (m *mockNodeRegistry) UpsertNode(_ context.Context, n port.Node) error {
	m.nodes[n.Name] = &n
	return nil
}

// mockDiscoverer имитирует FetchWellKnownInfo
type mockDiscoverer struct {
	results map[string][2]string // nodeName → [node, baseURL]
	err     error
}

func (m *mockDiscoverer) FetchWellKnownInfo(_ context.Context, nodeName string) (string, string, error) {
	if m.err != nil {
		return "", "", m.err
	}
	r, ok := m.results[nodeName]
	if !ok {
		return "", "", errors.New("node not reachable: " + nodeName)
	}
	return r[0], r[1], nil
}

// — Хелперы ──────────────────────────────────────────────────────────

func followTestCfg() *config.Config {
	return &config.Config{NodeName: "node-a", BaseURL: "http://node-a:8080", SharedSecret: "secret"}
}

func newSvc(fr *mockFollowRepo, ur *mockUserLookup, fe *mockFedEnqueuer, nr *mockNodeRegistry, disc *mockDiscoverer) *follows.Service {
	return follows.NewService(fr, ur, fe, nr, disc, logger.Nop(), followTestCfg())
}

// — Follow — локальный ───────────────────────────────────────────────

func TestFollow_Local_NoFedEvent(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	svc := newSvc(followRepo, userRepo, fedRepo, newMockNodeRegistry(), &mockDiscoverer{})

	if err := svc.Follow(context.Background(), "alice@node-a", "bob@node-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, _ := followRepo.Exists(context.Background(), "uuid-alice", "bob@node-a")
	if !exists {
		t.Error("follow not saved")
	}
	if len(fedRepo.events) != 0 {
		t.Errorf("expected 0 federation events for local follow, got %d", len(fedRepo.events))
	}
}

// — Follow — удалённый с auto-discovery ──────────────────────────────

func TestFollow_Remote_DiscoversNode(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	nodeRepo := newMockNodeRegistry()
	disc := &mockDiscoverer{results: map[string][2]string{
		"node-b": {"node-b", "http://node-b:8080"},
	}}
	svc := newSvc(followRepo, userRepo, fedRepo, nodeRepo, disc)

	if err := svc.Follow(context.Background(), "alice@node-a", "carol@node-b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Узел был сохранён через discovery
	if _, ok := nodeRepo.nodes["node-b"]; !ok {
		t.Error("node-b should be saved after discovery")
	}

	// Federation событие отправлено
	if len(fedRepo.events) != 1 {
		t.Fatalf("expected 1 federation event, got %d", len(fedRepo.events))
	}
	if fedRepo.events[0].EventType != "user.followed" {
		t.Errorf("event_type = %q, want user.followed", fedRepo.events[0].EventType)
	}
	if fedRepo.events[0].TargetNode != "node-b" {
		t.Errorf("target_node = %q, want node-b", fedRepo.events[0].TargetNode)
	}
}

func TestFollow_Remote_NodeAlreadyKnown_NoDiscovery(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	nodeRepo := newMockNodeRegistry()
	// node-b уже известен
	nodeRepo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}

	// Discoverer вернёт ошибку — но он не должен вызываться
	disc := &mockDiscoverer{err: errors.New("should not be called")}
	svc := newSvc(followRepo, userRepo, fedRepo, nodeRepo, disc)

	err := svc.Follow(context.Background(), "alice@node-a", "carol@node-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Всё равно отправили federation событие
	if len(fedRepo.events) != 1 {
		t.Errorf("expected 1 federation event, got %d", len(fedRepo.events))
	}
}

func TestFollow_Remote_DiscoveryFails_ReturnsError(t *testing.T) {
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	disc := &mockDiscoverer{err: errors.New("connection refused")}
	svc := newSvc(newMockFollowRepo(), userRepo, &mockFedEnqueuer{}, newMockNodeRegistry(), disc)

	err := svc.Follow(context.Background(), "alice@node-a", "carol@unknown-node")
	if err == nil {
		t.Error("expected error when discovery fails")
	}
}

// — Follow — ошибки ───────────────────────────────────────────────────

func TestFollow_SelfFollow(t *testing.T) {
	svc := newSvc(newMockFollowRepo(), &mockUserLookup{}, &mockFedEnqueuer{}, newMockNodeRegistry(), &mockDiscoverer{})

	err := svc.Follow(context.Background(), "alice@node-a", "alice@node-a")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "self_follow" {
		t.Errorf("expected self_follow AppError, got %v", err)
	}
}

func TestFollow_UserNotFound(t *testing.T) {
	svc := newSvc(newMockFollowRepo(), &mockUserLookup{uuids: map[string]string{}}, &mockFedEnqueuer{}, newMockNodeRegistry(), &mockDiscoverer{})

	err := svc.Follow(context.Background(), "nobody@node-a", "bob@node-a")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestFollow_AlreadyFollowing(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newSvc(followRepo, userRepo, &mockFedEnqueuer{}, newMockNodeRegistry(), &mockDiscoverer{})

	_ = svc.Follow(context.Background(), "alice@node-a", "bob@node-a")
	err := svc.Follow(context.Background(), "alice@node-a", "bob@node-a")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "already_following" {
		t.Errorf("expected already_following, got %v", err)
	}
}

// — Unfollow ──────────────────────────────────────────────────────────

func TestUnfollow_Success(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newSvc(followRepo, userRepo, &mockFedEnqueuer{}, newMockNodeRegistry(), &mockDiscoverer{})

	_ = svc.Follow(context.Background(), "alice@node-a", "bob@node-a")
	if err := svc.Unfollow(context.Background(), "alice@node-a", "bob@node-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exists, _ := followRepo.Exists(context.Background(), "uuid-alice", "bob@node-a")
	if exists {
		t.Error("follow should be deleted")
	}
}

func TestUnfollow_NotFollowing(t *testing.T) {
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newSvc(newMockFollowRepo(), userRepo, &mockFedEnqueuer{}, newMockNodeRegistry(), &mockDiscoverer{})

	err := svc.Unfollow(context.Background(), "alice@node-a", "bob@node-a")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "not_following" {
		t.Errorf("expected not_following, got %v", err)
	}
}

// — payload содержит follower_base_url ────────────────────────────────

func TestFollow_Remote_PayloadContainsBaseURL(t *testing.T) {
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	nodeRepo := newMockNodeRegistry()
	nodeRepo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}
	svc := newSvc(newMockFollowRepo(), userRepo, fedRepo, nodeRepo, &mockDiscoverer{})

	_ = svc.Follow(context.Background(), "alice@node-a", "carol@node-b")

	if len(fedRepo.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(fedRepo.events))
	}
	payload := fedRepo.events[0].Payload
	baseURL, ok := payload["follower_base_url"].(string)
	if !ok || baseURL != "http://node-a:8080" {
		t.Errorf("follower_base_url = %v, want http://node-a:8080", payload["follower_base_url"])
	}
}
