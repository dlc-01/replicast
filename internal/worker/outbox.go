package worker

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/config"
	"github.com/dlc-01/replicast/internal/federation"
	"github.com/dlc-01/replicast/internal/port"
)

// outboxRepository — минимальный интерфейс для worker'а.
type outboxRepository interface {
	GetPendingEvents(ctx context.Context, limit int) ([]port.OutboxRow, error)
	MarkDelivered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, retryCount int) error
	GetNodeByName(ctx context.Context, name string) (*port.Node, error)
}

// OutboxWorker читает federation_outbox и доставляет события на удалённые узлы.
// Ключевой паттерн для диплома: надёжная доставка с exponential backoff.
// Если узел недоступен — событие не теряется, а откладывается и ретраится.
type OutboxWorker struct {
	repo   outboxRepository
	sender federation.EventSender
	cfg    *config.Config
}

func NewOutboxWorker(repo outboxRepository, sender federation.EventSender, cfg *config.Config) *OutboxWorker {
	return &OutboxWorker{repo: repo, sender: sender, cfg: cfg}
}

// Run запускает цикл обработки. Останавливается когда ctx отменён.
func (w *OutboxWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.OutboxInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *OutboxWorker) process(ctx context.Context) {
	events, err := w.repo.GetPendingEvents(ctx, 50)
	if err != nil {
		slog.Error("outbox: get pending events", "err", err)
		return
	}
	for _, e := range events {
		w.deliver(ctx, e)
	}
}

func (w *OutboxWorker) deliver(ctx context.Context, e port.OutboxRow) {
	node, err := w.repo.GetNodeByName(ctx, e.TargetNode)
	if err != nil {
		if appErr, ok := apperr.As(err); ok && appErr.Code == "node_not_found" {
			slog.Warn("outbox: target node not registered, skipping",
				"node", e.TargetNode,
				"event_id", e.EventID,
			)
			// Не ретраим — узел неизвестен
			_ = w.repo.MarkFailed(ctx, e.ID, 10)
			return
		}
		slog.Error("outbox: get node", "err", err, "node", e.TargetNode)
		_ = w.repo.MarkFailed(ctx, e.ID, e.RetryCount)
		return
	}

	var payload json.RawMessage
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		slog.Error("outbox: unmarshal payload", "err", err, "event_id", e.EventID)
		_ = w.repo.MarkFailed(ctx, e.ID, 10)
		return
	}

	event := federation.EventPayload{
		EventID:    e.EventID,
		EventType:  e.EventType,
		SourceNode: w.cfg.NodeName,
		Payload:    payload,
	}

	if err := w.sender.SendEvent(ctx, node.BaseURL, node.SharedSecret, event); err != nil {
		slog.Warn("outbox: delivery failed, will retry",
			"event_id", e.EventID,
			"target", e.TargetNode,
			"retry", e.RetryCount+1,
			"err", err,
		)
		_ = w.repo.MarkFailed(ctx, e.ID, e.RetryCount)
		return
	}

	_ = w.repo.MarkDelivered(ctx, e.ID)
	slog.Info("outbox: delivered",
		"event_id", e.EventID,
		"type", e.EventType,
		"target", e.TargetNode,
	)
}
