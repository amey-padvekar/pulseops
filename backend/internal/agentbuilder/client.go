package agentbuilder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client submits investigation requests to Agent Builder.
type Client interface {
	SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error)
}

// HTTPClient sends Agent Builder requests over HTTP.
type HTTPClient struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
	maxRetries int
}

// HTTPClientOptions configure the Agent Builder HTTP client.
type HTTPClientOptions struct {
	Endpoint   string
	AuthToken  string
	Timeout    time.Duration
	Client     *http.Client
	MaxRetries int
}

// NewHTTPClient returns a configured HTTP client for Agent Builder.
func NewHTTPClient(opts HTTPClientOptions) (*HTTPClient, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, errors.New("agent builder endpoint is required")
	}

	client := opts.Client
	if client == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	return &HTTPClient{
		endpoint:   endpoint,
		authToken:  strings.TrimSpace(opts.AuthToken),
		httpClient: client,
		maxRetries: normalizeRetries(opts.MaxRetries),
	}, nil
}

// SubmitInvestigation sends the request payload and parses the response.
func (c *HTTPClient) SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error) {
	if c == nil || c.httpClient == nil {
		return AgentBuilderResponse{}, errors.New("agent builder client is not initialized")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return AgentBuilderResponse{}, fmt.Errorf("marshal agent builder request: %w", err)
	}

	var lastResponse AgentBuilderResponse
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return lastResponse, ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return AgentBuilderResponse{}, fmt.Errorf("build agent builder request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if c.authToken != "" {
			httpReq.Header.Set("Authorization", c.authToken)
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("agent builder request failed: %w", err)
			if shouldRetry(ctx, 0, err) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read agent builder response: %w", readErr)
			if shouldRetry(ctx, resp.StatusCode, readErr) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		response := AgentBuilderResponse{
			RequestID:  req.RequestID,
			ReceivedAt: time.Now().UTC(),
			Status: ResponseStatus{
				Transport: transportStatus(resp.StatusCode),
				Workflow:  "accepted",
			},
		}

		if len(payload) > 0 {
			response.RawPayload = json.RawMessage(payload)
			var decoded AgentBuilderResponse
			if err := json.Unmarshal(payload, &decoded); err == nil {
				if decoded.RequestID != "" {
					response.RequestID = decoded.RequestID
				}
				if decoded.TraceID != "" {
					response.TraceID = decoded.TraceID
				}
				if decoded.ReceivedAt.IsZero() {
					decoded.ReceivedAt = response.ReceivedAt
				}
				if decoded.Status.Transport == "" {
					decoded.Status.Transport = response.Status.Transport
				}
				if decoded.Status.Workflow == "" {
					decoded.Status.Workflow = response.Status.Workflow
				}
				response = decoded
				response.RawPayload = json.RawMessage(payload)
			}
		}

		lastResponse = response
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("agent builder status %d", resp.StatusCode)
			if shouldRetry(ctx, resp.StatusCode, lastErr) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		return response, nil
	}

	return lastResponse, lastErr
}

// StubClient returns a canned response without network calls.
type StubClient struct {
	Response AgentBuilderResponse
	Err      error
}

// SubmitInvestigation returns the configured stub response.
func (s *StubClient) SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error) {
	if s == nil {
		return AgentBuilderResponse{}, errors.New("stub client is nil")
	}

	if s.Err != nil {
		return s.Response, s.Err
	}

	response := s.Response
	if response.RequestID == "" {
		response.RequestID = req.RequestID
	}
	if response.ReceivedAt.IsZero() {
		response.ReceivedAt = time.Now().UTC()
	}
	if response.Status.Transport == "" {
		response.Status.Transport = "success"
	}
	if response.Status.Workflow == "" {
		response.Status.Workflow = "accepted"
	}

	return response, nil
}

func transportStatus(statusCode int) string {
	if statusCode >= 200 && statusCode < 300 {
		return "success"
	}
	return "error"
}

func normalizeRetries(value int) int {
	if value < 0 {
		return 0
	}
	if value > 2 {
		return 2
	}
	return value
}

func shouldRetry(ctx context.Context, statusCode int, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	if err != nil && statusCode == 0 {
		return true
	}
	if statusCode == http.StatusTooManyRequests {
		return true
	}
	return statusCode >= 500
}
