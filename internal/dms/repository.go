package dms

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

func (r *Repository) GetOrCreateConversation(ctx context.Context, a, b, sessionKeyA, sessionKeyB string) (*port.Conversation, error) {
	pa, pb := a, b
	skA, skB := sessionKeyA, sessionKeyB
	if a > b {
		pa, pb = b, a
		skA, skB = sessionKeyB, sessionKeyA
	}

	conv := &port.Conversation{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO conversations (participant_a, participant_b, session_key_a, session_key_b)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (participant_a, participant_b) DO UPDATE
		  SET participant_a = EXCLUDED.participant_a
		RETURNING id, participant_a, participant_b, session_key_a, session_key_b, last_message_at, created_at`,
		pa, pb, nullStr(skA), nullStr(skB),
	).Scan(&conv.ID, &conv.ParticipantA, &conv.ParticipantB,
		&conv.SessionKeyA, &conv.SessionKeyB,
		&conv.LastMessageAt, &conv.CreatedAt)
	return conv, err
}

func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *Repository) GetConversation(ctx context.Context, id string) (*port.Conversation, error) {
	conv := &port.Conversation{}
	err := r.db.QueryRow(ctx,
		`SELECT id, participant_a, participant_b, session_key_a, session_key_b, last_message_at, created_at
		 FROM conversations WHERE id = $1`, id,
	).Scan(&conv.ID, &conv.ParticipantA, &conv.ParticipantB,
		&conv.SessionKeyA, &conv.SessionKeyB,
		&conv.LastMessageAt, &conv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperr.NotFound("conversation_not_found", "conversation not found")
	}
	return conv, err
}

func (r *Repository) ListConversations(ctx context.Context, userGlobalID string) ([]port.Conversation, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, participant_a, participant_b, session_key_a, session_key_b, last_message_at, created_at
		FROM conversations
		WHERE participant_a = $1 OR participant_b = $1
		ORDER BY COALESCE(last_message_at, created_at) DESC`,
		userGlobalID,
	)
	if err != nil {
		return nil, fmt.Errorf("dms.ListConversations: %w", err)
	}
	defer rows.Close()

	var out []port.Conversation
	for rows.Next() {
		var c port.Conversation
		if err := rows.Scan(&c.ID, &c.ParticipantA, &c.ParticipantB,
			&c.SessionKeyA, &c.SessionKeyB,
			&c.LastMessageAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if out == nil {
		out = []port.Conversation{}
	}
	return out, rows.Err()
}

func (r *Repository) CreateMessage(ctx context.Context, m port.Message) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO messages (global_id, conversation_id, sender_id, content)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (global_id) DO NOTHING`,
		m.GlobalID, m.ConversationID, m.SenderID, m.Content,
	)
	if err != nil {
		return err
	}
	// Обновляем last_message_at
	_, err = r.db.Exec(ctx,
		`UPDATE conversations SET last_message_at = now() WHERE id = $1`, m.ConversationID)
	return err
}

func (r *Repository) GetMessages(ctx context.Context, conversationID string, limit int) ([]port.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT m.global_id, m.conversation_id, m.sender_id, u.global_id, m.content, m.status, m.created_at
		FROM messages m
		JOIN users u ON u.id = m.sender_id
		WHERE m.conversation_id = $1
		ORDER BY m.created_at DESC
		LIMIT $2`,
		conversationID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("dms.GetMessages: %w", err)
	}
	defer rows.Close()

	var out []port.Message
	for rows.Next() {
		var m port.Message
		if err := rows.Scan(&m.GlobalID, &m.ConversationID, &m.SenderID, &m.SenderGlobalID, &m.Content, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if out == nil {
		out = []port.Message{}
	}
	return out, rows.Err()
}

func (r *Repository) GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM users WHERE global_id = $1`, globalID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("dms.GetUUIDByGlobalID: %w", err)
	}
	return id, nil
}

func (r *Repository) GetNodeByGlobalID(ctx context.Context, globalID string) (string, error) {
	var homeNode string
	err := r.db.QueryRow(ctx, `SELECT home_node FROM users WHERE global_id = $1`, globalID).Scan(&homeNode)
	if err != nil {
		return "", fmt.Errorf("dms.GetNodeByGlobalID: %w", err)
	}
	return homeNode, nil
}
