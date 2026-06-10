package remediation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HTTPReporter posts ExecutionResults to the backend ingestion endpoint
// (POST /remediation/results). It implements the Executor's ResultReporter seam,
// closing the remediation loop: the agent reports outcomes the backend stores.
type HTTPReporter struct {
	endpoint string
	client   *http.Client
}

// NewHTTPReporter builds a reporter targeting the backend at baseURL.
func NewHTTPReporter(baseURL string, client *http.Client) (*HTTPReporter, error) {
	endpoint, err := joinResultsEndpoint(baseURL)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPReporter{endpoint: endpoint, client: client}, nil
}

// Report sends one execution result to the backend. It returns an error on transport
// failure or a non-2xx response so the caller (the poller, via the Executor) can log it.
func (r *HTTPReporter) Report(ctx context.Context, result ExecutionResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal execution result: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build result request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PulseOps-Device-ID", result.DeviceID)
	req.Header.Set("X-PulseOps-Request-ID", result.RequestID)

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("backend connectivity error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		message := strings.TrimSpace(string(msg))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return fmt.Errorf("backend rejected result with status %d: %s", resp.StatusCode, message)
	}
	return nil
}

func joinResultsEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse backend base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("backend base URL must be absolute")
	}
	return parsed.JoinPath("remediation", "results").String(), nil
}
