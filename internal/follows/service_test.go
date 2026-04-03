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

// — Хелперы ──────────────────────────────────────────────────────────

func followTestCfg() *config.Config {
	return &config.Config{NodeName: "node-a"}
}

func newSvc(fr *mockFollowRepo, ur *mockUserLookup, fe *mockFedEnqueuer) *follows.Service {
	return follows.NewService(fr, ur, fe, logger.Nop(), followTestCfg())
}

// — Follow ────────────────────────────────────────────────────────────

func TestFollow_Success_Local(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	svc := newSvc(followRepo, userRepo, fedRepo)

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

func TestFollow_Success_Remote_SendsFederationEvent(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	fedRepo := &mockFedEnqueuer{}
	svc := newSvc(followRepo, userRepo, fedRepo)

	if err := svc.Follow(context.Background(), "alice@node-a", "carol@node-b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fedRepo.events) != 1 {
		t.Fatalf("expected 1 federation event, got %d", len(fedRepo.events))
	}
	e := fedRepo.events[0]
	if e.EventType != "user.followed" {
		t.Errorf("event_type = %q, want user.followed", e.EventType)
	}
	if e.TargetNode != "node-b" {
		t.Errorf("target_node = %q, want node-b", e.TargetNode)
	}
}

func TestFollow_SelfFollow(t *testing.T) {
	svc := newSvc(newMockFollowRepo(), &mockUserLookup{}, &mockFedEnqueuer{})

	err := svc.Follow(context.Background(), "alice@node-a", "alice@node-a")
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "self_follow" {
		t.Errorf("expected self_follow AppError, got %v", err)
	}
}

func TestFollow_UserNotFound(t *testing.T) {
	svc := newSvc(newMockFollowRepo(), &mockUserLookup{uuids: map[string]string{}}, &mockFedEnqueuer{})

	err := svc.Follow(context.Background(), "nobody@node-a", "bob@node-a")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestFollow_AlreadyFollowing(t *testing.T) {
	followRepo := newMockFollowRepo()
	userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
	svc := newSvc(followRepo, userRepo, &mockFedEnqueuer{})

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
	svc := newSvc(followRepo, userRepo, &mockFedEnqueuer{})

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
	svc := newSvc(newMockFollowRepo(), userRepo, &mockFedEnqueuer{})

	err := svc.Unfollow(context.Background(), "alice@node-a", "bob@node-a")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "not_following" {
		t.Errorf("expected not_following, got %v", err)
	}
}

func TestUnfollow_UserNotFound(t *testing.T) {
	svc := newSvc(newMockFollowRepo(), &mockUserLookup{uuids: map[string]string{}}, &mockFedEnqueuer{})

	err := svc.Unfollow(context.Background(), "nobody@node-a", "bob@node-a")
	if !errors.Is(err, apperr.ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

// — Табличный тест: локальный vs удалённый узел ───────────────────────

func TestFollow_NodeDetection(t *testing.T) {
	tests := []struct {
		target       string
		wantFedEvent bool
		wantNode     string
	}{
		{"bob@node-a", false, ""},      // локальный — нет события
		{"bob@node-b", true, "node-b"}, // удалённый — есть событие
		{"bob@node-c", true, "node-c"}, // другой удалённый
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			userRepo := &mockUserLookup{uuids: map[string]string{"alice@node-a": "uuid-alice"}}
			fedRepo := &mockFedEnqueuer{}
			svc := newSvc(newMockFollowRepo(), userRepo, fedRepo)

			if err := svc.Follow(context.Background(), "alice@node-a", tt.target); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			hasFed := len(fedRepo.events) > 0
			if hasFed != tt.wantFedEvent {
				t.Errorf("federation event = %v, want %v", hasFed, tt.wantFedEvent)
			}
			if tt.wantFedEvent && fedRepo.events[0].TargetNode != tt.wantNode {
				t.Errorf("target_node = %q, want %q", fedRepo.events[0].TargetNode, tt.wantNode)
			}
		})
	}
}
