package feed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dlc-01/replicast/internal/port"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) AddItem(ctx context.Context, item port.FeedItem) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO feed_items (owner_user_id, post_global_id, source_node)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		item.OwnerUserID, item.PostGlobalID, item.SourceNode,
	)
	return err
}

func (r *Repository) RemoveItem(ctx context.Context, ownerUserID, postGlobalID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM feed_items WHERE owner_user_id = $1 AND post_global_id = $2`,
		ownerUserID, postGlobalID,
	)
	return err
}

// GetFeed принимает ownerUserID (UUID) и возвращает ленту.
func (r *Repository) GetFeed(ctx context.Context, ownerUserID string, limit int) ([]port.FeedPost, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			fi.post_global_id,
			fi.source_node,
			p.content,
			u.global_id AS author_global_id,
			fi.created_at
		FROM feed_items fi
		JOIN posts p ON p.global_id = fi.post_global_id AND p.status = 'active'
		JOIN users u ON u.id = p.author_id
		WHERE fi.owner_user_id = $1
		ORDER BY fi.created_at DESC
		LIMIT $2`,
		ownerUserID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("feed.GetFeed: %w", err)
	}
	defer rows.Close()

	var items []port.FeedPost
	for rows.Next() {
		var fp port.FeedPost
		if err := rows.Scan(
			&fp.PostGlobalID, &fp.SourceNode,
			&fp.Content, &fp.AuthorGlobalID, &fp.CreatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, fp)
	}
	if items == nil {
		items = []port.FeedPost{}
	}
	return items, rows.Err()
}

// GetFollowerUserIDs возвращает UUID-ы локальных подписчиков автора.
func (r *Repository) GetFollowerUserIDs(ctx context.Context, authorGlobalID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT f.follower_user_id
		FROM follows f
		JOIN users u ON u.id = f.follower_user_id
		WHERE f.target_global_user_id = $1
		  AND f.status = 'active'
		  AND u.is_local = true`,
		authorGlobalID,
	)
	if err != nil {
		return nil, fmt.Errorf("feed.GetFollowerUserIDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetUUIDByGlobalID резолвит global_id → UUID — нужен feed хендлеру.
func (r *Repository) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM users WHERE global_id = $1`, globalID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("feed.GetUUIDByGlobalID: %w", err)
	}
	return id, nil
}
