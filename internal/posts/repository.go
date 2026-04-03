package posts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/port"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, p port.Post) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO posts (global_id, author_id, origin_node, content, visibility)
		VALUES ($1, $2, $3, $4, $5)`,
		p.GlobalID, p.AuthorID, p.OriginNode, p.Content, p.Visibility,
	)
	return err
}

func (r *Repository) GetByGlobalID(ctx context.Context, globalID string) (*port.Post, error) {
	p := &port.Post{}
	err := r.db.QueryRow(ctx, `
		SELECT id, global_id, author_id, origin_node, content, visibility, status, version, created_at, updated_at
		FROM posts WHERE global_id = $1 AND status = 'active'`,
		globalID,
	).Scan(
		&p.ID, &p.GlobalID, &p.AuthorID, &p.OriginNode,
		&p.Content, &p.Visibility, &p.Status, &p.Version,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) Update(ctx context.Context, globalID, content string) (*port.Post, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE posts
		SET content = $2, updated_at = $3, version = version + 1
		WHERE global_id = $1 AND status = 'active'`,
		globalID, content, time.Now(),
	)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() == 0 {
		return nil, apperr.ErrPostNotFound
	}
	return r.GetByGlobalID(ctx, globalID)
}

func (r *Repository) Delete(ctx context.Context, globalID string) (*port.Post, error) {
	p, err := r.GetByGlobalID(ctx, globalID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperr.ErrPostNotFound
	}
	_, err = r.db.Exec(ctx,
		`UPDATE posts SET status = 'deleted', updated_at = $2 WHERE global_id = $1`,
		globalID, time.Now(),
	)
	return p, err
}

// GetFollowerNodes возвращает узлы где есть удалённые подписчики автора.
// authorID — UUID пользователя.
// Исправлено: используем u.id = f.follower_user_id (оба UUID).
func (r *Repository) GetFollowerNodes(ctx context.Context, authorID string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		SELECT DISTINCT u.home_node
		FROM follows f
		JOIN users u ON u.id = f.follower_user_id
		WHERE f.target_global_user_id = (
			SELECT global_id FROM users WHERE id = $1
		)
		  AND f.status = 'active'
		  AND u.is_local = false`,
		authorID,
	)
	if err != nil {
		return nil, fmt.Errorf("posts.GetFollowerNodes: %w", err)
	}
	defer rows.Close()

	var nodes []string
	for rows.Next() {
		var node string
		if err := rows.Scan(&node); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}
