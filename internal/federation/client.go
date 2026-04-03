package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dlc-01/replicast/internal/config"
)

// EventSender — интерфейс для отправки событий на удалённые узлы.
// Позволяет подменять реализацию в тестах worker'а.
type EventSender interface {
	SendEvent(ctx context.Context, baseURL, sharedSecret string, e EventPayload) error
}

type EventPayload struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SourceNode string          `json:"source_node"`
	Payload    json.RawMessage `json:"payload"`
}

// Client — HTTP реализация EventSender.
type Client struct {
	http *http.Client
	cfg  *config.Config
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		http: &http.Client{Timeout: 10 * time.Second},
		cfg:  cfg,
	}
}

// Проверяем что Client реализует EventSender — ошибка компиляции если нет.
var _ EventSender = (*Client)(nil)

func (c *Client) SendEvent(ctx context.Context, baseURL, sharedSecret string, e EventPayload) error {
	body, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("client.SendEvent marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx,
		http.MethodPost,
		baseURL+"/api/v1/federation/events",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("client.SendEvent request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Replicast-Secret", sharedSecret)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("client.SendEvent http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("client.SendEvent: remote returned %d", resp.StatusCode)
	}
	return nil
}
