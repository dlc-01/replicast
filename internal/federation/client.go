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
	"github.com/dlc-01/replicast/internal/port"
	"github.com/dlc-01/replicast/internal/signing"
)

type EventSender interface {
	SendEvent(ctx context.Context, baseURL, sharedSecret string, e EventPayload) error
}

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

	// HMAC подпись — целостность тела и защита от replay атак
	if err := signing.SignRequest(req, c.cfg.NodeName, sharedSecret); err != nil {
		return fmt.Errorf("client.SendEvent sign: %w", err)
	}

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
func nodeNameToBaseURL(nodeName string) string {
	if strings.HasPrefix(nodeName, "http://") || strings.HasPrefix(nodeName, "https://") {
		return nodeName
	}
	if strings.Contains(nodeName, ":") {
		return "http://" + nodeName
	}
	if strings.Contains(nodeName, ".") {
		return "https://" + nodeName
	}
	return "http://" + nodeName + ":8080"
}

// FetchWellKnownInfo реализует follows.FedDiscoverer.
func (c *Client) FetchWellKnownInfo(ctx context.Context, nodeName string) (node, baseURL string, err error) {
	wk, err := c.FetchWellKnown(ctx, nodeName)
	if err != nil {
		return "", "", err
	}
	return wk.Node, wk.BaseURL, nil
}

// FetchRemoteUser получает профиль пользователя с удалённого узла.
// globalID формат: username@node
// Обращается к GET /api/v1/federation/users/{global_id} на удалённом узле.
func (c *Client) FetchRemoteUser(ctx context.Context, globalID string) (*port.User, error) {
	parts := strings.SplitN(globalID, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("client.FetchRemoteUser: invalid global_id %q", globalID)
	}
	nodeName := parts[1]
	baseURL := nodeNameToBaseURL(nodeName)

	encoded := strings.ReplaceAll(globalID, "@", "%40")
	encoded = strings.ReplaceAll(encoded, ":", "%3A")
	url := baseURL + "/api/v1/federation/users/" + encoded

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("client.FetchRemoteUser request: %w", err)
	}
	req.Header.Set("X-Replicast-Secret", c.cfg.SharedSecret)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("client.FetchRemoteUser http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("client.FetchRemoteUser: user %q not found on %s", globalID, nodeName)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("client.FetchRemoteUser: remote returned %d", resp.StatusCode)
	}

	var u port.User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("client.FetchRemoteUser decode: %w", err)
	}
	return &u, nil
}
