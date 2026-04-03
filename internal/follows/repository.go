package follows

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dlc-01/replicast/internal/port"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, f port.Follow) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO follows (follower_user_id, target_global_user_id, target_node)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`,
		f.FollowerUserID, f.TargetGlobalUserID, f.TargetNode,
	)
	return err
}

func (r *Repository) Delete(ctx context.Context, followerUserID, targetGlobalID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE follows SET status = 'removed'
		WHERE follower_user_id = $1 AND target_global_user_id = $2`,
		followerUserID, targetGlobalID,
	)
	return err
}

func (r *Repository) Exists(ctx context.Context, followerUserID, targetGlobalID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM follows
			WHERE follower_user_id      = $1
			  AND target_global_user_id = $2
			  AND status = 'active'
		)`, followerUserID, targetGlobalID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) GetFollowees(ctx context.Context, followerUserID string) ([]port.Follow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, follower_user_id, target_global_user_id, target_node, status, created_at
		FROM follows
		WHERE follower_user_id = $1 AND status = 'active'`,
		followerUserID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []port.Follow
	for rows.Next() {
		var f port.Follow
		if err := rows.Scan(
			&f.ID, &f.FollowerUserID, &f.TargetGlobalUserID,
			&f.TargetNode, &f.Status, &f.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}
