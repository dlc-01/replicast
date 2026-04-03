package auth

import (
	"context"
	"errors"
	"fmt"

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

func (r *Repository) GetPasswordHash(ctx context.Context, globalID string) (string, error) {
	var hash string
	err := r.db.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE global_id = $1 AND is_local = true`,
		globalID,
	).Scan(&hash)
	if err != nil {
		return "", apperr.ErrInvalidPassword
	}
	return hash, nil
}

func (r *Repository) CreateUser(ctx context.Context, u port.User) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (global_id, local_username, home_node, display_name, bio, password_hash, is_local)
		VALUES ($1, $2, $3, $4, $5, $6, true)`,
		u.GlobalID, u.LocalUsername, u.HomeNode, u.DisplayName, u.Bio, u.PasswordHash,
	)
	if err != nil {
		return fmt.Errorf("auth.CreateUser: %w", err)
	}
	return nil
}

// GetByGlobalID возвращает пользователя или apperr.ErrUserNotFound.
// Никогда не возвращает (nil, nil).
func (r *Repository) GetByGlobalID(ctx context.Context, globalID string) (*port.User, error) {
	u := &port.User{}
	err := r.db.QueryRow(ctx,
		`SELECT id, global_id, local_username, home_node, display_name, bio, is_local, created_at
		 FROM users WHERE global_id = $1`,
		globalID,
	).Scan(&u.ID, &u.GlobalID, &u.LocalUsername, &u.HomeNode,
		&u.DisplayName, &u.Bio, &u.IsLocal, &u.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrUserNotFound
		}
		return nil, fmt.Errorf("auth.GetByGlobalID: %w", err)
	}
	return u, nil
}

func (r *Repository) UsernameExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE local_username = $1 AND is_local = true)`,
		username,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("auth.UsernameExists: %w", err)
	}
	return exists, nil
}
