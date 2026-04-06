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

// GetLikers возвращает список пользователей лайкнувших пост.
func (r *Repository) GetLikers(ctx context.Context, postGlobalID string) ([]LikerInfo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT u.global_id, u.display_name
		FROM likes l
		JOIN users u ON u.id = l.user_id
		WHERE l.post_global_id = $1
		ORDER BY l.created_at DESC`,
		postGlobalID,
	)
	if err != nil {
		return nil, fmt.Errorf("likes.GetLikers: %w", err)
	}
	defer rows.Close()

	var out []LikerInfo
	for rows.Next() {
		var li LikerInfo
		if err := rows.Scan(&li.GlobalID, &li.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, li)
	}
	if out == nil {
		out = []LikerInfo{}
	}
	return out, rows.Err()
}

// GetPostHideLikes проверяет флаг hide_likes для поста.
func (r *Repository) GetPostHideLikes(ctx context.Context, postGlobalID string) (bool, error) {
	var hide bool
	err := r.db.QueryRow(ctx,
		`SELECT hide_likes FROM posts WHERE global_id = $1`, postGlobalID,
	).Scan(&hide)
	return hide, err
}

type LikerInfo struct {
	GlobalID    string `json:"global_id"`
	DisplayName string `json:"display_name"`
}
