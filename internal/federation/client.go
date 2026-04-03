package federation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dlc-01/replicast/internal/config"
)

// EventSender — интерфейс для отправки событий на удалённые узлы.
type EventSender interface {
	SendEvent(ctx context.Context, baseURL, sharedSecret string, e EventPayload) error
}

// NodeDiscoverer — интерфейс для discovery удалённого узла через /.well-known/replicast.
type NodeDiscoverer interface {
	FetchWellKnown(ctx context.Context, nodeName string) (*WellKnownResponse, error)
}

type EventPayload struct {
	EventID    string          `json:"event_id"`
	EventType  string          `json:"event_type"`
	SourceNode string          `json:"source_node"`
	Payload    json.RawMessage `json:"payload"`
}

type WellKnownResponse struct {
	Node    string `json:"node"`
	BaseURL string `json:"base_url"`
	Version string `json:"version"`
}

// Client — HTTP реализация EventSender и NodeDiscoverer.
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

var _ EventSender = (*Client)(nil)
var _ NodeDiscoverer = (*Client)(nil)

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

// FetchWellKnown получает метаданные узла через /.well-known/replicast.
// nodeName может быть доменом (vasya.ru) или host:port (localhost:8082).
// Автоматически определяет схему — https для доменов без порта, http для host:port.
func (c *Client) FetchWellKnown(ctx context.Context, nodeName string) (*WellKnownResponse, error) {
	baseURL := nodeNameToBaseURL(nodeName)
	url := baseURL + "/.well-known/replicast"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("client.FetchWellKnown request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.FetchWellKnown http %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("client.FetchWellKnown: remote returned %d", resp.StatusCode)
	}

	var wk WellKnownResponse
	if err := json.NewDecoder(resp.Body).Decode(&wk); err != nil {
		return nil, fmt.Errorf("client.FetchWellKnown decode: %w", err)
	}
	return &wk, nil
}

// nodeNameToBaseURL превращает имя узла в base URL.
// vasya.ru           → https://vasya.ru  (прод — чистый домен с точкой)
// localhost:8082     → http://localhost:8082 (dev с портом)
// node-a:8080        → http://node-a:8080 (Docker с портом)
// node-a             → http://node-a:8080 (Docker без порта — добавляем дефолтный порт)
func nodeNameToBaseURL(nodeName string) string {
	if strings.HasPrefix(nodeName, "http://") || strings.HasPrefix(nodeName, "https://") {
		return nodeName
	}
	// host:port → http (dev/docker окружение)
	if strings.Contains(nodeName, ":") {
		return "http://" + nodeName
	}
	// содержит точку → настоящий домен → https (прод)
	if strings.Contains(nodeName, ".") {
		return "https://" + nodeName
	}
	// простое имя без точки и порта → Docker service name → http + дефолтный порт
	return "http://" + nodeName + ":8080"
}

// FetchWellKnownInfo реализует follows.FedDiscoverer.
// Возвращает node name и base_url удалённого узла.
func (c *Client) FetchWellKnownInfo(ctx context.Context, nodeName string) (node, baseURL string, err error) {
	wk, err := c.FetchWellKnown(ctx, nodeName)
	if err != nil {
		return "", "", err
	}
	return wk.Node, wk.BaseURL, nil
}
