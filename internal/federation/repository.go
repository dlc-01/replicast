package federation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oklog/ulid/v2"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/port"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnqueueEvent(ctx context.Context, e port.OutboxEvent) error {
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return fmt.Errorf("federation.EnqueueEvent marshal: %w", err)
	}
	eventID := e.EventID
	if eventID == "" {
		eventID = ulid.Make().String()
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO federation_outbox (event_id, target_node, event_type, payload)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (event_id) DO NOTHING`,
		eventID, e.TargetNode, e.EventType, payload,
	)
	return err
}

func (r *Repository) GetPendingEvents(ctx context.Context, limit int) ([]port.OutboxRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, event_id, target_node, event_type, payload, status, retry_count, next_retry_at
		FROM federation_outbox
		WHERE status = 'pending' AND next_retry_at <= now()
		ORDER BY next_retry_at
		LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("federation.GetPendingEvents: %w", err)
	}
	defer rows.Close()

	var events []port.OutboxRow
	for rows.Next() {
		var e port.OutboxRow
		if err := rows.Scan(
			&e.ID, &e.EventID, &e.TargetNode, &e.EventType,
			&e.Payload, &e.Status, &e.RetryCount, &e.NextRetryAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func (r *Repository) MarkDelivered(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE federation_outbox SET status = 'delivered' WHERE id = $1`, id)
	return err
}

// MarkFailed откладывает повторную доставку с exponential backoff.
// 5s → 10s → 20s → ... → 10min. После 10 попыток статус = 'failed'.
func (r *Repository) MarkFailed(ctx context.Context, id string, retryCount int) error {
	delay := time.Duration(math.Min(
		float64(5*time.Second)*math.Pow(2, float64(retryCount)),
		float64(10*time.Minute),
	))
	_, err := r.db.Exec(ctx, `
		UPDATE federation_outbox
		SET retry_count   = $2,
		    next_retry_at = now() + $3::interval,
		    status        = CASE WHEN $2 >= 10 THEN 'failed' ELSE 'pending' END
		WHERE id = $1`,
		id, retryCount, delay.String(),
	)
	return err
}

func (r *Repository) IsProcessed(ctx context.Context, eventID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)`, eventID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) MarkProcessed(ctx context.Context, eventID, sourceNode string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO processed_events (event_id, source_node) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		eventID, sourceNode,
	)
	return err
}

// GetNodeByName возвращает узел или apperr.ErrNotFound — никогда nil, nil.
func (r *Repository) GetNodeByName(ctx context.Context, name string) (*port.Node, error) {
	n := &port.Node{}
	err := r.db.QueryRow(ctx,
		`SELECT id, name, base_url, shared_secret, status FROM nodes WHERE name = $1`, name,
	).Scan(&n.ID, &n.Name, &n.BaseURL, &n.SharedSecret, &n.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.NotFound("node_not_found", "node not found")
		}
		return nil, fmt.Errorf("federation.GetNodeByName: %w", err)
	}
	return n, nil
}

func (r *Repository) UpsertNode(ctx context.Context, n port.Node) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO nodes (name, base_url, shared_secret, status)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE
		  SET base_url      = EXCLUDED.base_url,
		      shared_secret = EXCLUDED.shared_secret,
		      status        = EXCLUDED.status`,
		n.Name, n.BaseURL, n.SharedSecret, n.Status,
	)
	return err
}
