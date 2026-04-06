package users

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

func (r *Repository) Create(ctx context.Context, u port.User) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (global_id, local_username, home_node, display_name, bio, password_hash, is_local)
		VALUES ($1, $2, $3, $4, $5, $6, true)`,
		u.GlobalID, u.LocalUsername, u.HomeNode, u.DisplayName, u.Bio, u.PasswordHash,
	)
	return err
}

func (r *Repository) GetByID(ctx context.Context, id string) (*port.User, error) {
	return r.scanOne(ctx,
		`SELECT id, global_id, local_username, home_node, display_name, bio, public_key, is_local, created_at
		 FROM users WHERE id = $1`, id)
}

func (r *Repository) GetByGlobalID(ctx context.Context, globalID string) (*port.User, error) {
	return r.scanOne(ctx,
		`SELECT id, global_id, local_username, home_node, display_name, bio, public_key, is_local, created_at
		 FROM users WHERE global_id = $1`, globalID)
}

func (r *Repository) GetByUsername(ctx context.Context, username string) (*port.User, error) {
	return r.scanOne(ctx,
		`SELECT id, global_id, local_username, home_node, display_name, bio, public_key, is_local, created_at
		 FROM users WHERE local_username = $1 AND is_local = true`, username)
}

func (r *Repository) UpdateProfile(ctx context.Context, id, displayName, bio string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users SET display_name = $2, bio = $3 WHERE id = $1`, id, displayName, bio)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.ErrUserNotFound
	}
	return nil
}

func (r *Repository) UpsertRemote(ctx context.Context, u port.User) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (global_id, home_node, display_name, bio, is_local)
		VALUES ($1, $2, $3, $4, false)
		ON CONFLICT (global_id) DO UPDATE
		  SET display_name = EXCLUDED.display_name,
		      bio          = EXCLUDED.bio`,
		u.GlobalID, u.HomeNode, u.DisplayName, u.Bio,
	)
	return err
}

func (r *Repository) UsernameExists(ctx context.Context, username string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM users WHERE local_username = $1 AND is_local = true)`,
		username,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx,
		`SELECT id FROM users WHERE global_id = $1`, globalID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.ErrUserNotFound
	}
	return id, err
}

func (r *Repository) GetPasswordHash(ctx context.Context, globalID string) (string, error) {
	var hash string
	err := r.db.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE global_id = $1 AND is_local = true`, globalID,
	).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.ErrUserNotFound
	}
	return hash, err
}

// scanOne — внутренний хелпер для scan одной строки.
// При ErrNoRows возвращает ErrUserNotFound — никогда nil, nil.
func (r *Repository) scanOne(ctx context.Context, query string, args ...any) (*port.User, error) {
	u := &port.User{}
	var localUsername *string // NULL для remote users
	var publicKey *string     // NULL для remote users без ключа
	err := r.db.QueryRow(ctx, query, args...).Scan(
		&u.ID, &u.GlobalID, &localUsername, &u.HomeNode,
		&u.DisplayName, &u.Bio, &publicKey, &u.IsLocal, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.ErrUserNotFound
		}
		return nil, fmt.Errorf("users.scanOne: %w", err)
	}
	if localUsername != nil {
		u.LocalUsername = *localUsername
	}
	if publicKey != nil {
		u.PublicKey = *publicKey
	}
	return u, nil
}
