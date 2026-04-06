package likes_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/likes"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type mockLikeRepo struct {
	liked     map[string]map[string]bool
	uuids     map[string]string
	hideLikes bool
}

func newMockLikeRepo() *mockLikeRepo {
	return &mockLikeRepo{
		liked: make(map[string]map[string]bool),
		uuids: map[string]string{"alice@node-a": "uuid-alice", "bob@node-b": "uuid-bob"},
	}
}

func (m *mockLikeRepo) Like(_ context.Context, userID, postGlobalID string) error {
	if m.liked[postGlobalID] == nil {
		m.liked[postGlobalID] = make(map[string]bool)
	}
	m.liked[postGlobalID][userID] = true
	return nil
}
func (m *mockLikeRepo) Unlike(_ context.Context, userID, postGlobalID string) error {
	if m.liked[postGlobalID] != nil {
		delete(m.liked[postGlobalID], userID)
	}
	return nil
}
func (m *mockLikeRepo) Count(_ context.Context, postGlobalID string) (int, error) {
	return len(m.liked[postGlobalID]), nil
}
func (m *mockLikeRepo) IsLiked(_ context.Context, userID, postGlobalID string) (bool, error) {
	return m.liked[postGlobalID][userID], nil
}
func (m *mockLikeRepo) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	if id, ok := m.uuids[globalID]; ok {
		return id, nil
	}
	return "", errors.New("user not found")
}

type mockLikeFedEnqueuer struct{ events []port.OutboxEvent }

func (m *mockLikeFedEnqueuer) EnqueueEvent(_ context.Context, e port.OutboxEvent) error {
	m.events = append(m.events, e)
	return nil
}

type mockLikePostGetter struct{ posts map[string]*port.Post }

func (m *mockLikePostGetter) GetByGlobalID(_ context.Context, globalID string) (*port.Post, error) {
	return m.posts[globalID], nil
}

func newLikeSvc(repo *mockLikeRepo, fed *mockLikeFedEnqueuer, posts *mockLikePostGetter) *likes.Service {
	cfg := &config.Config{NodeName: "node-a"}
	return likes.NewService(repo, fed, posts, logger.Nop(), cfg)
}

func TestLikeService_Like_Success(t *testing.T) {
	repo := newMockLikeRepo()
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newLikeSvc(repo, fed, posts)

	if err := svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	count, _ := svc.GetCount(context.Background(), "post:alice@node-a:001")
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestLikeService_Like_RemotePost_SendsEvent(t *testing.T) {
	repo := newMockLikeRepo()
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:carol@node-c:001": {GlobalID: "post:carol@node-c:001", OriginNode: "node-c"},
	}}
	svc := newLikeSvc(repo, fed, posts)

	_ = svc.Like(context.Background(), "alice@node-a", "post:carol@node-c:001")

	if len(fed.events) != 1 {
		t.Fatalf("expected 1 federation event, got %d", len(fed.events))
	}
	if fed.events[0].EventType != "post.liked" {
		t.Errorf("event_type = %q, want post.liked", fed.events[0].EventType)
	}
	if fed.events[0].TargetNode != "node-c" {
		t.Errorf("target_node = %q, want node-c", fed.events[0].TargetNode)
	}
}

func TestLikeService_Like_LocalPost_NoEvent(t *testing.T) {
	repo := newMockLikeRepo()
	fed := &mockLikeFedEnqueuer{}
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newLikeSvc(repo, fed, posts)

	_ = svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001")

	if len(fed.events) != 0 {
		t.Errorf("expected 0 federation events for local post, got %d", len(fed.events))
	}
}

func TestLikeService_Unlike_Success(t *testing.T) {
	repo := newMockLikeRepo()
	svc := newLikeSvc(repo, &mockLikeFedEnqueuer{}, &mockLikePostGetter{posts: map[string]*port.Post{}})

	_ = svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001")
	_ = svc.Unlike(context.Background(), "alice@node-a", "post:alice@node-a:001")

	count, _ := svc.GetCount(context.Background(), "post:alice@node-a:001")
	if count != 0 {
		t.Errorf("count = %d, want 0 after unlike", count)
	}
}

func TestLikeService_IsLiked(t *testing.T) {
	repo := newMockLikeRepo()
	svc := newLikeSvc(repo, &mockLikeFedEnqueuer{}, &mockLikePostGetter{posts: map[string]*port.Post{}})

	liked, _ := svc.IsLiked(context.Background(), "alice@node-a", "post:alice@node-a:001")
	if liked {
		t.Error("should not be liked before liking")
	}

	_ = svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001")

	liked, _ = svc.IsLiked(context.Background(), "alice@node-a", "post:alice@node-a:001")
	if !liked {
		t.Error("should be liked after liking")
	}
}

func TestLikeService_UserNotFound(t *testing.T) {
	repo := newMockLikeRepo()
	svc := newLikeSvc(repo, &mockLikeFedEnqueuer{}, &mockLikePostGetter{posts: map[string]*port.Post{}})

	err := svc.Like(context.Background(), "nobody@node-x", "post:alice@node-a:001")
	if err == nil {
		t.Error("expected error for unknown user")
	}
}

// — Тесты GetLikers ───────────────────────────────────────────────────

func (m *mockLikeRepo) GetLikers(_ context.Context, postGlobalID string) ([]likes.LikerInfo, error) {
	var out []likes.LikerInfo
	for userID := range m.liked[postGlobalID] {
		out = append(out, likes.LikerInfo{GlobalID: userID, DisplayName: ""})
	}
	return out, nil
}

func (m *mockLikeRepo) GetPostHideLikes(_ context.Context, _ string) (bool, error) {
	return m.hideLikes, nil
}

func TestLikeService_GetLikers_Visible(t *testing.T) {
	repo := newMockLikeRepo()
	repo.hideLikes = false
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newLikeSvc(repo, &mockLikeFedEnqueuer{}, posts)

	_ = svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001")
	_ = svc.Like(context.Background(), "bob@node-b", "post:alice@node-a:001")

	likers, hidden, err := svc.GetLikers(context.Background(), "post:alice@node-a:001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hidden {
		t.Error("expected hidden=false")
	}
	if len(likers) != 2 {
		t.Errorf("likers = %d, want 2", len(likers))
	}
}

func TestLikeService_GetLikers_Hidden(t *testing.T) {
	repo := newMockLikeRepo()
	repo.hideLikes = true
	posts := &mockLikePostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newLikeSvc(repo, &mockLikeFedEnqueuer{}, posts)

	_ = svc.Like(context.Background(), "alice@node-a", "post:alice@node-a:001")

	likers, hidden, err := svc.GetLikers(context.Background(), "post:alice@node-a:001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hidden {
		t.Error("expected hidden=true")
	}
	if likers != nil {
		t.Error("likers should be nil when hidden")
	}
}
