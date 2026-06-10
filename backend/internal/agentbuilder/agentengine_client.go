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

// TokenProvider returns a Google OAuth2 bearer access token. Injected so the
// client is testable without GCP/ADC; the real provider lives in package main.
type TokenProvider func(ctx context.Context) (string, error)

// AgentEngineClient calls a deployed Vertex AI Agent Engine (reasoningEngine)
// via its streamQuery endpoint and extracts the InvestigationResult JSON from the
// streamed ADK events, so the existing parse/persist path works unchanged.
type AgentEngineClient struct {
	resource    string
	location    string
	baseURL     string
	classMethod string
	userID      string
	token       TokenProvider
	httpClient  *http.Client
	maxRetries  int
}

// AgentEngineClientOptions configure the Agent Engine client.
type AgentEngineClientOptions struct {
	Resource    string
	Location    string
	BaseURL     string // override for tests; default https://{location}-aiplatform.googleapis.com
	ClassMethod string // default "stream_query" (the only query op AdkApp registers)
	UserID      string // default "pulseops-backend"
	Token       TokenProvider
	Timeout     time.Duration
	Client      *http.Client
	MaxRetries  int
}

// NewAgentEngineClient returns a configured Agent Engine client.
func NewAgentEngineClient(opts AgentEngineClientOptions) (*AgentEngineClient, error) {
	resource := strings.TrimSpace(opts.Resource)
	if resource == "" {
		return nil, errors.New("agent engine resource is required")
	}
	location := strings.TrimSpace(opts.Location)
	if location == "" {
		location = resourceSegment(resource, "locations")
	}
	if location == "" {
		return nil, errors.New("agent engine location is required")
	}
	if opts.Token == nil {
		return nil, errors.New("agent engine token provider is required")
	}

	client := opts.Client
	if client == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 60 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	base := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if base == "" {
		base = fmt.Sprintf("https://%s-aiplatform.googleapis.com", location)
	}
	method := strings.TrimSpace(opts.ClassMethod)
	if method == "" {
		method = "stream_query"
	}
	userID := strings.TrimSpace(opts.UserID)
	if userID == "" {
		userID = "pulseops-backend"
	}

	return &AgentEngineClient{
		resource:    resource,
		location:    location,
		baseURL:     base,
		classMethod: method,
		userID:      userID,
		token:       opts.Token,
		httpClient:  client,
		maxRetries:  normalizeRetries(opts.MaxRetries),
	}, nil
}

type agentEngineRequest struct {
	ClassMethod string         `json:"classMethod"`
	Input       map[string]any `json:"input"`
}

// BuildAgentEngineMessage renders the incident context the deployed agent reasons
// over. The agent's instruction tells it to use the Elastic MCP tools (scoped by
// elasticContextHints) and return InvestigationResult JSON.
func BuildAgentEngineMessage(req AgentBuilderRequest) (string, error) {
	contextJSON, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal agent engine message: %w", err)
	}
	return "Investigate this operational incident using the Elastic tools, then return " +
		"ONLY valid InvestigationResult JSON.\n\nIncident context (JSON):\n" + string(contextJSON), nil
}

func (c *AgentEngineClient) endpoint() string {
	return fmt.Sprintf("%s/v1/%s:streamQuery", c.baseURL, c.resource)
}

// SubmitInvestigation sends the incident to the Agent Engine and returns the
// extracted InvestigationResult JSON as RawPayload.
func (c *AgentEngineClient) SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error) {
	if c == nil || c.httpClient == nil {
		return AgentBuilderResponse{}, errors.New("agent engine client is not initialized")
	}

	message, err := BuildAgentEngineMessage(req)
	if err != nil {
		return AgentBuilderResponse{}, err
	}
	body, err := json.Marshal(agentEngineRequest{
		ClassMethod: c.classMethod,
		Input:       map[string]any{"message": message, "user_id": c.userID},
	})
	if err != nil {
		return AgentBuilderResponse{}, fmt.Errorf("marshal agent engine request: %w", err)
	}

	var lastResponse AgentBuilderResponse
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return lastResponse, ctx.Err()
		}

		token, err := c.token(ctx)
		if err != nil {
			return AgentBuilderResponse{}, fmt.Errorf("agent engine auth: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
		if err != nil {
			return AgentBuilderResponse{}, fmt.Errorf("build agent engine request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+token)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("agent engine request failed: %w", err)
			if shouldRetry(ctx, 0, err) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read agent engine response: %w", readErr)
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

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastResponse = response
			lastErr = fmt.Errorf("agent engine status %d: %s", resp.StatusCode, truncateForLog(string(payload), 300))
			if shouldRetry(ctx, resp.StatusCode, lastErr) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		if investigation := extractAgentEngineInvestigation(payload); len(investigation) > 0 {
			response.RawPayload = investigation
			return response, nil
		}

		// Transport succeeded (200) but the model returned no parseable InvestigationResult —
		// e.g. empty text, or a MALFORMED_FUNCTION_CALL where Gemini emitted a bad tool call and
		// no final answer. Both are non-deterministic, so re-sample within the retry budget
		// before giving up; only then hand the raw payload to the parse layer (which falls back).
		if attempt < c.maxRetries {
			lastErr = errors.New("agent engine returned no parseable InvestigationResult; retrying")
			continue
		}
		response.RawPayload = json.RawMessage(payload)
		return response, nil
	}

	return lastResponse, lastErr
}

// extractAgentEngineInvestigation pulls the InvestigationResult JSON out of the
// streamed Agent Engine response (SSE / JSON array / NDJSON of ADK events).
func extractAgentEngineInvestigation(payload []byte) json.RawMessage {
	text := collectAgentEngineText(payload)
	if inv := investigationFromText(text); len(inv) > 0 {
		return inv
	}
	// Fallback: the body itself might already be (or wrap) the result.
	return investigationFromText(string(payload))
}

func collectAgentEngineText(payload []byte) string {
	var sb strings.Builder
	for _, ev := range parseStreamEvents(payload) {
		sb.WriteString(textFromEvent(ev))
	}
	return sb.String()
}

func parseStreamEvents(payload []byte) []map[string]any {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}

	// JSON array of events.
	if trimmed[0] == '[' {
		var arr []map[string]any
		if err := json.Unmarshal(trimmed, &arr); err == nil {
			return arr
		}
	}

	// Single JSON object (one event, possibly pretty-printed).
	var single map[string]any
	if err := json.Unmarshal(trimmed, &single); err == nil {
		return []map[string]any{single}
	}

	// SSE ("data: {...}") or newline-delimited JSON, one event per line.
	var events []map[string]any
	for _, line := range strings.Split(string(trimmed), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(line[len("data:"):])
		}
		if line == "" || line == "[DONE]" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err == nil {
			events = append(events, ev)
		}
	}
	return events
}

// textFromEvent concatenates text parts from an ADK event's content.parts[].
func textFromEvent(ev map[string]any) string {
	content, ok := ev["content"].(map[string]any)
	if !ok {
		return ""
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return ""
	}
	var sb strings.Builder
	for _, p := range parts {
		if pm, ok := p.(map[string]any); ok {
			if txt, ok := pm["text"].(string); ok {
				sb.WriteString(txt)
			}
		}
	}
	return sb.String()
}

// investigationFromText extracts the first balanced-looking JSON object that is a
// valid InvestigationResult.
func investigationFromText(text string) json.RawMessage {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil
	}
	candidate := text[start : end+1]
	if isInvestigationResultJSON([]byte(candidate)) {
		return json.RawMessage(candidate)
	}
	return nil
}

func truncateForLog(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
