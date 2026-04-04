package likes

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) Like(ctx context.Context, userID, postGlobalID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO likes (user_id, post_global_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, postGlobalID,
	)
	return err
}

func (r *Repository) Unlike(ctx context.Context, userID, postGlobalID string) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM likes WHERE user_id = $1 AND post_global_id = $2`,
		userID, postGlobalID,
	)
	return err
}

func (r *Repository) Count(ctx context.Context, postGlobalID string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM likes WHERE post_global_id = $1`, postGlobalID,
	).Scan(&n)
	return n, err
}

func (r *Repository) IsLiked(ctx context.Context, userID, postGlobalID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM likes WHERE user_id = $1 AND post_global_id = $2)`,
		userID, postGlobalID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM users WHERE global_id = $1`, globalID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("likes.GetUUIDByGlobalID: %w", err)
	}
	return id, nil
}
