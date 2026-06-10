package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// pendingCommandsResponse mirrors the backend's PendingCommandsResponse body returned
// by GET /devices/{deviceId}/commands.
type pendingCommandsResponse struct {
	DeviceID string    `json:"deviceId"`
	Commands []Command `json:"commands"`
}

// Client fetches pending remediation commands for a single device from the backend.
type Client struct {
	deviceID   string
	endpoint   string
	httpClient *http.Client
}

// NewClient builds a remediation command fetcher for deviceID against the backend at
// baseURL. The device id is path-escaped into the endpoint so the client only ever
// asks for its own commands.
func NewClient(baseURL, deviceID string, httpClient *http.Client) (*Client, error) {
	endpoint, err := joinCommandsEndpoint(baseURL, deviceID)
	if err != nil {
		return nil, err
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		deviceID:   deviceID,
		endpoint:   endpoint,
		httpClient: httpClient,
	}, nil
}

// Fetch retrieves the commands the backend currently has queued for this device.
// Fetching is the dispatch act on the backend, so each returned command has already
// transitioned to dispatched. As defense in depth the client drops any command whose
// deviceId does not match its own, so a misrouted payload is never acted on.
func (c *Client) Fetch(ctx context.Context) ([]Command, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build commands request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-PulseOps-Device-ID", c.deviceID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("backend connectivity error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read commands response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, message)
	}

	var decoded pendingCommandsResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode commands response: %w", err)
	}

	scoped := make([]Command, 0, len(decoded.Commands))
	for _, cmd := range decoded.Commands {
		if cmd.DeviceID != c.deviceID {
			continue
		}
		scoped = append(scoped, cmd)
	}
	return scoped, nil
}

func joinCommandsEndpoint(baseURL, deviceID string) (string, error) {
	if strings.TrimSpace(deviceID) == "" {
		return "", fmt.Errorf("deviceID is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse backend base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("backend base URL must be absolute")
	}
	// JoinPath escapes each segment correctly (e.g. a space in the device id) and keeps
	// Path/RawPath consistent so String() does not double-encode.
	return parsed.JoinPath("devices", deviceID, "commands").String(), nil
}
