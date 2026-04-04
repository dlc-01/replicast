package comments

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dlc-01/replicast/internal/apperr"
	"github.com/dlc-01/replicast/internal/port"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, c port.Comment) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO comments (global_id, post_global_id, author_id, origin_node, content)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (global_id) DO NOTHING`,
		c.GlobalID, c.PostGlobalID, c.AuthorID, c.OriginNode, c.Content,
	)
	return err
}

func (r *Repository) GetByPost(ctx context.Context, postGlobalID string, limit int) ([]port.Comment, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT c.global_id, c.post_global_id, c.author_id, u.global_id, c.origin_node, c.content, c.created_at
		FROM comments c
		JOIN users u ON u.id = c.author_id
		WHERE c.post_global_id = $1 AND c.status = 'active'
		ORDER BY c.created_at ASC
		LIMIT $2`,
		postGlobalID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("comments.GetByPost: %w", err)
	}
	defer rows.Close()

	var out []port.Comment
	for rows.Next() {
		var c port.Comment
		if err := rows.Scan(&c.GlobalID, &c.PostGlobalID, &c.AuthorID, &c.AuthorGlobalID, &c.OriginNode, &c.Content, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []port.Comment{}
	}
	return out, rows.Err()
}

func (r *Repository) GetByGlobalID(ctx context.Context, globalID string) (*port.Comment, error) {
	c := &port.Comment{}
	err := r.db.QueryRow(ctx, `
		SELECT c.global_id, c.post_global_id, c.author_id, u.global_id, c.origin_node, c.content, c.created_at
		FROM comments c JOIN users u ON u.id = c.author_id
		WHERE c.global_id = $1 AND c.status = 'active'`,
		globalID,
	).Scan(&c.GlobalID, &c.PostGlobalID, &c.AuthorID, &c.AuthorGlobalID, &c.OriginNode, &c.Content, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("comment_not_found", "comment not found")
	}
	return c, err
}

func (r *Repository) Delete(ctx context.Context, globalID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE comments SET status = 'deleted' WHERE global_id = $1`, globalID)
	return err
}

func (r *Repository) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM users WHERE global_id = $1`, globalID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("comments.GetUUIDByGlobalID: %w", err)
	}
	return id, nil
}
