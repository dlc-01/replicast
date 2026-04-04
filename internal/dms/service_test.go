package dms_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/dms"
	"github.com/dlc-01/replicast/internal/logger"
	"github.com/dlc-01/replicast/internal/port"
)

type mockDMRepo struct {
	conversations map[string]*port.Conversation
	messages      map[string][]port.Message
	uuids         map[string]string
	nodes         map[string]string
}

func newMockDMRepo() *mockDMRepo {
	return &mockDMRepo{
		conversations: make(map[string]*port.Conversation),
		messages:      make(map[string][]port.Message),
		uuids:         map[string]string{"alice@node-a": "uuid-alice", "bob@node-b": "uuid-bob"},
		nodes:         map[string]string{"alice@node-a": "node-a", "bob@node-b": "node-b"},
	}
}

func (m *mockDMRepo) GetOrCreateConversation(_ context.Context, a, b string) (*port.Conversation, error) {
	key := a + ":" + b
	if c, ok := m.conversations[key]; ok {
		return c, nil
	}
	c := &port.Conversation{
		ID:           "conv-" + a + "-" + b,
		ParticipantA: a,
		ParticipantB: b,
		CreatedAt:    time.Now(),
	}
	m.conversations[key] = c
	return c, nil
}
func (m *mockDMRepo) GetConversation(_ context.Context, id string) (*port.Conversation, error) {
	for _, c := range m.conversations {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, apperr.NotFound("conversation_not_found", "not found")
}
func (m *mockDMRepo) ListConversations(_ context.Context, userGlobalID string) ([]port.Conversation, error) {
	var out []port.Conversation
	for _, c := range m.conversations {
		if c.ParticipantA == userGlobalID || c.ParticipantB == userGlobalID {
			out = append(out, *c)
		}
	}
	return out, nil
}
func (m *mockDMRepo) CreateMessage(_ context.Context, msg port.Message) error {
	m.messages[msg.ConversationID] = append(m.messages[msg.ConversationID], msg)
	return nil
}
func (m *mockDMRepo) GetMessages(_ context.Context, conversationID string, _ int) ([]port.Message, error) {
	msgs := m.messages[conversationID]
	if msgs == nil {
		return []port.Message{}, nil
	}
	return msgs, nil
}
func (m *mockDMRepo) GetUUIDByGlobalID(_ context.Context, globalID string) (string, error) {
	if id, ok := m.uuids[globalID]; ok {
		return id, nil
	}
	return "", errors.New("user not found")
}
func (m *mockDMRepo) GetNodeByGlobalID(_ context.Context, globalID string) (string, error) {
	if node, ok := m.nodes[globalID]; ok {
		return node, nil
	}
	return "", errors.New("user not found")
}

type mockDMFed struct{ events []port.OutboxEvent }

func (m *mockDMFed) EnqueueEvent(_ context.Context, e port.OutboxEvent) error {
	m.events = append(m.events, e)
	return nil
}

func newDMSvc(repo *mockDMRepo, fed *mockDMFed) *dms.Service {
	cfg := &config.Config{NodeName: "node-a"}
	return dms.NewService(repo, fed, logger.Nop(), cfg)
}

// — Тесты StartConversation ───────────────────────────────────────────

func TestDMService_StartConversation_Success(t *testing.T) {
	repo := newMockDMRepo()
	svc := newDMSvc(repo, &mockDMFed{})

	conv, err := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conv.ID == "" {
		t.Error("conversation ID should not be empty")
	}
}

func TestDMService_StartConversation_SelfMessage(t *testing.T) {
	svc := newDMSvc(newMockDMRepo(), &mockDMFed{})

	_, err := svc.StartConversation(context.Background(), "alice@node-a", "alice@node-a")
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "self_message" {
		t.Errorf("expected self_message error, got %v", err)
	}
}

func TestDMService_StartConversation_Idempotent(t *testing.T) {
	repo := newMockDMRepo()
	svc := newDMSvc(repo, &mockDMFed{})

	conv1, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	conv2, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")

	if conv1.ID != conv2.ID {
		t.Error("same participants should return same conversation")
	}
}

// — Тесты SendMessage ─────────────────────────────────────────────────

func TestDMService_SendMessage_Success(t *testing.T) {
	repo := newMockDMRepo()
	svc := newDMSvc(repo, &mockDMFed{})

	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	msg, err := svc.SendMessage(context.Background(), "alice@node-a", conv.ID, "hello bob!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "hello bob!" {
		t.Errorf("content = %q, want hello bob!", msg.Content)
	}
}

func TestDMService_SendMessage_RemoteRecipient_SendsEvent(t *testing.T) {
	repo := newMockDMRepo()
	fed := &mockDMFed{}
	svc := newDMSvc(repo, fed)

	// alice@node-a пишет bob@node-b (другой узел)
	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	_, err := svc.SendMessage(context.Background(), "alice@node-a", conv.ID, "hello!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fed.events) != 1 {
		t.Fatalf("expected 1 federation event, got %d", len(fed.events))
	}
	if fed.events[0].EventType != "message.sent" {
		t.Errorf("event_type = %q, want message.sent", fed.events[0].EventType)
	}
	if fed.events[0].TargetNode != "node-b" {
		t.Errorf("target_node = %q, want node-b", fed.events[0].TargetNode)
	}
}

func TestDMService_SendMessage_NotParticipant(t *testing.T) {
	repo := newMockDMRepo()
	repo.uuids["carol@node-a"] = "uuid-carol"
	svc := newDMSvc(repo, &mockDMFed{})

	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	_, err := svc.SendMessage(context.Background(), "carol@node-a", conv.ID, "intruder!")

	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "not_participant" {
		t.Errorf("expected not_participant error, got %v", err)
	}
}

// — Тесты GetMessages ─────────────────────────────────────────────────

func TestDMService_GetMessages_Success(t *testing.T) {
	repo := newMockDMRepo()
	svc := newDMSvc(repo, &mockDMFed{})

	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	_, _ = svc.SendMessage(context.Background(), "alice@node-a", conv.ID, "msg 1")
	_, _ = svc.SendMessage(context.Background(), "alice@node-a", conv.ID, "msg 2")

	msgs, err := svc.GetMessages(context.Background(), "alice@node-a", conv.ID, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("len = %d, want 2", len(msgs))
	}
}

func TestDMService_GetMessages_NotParticipant(t *testing.T) {
	repo := newMockDMRepo()
	repo.uuids["carol@node-a"] = "uuid-carol"
	svc := newDMSvc(repo, &mockDMFed{})

	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")

	_, err := svc.GetMessages(context.Background(), "carol@node-a", conv.ID, 10)
	var appErr *apperr.AppError
	if !errors.As(err, &appErr) || appErr.Code != "not_participant" {
		t.Errorf("expected not_participant error, got %v", err)
	}
}

// — Тесты ListConversations ───────────────────────────────────────────

func TestDMService_ListConversations(t *testing.T) {
	repo := newMockDMRepo()
	repo.uuids["carol@node-a"] = "uuid-carol"
	svc := newDMSvc(repo, &mockDMFed{})

	_, _ = svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	_, _ = svc.StartConversation(context.Background(), "alice@node-a", "carol@node-a")

	convs, err := svc.ListConversations(context.Background(), "alice@node-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(convs) != 2 {
		t.Errorf("len = %d, want 2", len(convs))
	}
}

func TestDMService_MessageGlobalIDFormat(t *testing.T) {
	repo := newMockDMRepo()
	svc := newDMSvc(repo, &mockDMFed{})

	conv, _ := svc.StartConversation(context.Background(), "alice@node-a", "bob@node-b")
	msg, _ := svc.SendMessage(context.Background(), "alice@node-a", conv.ID, "test")

	if len(msg.GlobalID) < 10 || msg.GlobalID[:4] != "msg:" {
		t.Errorf("global_id = %q, should start with msg:", msg.GlobalID)
	}
}
