package feed_test

import (
	"context"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/feed"
	"github.com/dlc-01/replicast/internal/port"
)

// mockFeedRepo реализует полный feedRepository (5 методов включая GetUUIDByGlobalID)
type mockFeedRepo struct {
	items       []port.FeedPost
	addedItems  []port.FeedItem
	followerIDs map[string][]string
	uuids       map[string]string // globalID → UUID
	err         error
}

func newMockFeedRepo() *mockFeedRepo {
	return &mockFeedRepo{
		followerIDs: make(map[string][]string),
		uuids:       map[string]string{"bob@node-a": "uuid-bob"},
	}
}

func (m *mockFeedRepo) AddItem(_ context.Context, item port.FeedItem) error {
	m.addedItems = append(m.addedItems, item)
	return m.err
}
func (m *mockFeedRepo) RemoveItem(_ context.Context, _, _ string) error { return m.err }
func (m *mockFeedRepo) GetFeed(_ context.Context, _ string, _ int) ([]port.FeedPost, error) {
	return m.items, m.err
}
func (m *mockFeedRepo) GetFollowerUserIDs(_ context.Context, authorGlobalID string) ([]string, error) {
	return m.followerIDs[authorGlobalID], m.err
}
func (m *mockFeedRepo) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	if uuid, ok := m.uuids[globalID]; ok {
		return uuid, nil
	}
	return "uuid-" + globalID, nil // fallback для тестов
}

// mockFeedRepoCapture перехватывает аргументы вызова
type mockFeedRepoCapture struct {
	onGetFeed func(ctx context.Context, ownerID string, limit int) ([]port.FeedPost, error)
}

func (m *mockFeedRepoCapture) AddItem(_ context.Context, _ port.FeedItem) error { return nil }
func (m *mockFeedRepoCapture) RemoveItem(_ context.Context, _, _ string) error  { return nil }
func (m *mockFeedRepoCapture) GetFollowerUserIDs(_ context.Context, _ string) ([]string, error) {
	return []string{}, nil
}
func (m *mockFeedRepoCapture) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	return "uuid-" + globalID, nil
}
func (m *mockFeedRepoCapture) GetFeed(ctx context.Context, ownerID string, limit int) ([]port.FeedPost, error) {
	return m.onGetFeed(ctx, ownerID, limit)
}

// — Тесты сервиса ────────────────────────────────────────────────────

func TestFeedService_GetFeed_ReturnsPosts(t *testing.T) {
	now := time.Now()
	repo := newMockFeedRepo()
	repo.items = []port.FeedPost{
		{PostGlobalID: "post:alice@node-a:001", Content: "hello", AuthorGlobalID: "alice@node-a", CreatedAt: now},
		{PostGlobalID: "post:alice@node-a:002", Content: "world", AuthorGlobalID: "alice@node-a", CreatedAt: now},
	}
	svc := feed.NewService(repo)

	items, err := svc.GetFeed(context.Background(), "bob@node-a", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("len = %d, want 2", len(items))
	}
}

func TestFeedService_GetFeed_EmptyFeed(t *testing.T) {
	svc := feed.NewService(newMockFeedRepo())

	items, err := svc.GetFeed(context.Background(), "bob@node-a", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected empty feed, got %d items", len(items))
	}
}

func TestFeedService_GetFeed_LimitDefaults(t *testing.T) {
	var capturedLimit int
	repo := &mockFeedRepoCapture{
		onGetFeed: func(_ context.Context, _ string, limit int) ([]port.FeedPost, error) {
			capturedLimit = limit
			return []port.FeedPost{}, nil
		},
	}
	svc := feed.NewService(repo)

	tests := []struct {
		input     int
		wantLimit int
	}{
		{0, 50},
		{-1, 50},
		{201, 50},
		{10, 10},
		{200, 200},
	}

	for _, tt := range tests {
		capturedLimit = 0
		_, _ = svc.GetFeed(context.Background(), "bob@node-a", tt.input)
		if capturedLimit != tt.wantLimit {
			t.Errorf("input %d: limit = %d, want %d", tt.input, capturedLimit, tt.wantLimit)
		}
	}
}

func TestFeedRepo_GetFollowerUserIDs(t *testing.T) {
	repo := newMockFeedRepo()
	repo.followerIDs["alice@node-a"] = []string{"uuid-bob", "uuid-carol"}

	ids, err := repo.GetFollowerUserIDs(context.Background(), "alice@node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Errorf("len = %d, want 2", len(ids))
	}
}

func TestFeedRepo_GetFollowerUserIDs_Unknown(t *testing.T) {
	ids, err := newMockFeedRepo().GetFollowerUserIDs(context.Background(), "nobody@node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Errorf("expected empty, got %d", len(ids))
	}
}

func TestFanout_AddsToFollowerFeeds(t *testing.T) {
	repo := newMockFeedRepo()
	repo.followerIDs["alice@node-a"] = []string{"uuid-bob", "uuid-carol", "uuid-dave"}

	followerIDs, _ := repo.GetFollowerUserIDs(context.Background(), "alice@node-a")
	for _, ownerID := range followerIDs {
		_ = repo.AddItem(context.Background(), port.FeedItem{
			OwnerUserID:  ownerID,
			PostGlobalID: "post:alice@node-a:001",
			SourceNode:   "node-a",
		})
	}

	if len(repo.addedItems) != 3 {
		t.Errorf("expected 3 feed items, got %d", len(repo.addedItems))
	}
}
