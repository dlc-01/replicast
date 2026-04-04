package comments_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/comments"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type mockCommentRepo struct {
	data  map[string]*port.Comment
	uuids map[string]string
}

func newMockCommentRepo() *mockCommentRepo {
	return &mockCommentRepo{
		data:  make(map[string]*port.Comment),
		uuids: map[string]string{"alice@node-a": "uuid-alice", "bob@node-b": "uuid-bob"},
	}
}

func (m *mockCommentRepo) Create(_ context.Context, c port.Comment) error {
	m.data[c.GlobalID] = &c
	return nil
}
func (m *mockCommentRepo) GetByPost(_ context.Context, postGlobalID string, _ int) ([]port.Comment, error) {
	var out []port.Comment
	for _, c := range m.data {
		if c.PostGlobalID == postGlobalID && c.Status != "deleted" {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (m *mockCommentRepo) GetByGlobalID(_ context.Context, globalID string) (*port.Comment, error) {
	c, ok := m.data[globalID]
	if !ok {
		return nil, apperr.NotFound("comment_not_found", "not found")
	}
	return c, nil
}
func (m *mockCommentRepo) Delete(_ context.Context, globalID string) error {
	if c, ok := m.data[globalID]; ok {
		c.Status = "deleted"
	}
	return nil
}
func (m *mockCommentRepo) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	if id, ok := m.uuids[globalID]; ok {
		return id, nil
	}
	return "", errors.New("user not found")
}

type mockCommentFed struct{ events []port.OutboxEvent }

func (m *mockCommentFed) EnqueueEvent(_ context.Context, e port.OutboxEvent) error {
	m.events = append(m.events, e)
	return nil
}

type mockCommentPostGetter struct{ posts map[string]*port.Post }

func (m *mockCommentPostGetter) GetByGlobalID(_ context.Context, globalID string) (*port.Post, error) {
	return m.posts[globalID], nil
}

func newCommentSvc(repo *mockCommentRepo, fed *mockCommentFed, posts *mockCommentPostGetter) *comments.Service {
	cfg := &config.Config{NodeName: "node-a"}
	return comments.NewService(repo, fed, posts, logger.Nop(), cfg)
}

func TestCommentService_Create_LocalPost(t *testing.T) {
	repo := newMockCommentRepo()
	fed := &mockCommentFed{}
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newCommentSvc(repo, fed, posts)

	c, err := svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "hello!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Content != "hello!" {
		t.Errorf("content = %q, want hello!", c.Content)
	}
	// Локальный пост — нет federation события
	if len(fed.events) != 0 {
		t.Errorf("expected 0 federation events, got %d", len(fed.events))
	}
}

func TestCommentService_Create_RemotePost_SendsEvent(t *testing.T) {
	repo := newMockCommentRepo()
	fed := &mockCommentFed{}
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:carol@node-c:001": {GlobalID: "post:carol@node-c:001", OriginNode: "node-c"},
	}}
	svc := newCommentSvc(repo, fed, posts)

	_, err := svc.Create(context.Background(), "alice@node-a", "post:carol@node-c:001", "nice post!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fed.events) != 1 {
		t.Fatalf("expected 1 federation event, got %d", len(fed.events))
	}
	if fed.events[0].EventType != "comment.created" {
		t.Errorf("event_type = %q, want comment.created", fed.events[0].EventType)
	}
	if fed.events[0].TargetNode != "node-c" {
		t.Errorf("target_node = %q, want node-c", fed.events[0].TargetNode)
	}
}

func TestCommentService_GetByPost(t *testing.T) {
	repo := newMockCommentRepo()
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newCommentSvc(repo, &mockCommentFed{}, posts)

	_, _ = svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "first")
	_, _ = svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "second")

	items, err := svc.GetByPost(context.Background(), "post:alice@node-a:001", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len = %d, want 2", len(items))
	}
}

func TestCommentService_Delete_Success(t *testing.T) {
	repo := newMockCommentRepo()
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newCommentSvc(repo, &mockCommentFed{}, posts)

	c, _ := svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "delete me")
	if err := svc.Delete(context.Background(), c.GlobalID, "alice@node-a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, _ := svc.GetByPost(context.Background(), "post:alice@node-a:001", 10)
	if len(items) != 0 {
		t.Errorf("expected 0 comments after delete, got %d", len(items))
	}
}

func TestCommentService_Delete_Forbidden(t *testing.T) {
	repo := newMockCommentRepo()
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newCommentSvc(repo, &mockCommentFed{}, posts)

	c, _ := svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "alice's comment")

	// Bob пытается удалить комментарий Alice
	err := svc.Delete(context.Background(), c.GlobalID, "bob@node-b")
	if !errors.Is(err, apperr.ErrPostForbidden) {
		t.Errorf("expected ErrPostForbidden, got %v", err)
	}
}

func TestCommentService_GlobalIDFormat(t *testing.T) {
	repo := newMockCommentRepo()
	posts := &mockCommentPostGetter{posts: map[string]*port.Post{
		"post:alice@node-a:001": {GlobalID: "post:alice@node-a:001", OriginNode: "node-a"},
	}}
	svc := newCommentSvc(repo, &mockCommentFed{}, posts)

	c, _ := svc.Create(context.Background(), "alice@node-a", "post:alice@node-a:001", "test")

	// global_id должен начинаться с "comment:alice@node-a:"
	prefix := "comment:alice@node-a:"
	if len(c.GlobalID) < len(prefix) || c.GlobalID[:len(prefix)] != prefix {
		t.Errorf("global_id = %q, should start with %s", c.GlobalID, prefix)
	}
}
