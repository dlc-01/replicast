package worker_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/worker"
)

// — Моки ─────────────────────────────────────────────────────────────

type mockOutboxRepo struct {
	events    []port.OutboxRow
	nodes     map[string]*port.Node
	delivered []string
	failed    []string
}

func newMockOutboxRepo() *mockOutboxRepo {
	return &mockOutboxRepo{nodes: make(map[string]*port.Node)}
}

func (m *mockOutboxRepo) GetPendingEvents(_ context.Context, _ int) ([]port.OutboxRow, error) {
	return m.events, nil
}

func (m *mockOutboxRepo) MarkDelivered(_ context.Context, id string) error {
	m.delivered = append(m.delivered, id)
	return nil
}

func (m *mockOutboxRepo) MarkFailed(_ context.Context, id string, _ int) error {
	m.failed = append(m.failed, id)
	// Убираем из очереди чтобы не переотправлялось
	var remaining []port.OutboxRow
	for _, e := range m.events {
		if e.ID != id {
			remaining = append(remaining, e)
		}
	}
	m.events = remaining
	return nil
}

func (m *mockOutboxRepo) GetNodeByName(_ context.Context, name string) (*port.Node, error) {
	n, ok := m.nodes[name]
	if !ok {
		return nil, &nodeNotFoundErr{name: name}
	}
	return n, nil
}

type nodeNotFoundErr struct{ name string }

func (e *nodeNotFoundErr) Error() string { return "node not found: " + e.name }

// mockSender перехватывает отправленные события.
type mockSender struct {
	sent []federation.EventPayload
	err  error
}

func (m *mockSender) SendEvent(_ context.Context, _, _ string, e federation.EventPayload) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, e)
	return nil
}

// — Хелпер ────────────────────────────────────────────────────────────

func newWorker(repo *mockOutboxRepo, sender *mockSender) *worker.OutboxWorker {
	cfg := &config.Config{
		NodeName:       "node-a",
		OutboxInterval: 100 * time.Millisecond,
	}
	return worker.NewOutboxWorker(repo, sender, cfg)
}

func makeEvent(id, targetNode, eventType string) port.OutboxRow {
	payload, _ := json.Marshal(map[string]string{"key": "value"})
	return port.OutboxRow{
		ID:         id,
		EventID:    "evt-" + id,
		TargetNode: targetNode,
		EventType:  eventType,
		Payload:    payload,
		Status:     "pending",
		RetryCount: 0,
	}
}

// — Тесты ─────────────────────────────────────────────────────────────

func TestOutboxWorker_DeliverySuccess(t *testing.T) {
	repo := newMockOutboxRepo()
	repo.events = []port.OutboxRow{makeEvent("1", "node-b", "post.created")}
	repo.nodes["node-b"] = &port.Node{
		Name:         "node-b",
		BaseURL:      "http://node-b:8080",
		SharedSecret: "secret",
	}
	sender := &mockSender{}

	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(repo, sender)

	// Запускаем один тик вручную
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].EventType != "post.created" {
		t.Errorf("event_type = %q, want post.created", sender.sent[0].EventType)
	}
	if len(repo.delivered) != 1 || repo.delivered[0] != "1" {
		t.Errorf("delivered = %v, want [1]", repo.delivered)
	}
	if len(repo.failed) != 0 {
		t.Errorf("expected 0 failed, got %v", repo.failed)
	}
}

func TestOutboxWorker_DeliveryFailure_MarkedFailed(t *testing.T) {
	repo := newMockOutboxRepo()
	repo.events = []port.OutboxRow{makeEvent("1", "node-b", "post.created")}
	repo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}
	sender := &mockSender{err: errors.New("connection refused")}

	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(repo, sender)

	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(repo.delivered) != 0 {
		t.Errorf("expected 0 delivered, got %d", len(repo.delivered))
	}
	if len(repo.failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(repo.failed))
	}
}

func TestOutboxWorker_UnknownNode_MarkedFailed(t *testing.T) {
	repo := newMockOutboxRepo()
	repo.events = []port.OutboxRow{makeEvent("1", "unknown-node", "post.created")}
	// Узел не добавляем в nodes map
	sender := &mockSender{}

	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(repo, sender)

	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(sender.sent) != 0 {
		t.Errorf("expected 0 sent for unknown node, got %d", len(sender.sent))
	}
	if len(repo.failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(repo.failed))
	}
}

func TestOutboxWorker_EmptyOutbox_NoOp(t *testing.T) {
	repo := newMockOutboxRepo() // events пустой
	sender := &mockSender{}

	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(repo, sender)

	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(sender.sent) != 0 {
		t.Errorf("expected 0 sent, got %d", len(sender.sent))
	}
}

func TestOutboxWorker_MultipleEvents_AllDelivered(t *testing.T) {
	repo := newMockOutboxRepo()
	repo.events = []port.OutboxRow{
		makeEvent("1", "node-b", "post.created"),
		makeEvent("2", "node-b", "post.updated"),
		makeEvent("3", "node-c", "user.followed"),
	}
	repo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}
	repo.nodes["node-c"] = &port.Node{Name: "node-c", BaseURL: "http://node-c:8080"}
	sender := &mockSender{}

	ctx, cancel := context.WithCancel(context.Background())
	w := newWorker(repo, sender)

	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(sender.sent) != 3 {
		t.Errorf("sent = %d, want 3", len(sender.sent))
	}
	if len(repo.delivered) != 3 {
		t.Errorf("delivered = %d, want 3", len(repo.delivered))
	}
}

func TestOutboxWorker_SourceNodeInEvent(t *testing.T) {
	repo := newMockOutboxRepo()
	repo.events = []port.OutboxRow{makeEvent("1", "node-b", "post.created")}
	repo.nodes["node-b"] = &port.Node{Name: "node-b", BaseURL: "http://node-b:8080"}
	sender := &mockSender{}

	cfg := &config.Config{
		NodeName:       "node-a",
		OutboxInterval: 200 * time.Millisecond, // интервал больше таймаута — ровно один тик
	}
	w := worker.NewOutboxWorker(repo, sender, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(250 * time.Millisecond)
		cancel()
	}()
	w.Run(ctx)

	if len(sender.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sender.sent))
	}
	if sender.sent[0].SourceNode != "node-a" {
		t.Errorf("source_node = %q, want node-a", sender.sent[0].SourceNode)
	}
}
