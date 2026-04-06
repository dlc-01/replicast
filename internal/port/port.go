package port

import (
	"context"
	"time"
)

type User struct {
	ID            string    `json:"id,omitempty"`
	GlobalID      string    `json:"global_id"`
	LocalUsername string    `json:"username,omitempty"`
	HomeNode      string    `json:"home_node"`
	DisplayName   string    `json:"display_name,omitempty"`
	Bio           string    `json:"bio,omitempty"`
	PasswordHash  string    `json:"-"`
	PublicKey     string    `json:"public_key,omitempty"` // RSA публичный ключ для E2E
	IsLocal       bool      `json:"is_local"`
	CreatedAt     time.Time `json:"created_at"`
}

type Post struct {
	ID         string    `json:"id,omitempty"`
	GlobalID   string    `json:"global_id"`
	AuthorID   string    `json:"author_id,omitempty"`
	OriginNode string    `json:"origin_node"`
	Content    string    `json:"content"`
	Visibility string    `json:"visibility"`
	Status     string    `json:"status,omitempty"`
	HideLikes  bool      `json:"hide_likes"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Follow struct {
	ID                 string
	FollowerUserID     string
	TargetGlobalUserID string
	TargetNode         string
	Status             string
	CreatedAt          time.Time
}

type FeedItem struct {
	OwnerUserID  string
	PostGlobalID string
	SourceNode   string
}

type FeedPost struct {
	PostGlobalID   string    `json:"post_global_id"`
	SourceNode     string    `json:"source_node"`
	Content        string    `json:"content"`
	AuthorGlobalID string    `json:"author_global_id"`
	CreatedAt      time.Time `json:"created_at"`
}

type Comment struct {
	ID             string    `json:"id,omitempty"`
	GlobalID       string    `json:"global_id"`
	PostGlobalID   string    `json:"post_global_id"`
	AuthorID       string    `json:"-"`
	AuthorGlobalID string    `json:"author_global_id"`
	OriginNode     string    `json:"origin_node"`
	Content        string    `json:"content"`
	Status         string    `json:"status,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Message struct {
	ID             string    `json:"id,omitempty"`
	GlobalID       string    `json:"global_id"`
	ConversationID string    `json:"conversation_id"`
	SenderID       string    `json:"-"`
	SenderGlobalID string    `json:"sender_global_id"`
	Content        string    `json:"content"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type Conversation struct {
	ID            string     `json:"id"`
	ParticipantA  string     `json:"participant_a"`
	ParticipantB  string     `json:"participant_b"`
	SessionKeyA   string     `json:"session_key_a,omitempty"` // AES key зашифрованный для participant_a
	SessionKeyB   string     `json:"session_key_b,omitempty"` // AES key зашифрованный для participant_b
	LastMessageAt *time.Time `json:"last_message_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type Node struct {
	ID           string `json:"id,omitempty"`
	Name         string `json:"name"`
	BaseURL      string `json:"base_url"`
	SharedSecret string `json:"-"`
	PublicKey    string `json:"public_key,omitempty"`
	Status       string `json:"status,omitempty"`
}

type OutboxEvent struct {
	EventID    string
	TargetNode string
	EventType  string
	Payload    map[string]any
}

type OutboxRow struct {
	ID          string
	EventID     string
	TargetNode  string
	EventType   string
	Payload     []byte
	Status      string
	RetryCount  int
	NextRetryAt time.Time
}

type UserRepository interface {
	Create(ctx context.Context, u User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByGlobalID(ctx context.Context, globalID string) (*User, error)
	GetByUsername(ctx context.Context, username string) (*User, error)
	GetUUIDByGlobalID(ctx context.Context, globalID string) (string, error)
	UpdateProfile(ctx context.Context, id, displayName, bio string) error
	UpsertRemote(ctx context.Context, u User) error
	UsernameExists(ctx context.Context, username string) (bool, error)
	GetPasswordHash(ctx context.Context, globalID string) (string, error)
}

type PostRepository interface {
	Create(ctx context.Context, p Post) error
	GetByID(ctx context.Context, id string) (*Post, error)
	GetByGlobalID(ctx context.Context, globalID string) (*Post, error)
	Update(ctx context.Context, globalID, content string) (*Post, error)
	Delete(ctx context.Context, globalID string) (*Post, error)
	GetFollowerNodes(ctx context.Context, authorID string) ([]string, error)
}

type FeedRepository interface {
	AddItem(ctx context.Context, item FeedItem) error
	RemoveItem(ctx context.Context, ownerUserID, postGlobalID string) error
	GetFeed(ctx context.Context, ownerUserID string, limit int) ([]FeedPost, error)
	GetFollowerUserIDs(ctx context.Context, authorGlobalID string) ([]string, error)
}

type FederationRepository interface {
	EnqueueEvent(ctx context.Context, e OutboxEvent) error
	GetPendingEvents(ctx context.Context, limit int) ([]OutboxRow, error)
	MarkDelivered(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, retryCount int) error
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	MarkProcessed(ctx context.Context, eventID, sourceNode string) error
	GetNodeByName(ctx context.Context, name string) (*Node, error)
	UpsertNode(ctx context.Context, n Node) error
}
