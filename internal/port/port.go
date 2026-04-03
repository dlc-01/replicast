package port

import (
	"context"
	"time"
)

// — Users ──────────────────────────────────────────────────────────

type User struct {
	ID            string
	GlobalID      string
	LocalUsername string
	HomeNode      string
	DisplayName   string
	Bio           string
	PasswordHash  string
	IsLocal       bool
	CreatedAt     time.Time
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

// — Posts ──────────────────────────────────────────────────────────

type Post struct {
	ID         string
	GlobalID   string
	AuthorID   string
	OriginNode string
	Content    string
	Visibility string
	Status     string
	Version    int
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type PostRepository interface {
	Create(ctx context.Context, p Post) error
	GetByGlobalID(ctx context.Context, globalID string) (*Post, error)
	Update(ctx context.Context, globalID, content string) (*Post, error)
	Delete(ctx context.Context, globalID string) (*Post, error)
	GetFollowerNodes(ctx context.Context, authorID string) ([]string, error)
}

// — Follows ────────────────────────────────────────────────────────

type Follow struct {
	ID                 string
	FollowerUserID     string
	TargetGlobalUserID string
	TargetNode         string
	Status             string
	CreatedAt          time.Time
}

type FollowRepository interface {
	Create(ctx context.Context, f Follow) error
	Delete(ctx context.Context, followerUserID, targetGlobalID string) error
	Exists(ctx context.Context, followerUserID, targetGlobalID string) (bool, error)
	GetFollowees(ctx context.Context, followerUserID string) ([]Follow, error)
}

// — Feed ───────────────────────────────────────────────────────────

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

type FeedRepository interface {
	AddItem(ctx context.Context, item FeedItem) error
	RemoveItem(ctx context.Context, ownerUserID, postGlobalID string) error
	GetFeed(ctx context.Context, ownerUserID string, limit int) ([]FeedPost, error)
	GetFollowerUserIDs(ctx context.Context, authorGlobalID string) ([]string, error)
}

// — Federation ─────────────────────────────────────────────────────

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

type Node struct {
	ID           string
	Name         string
	BaseURL      string
	SharedSecret string
	Status       string
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
