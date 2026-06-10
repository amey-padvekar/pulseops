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
	"text/template"
	"time"
)

// ADKClient submits investigation requests through a Google Agent ADK boundary.
// It builds an ADK-shaped request that includes trace metadata and extracts the
// structured investigation payload from the ADK response envelope.
type ADKClient struct {
	endpoint   string
	authToken  string
	httpClient *http.Client
	maxRetries int
}

// ADKClientOptions configure the ADK client transport.
type ADKClientOptions struct {
	Endpoint   string
	AuthToken  string
	Timeout    time.Duration
	Client     *http.Client
	MaxRetries int
}

// ADKRequestMetadata carries traceability fields expected by Phase 7.
type ADKRequestMetadata struct {
	IncidentID       string `json:"incident_id"`
	DeviceID         string `json:"device_id"`
	RequestID        string `json:"request_id"`
	IdempotencyToken string `json:"idempotency_token,omitempty"`
}

// ADKRequestPayload is the ADK-facing request envelope.
type ADKRequestPayload struct {
	Task                string              `json:"task"`
	Prompt              string              `json:"prompt"`
	Metadata            ADKRequestMetadata  `json:"metadata"`
	ElasticContextHints ElasticContextHints `json:"elastic_context_hints"`
	AvailableActions    []ActionOption      `json:"available_actions"`
	EvidenceSummary     string              `json:"evidence_summary,omitempty"`
}

type promptData struct {
	IncidentID      string
	DeviceID        string
	Service         string
	TimeWindow      string
	EvidenceSummary string
}

// BuildADKRequestPayload creates an ADK request payload from the backend request.
func BuildADKRequestPayload(req AgentBuilderRequest, idempotencyToken string) (ADKRequestPayload, error) {
	tmpl, err := template.New("agentbuilder_prompt").Parse(PromptTemplate)
	if err != nil {
		return ADKRequestPayload{}, fmt.Errorf("parse prompt template: %w", err)
	}

	var promptBuf bytes.Buffer
	data := promptData{
		IncidentID:      req.IncidentID,
		DeviceID:        req.DeviceID,
		Service:         req.ServiceName,
		TimeWindow:      req.TimeWindow.Start.UTC().Format(time.RFC3339) + " to " + req.TimeWindow.End.UTC().Format(time.RFC3339),
		EvidenceSummary: strings.TrimSpace(req.EvidenceSummary),
	}
	if data.EvidenceSummary == "" {
		data.EvidenceSummary = "No Elastic evidence summary provided by backend."
	}

	if err := tmpl.Execute(&promptBuf, data); err != nil {
		return ADKRequestPayload{}, fmt.Errorf("render prompt template: %w", err)
	}

	return ADKRequestPayload{
		Task:   "phase7_investigation",
		Prompt: promptBuf.String(),
		Metadata: ADKRequestMetadata{
			IncidentID:       req.IncidentID,
			DeviceID:         req.DeviceID,
			RequestID:        req.RequestID,
			IdempotencyToken: strings.TrimSpace(idempotencyToken),
		},
		ElasticContextHints: req.ElasticContextHints,
		AvailableActions:    req.AvailableActions,
		EvidenceSummary:     req.EvidenceSummary,
	}, nil
}

// NewADKClient returns a configured ADK client.
func NewADKClient(opts ADKClientOptions) (*ADKClient, error) {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		return nil, errors.New("agent adk endpoint is required")
	}

	client := opts.Client
	if client == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	return &ADKClient{
		endpoint:   endpoint,
		authToken:  strings.TrimSpace(opts.AuthToken),
		httpClient: client,
		maxRetries: normalizeRetries(opts.MaxRetries),
	}, nil
}

// SubmitInvestigation sends an ADK request and returns extracted investigation JSON.
func (c *ADKClient) SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error) {
	if c == nil || c.httpClient == nil {
		return AgentBuilderResponse{}, errors.New("agent adk client is not initialized")
	}

	adkReq, err := BuildADKRequestPayload(req, "")
	if err != nil {
		return AgentBuilderResponse{}, fmt.Errorf("build adk request payload: %w", err)
	}

	body, err := json.Marshal(adkReq)
	if err != nil {
		return AgentBuilderResponse{}, fmt.Errorf("marshal adk request: %w", err)
	}

	var lastResponse AgentBuilderResponse
	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return lastResponse, ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return AgentBuilderResponse{}, fmt.Errorf("build adk transport request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if c.authToken != "" {
			httpReq.Header.Set("Authorization", c.authToken)
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("agent adk request failed: %w", err)
			if shouldRetry(ctx, 0, err) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		payload, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read adk response: %w", readErr)
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
			response = parseADKResponseEnvelope(req.RequestID, payload, response)
		}

		lastResponse = response
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("agent adk status %d", resp.StatusCode)
			if shouldRetry(ctx, resp.StatusCode, lastErr) && attempt < c.maxRetries {
				continue
			}
			return lastResponse, lastErr
		}

		return response, nil
	}

	return lastResponse, lastErr
}

// SubmitSummary sends a final-summary ADK request and returns the raw response payload for
// the backend to parse (Phase 11). It mirrors SubmitInvestigation's transport and retry
// handling but returns the raw bytes, since summary parsing (ParseFinalSummary) lives in
// the backend and tolerates either a bare or enveloped IncidentSummary.
func (c *ADKClient) SubmitSummary(ctx context.Context, payload ADKSummaryRequestPayload) (json.RawMessage, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("agent adk client is not initialized")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal adk summary request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build adk summary transport request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if c.authToken != "" {
			httpReq.Header.Set("Authorization", c.authToken)
		}

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("agent adk summary request failed: %w", err)
			if shouldRetry(ctx, 0, err) && attempt < c.maxRetries {
				continue
			}
			return nil, lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read adk summary response: %w", readErr)
			if shouldRetry(ctx, resp.StatusCode, readErr) && attempt < c.maxRetries {
				continue
			}
			return nil, lastErr
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("agent adk summary status %d", resp.StatusCode)
			if shouldRetry(ctx, resp.StatusCode, lastErr) && attempt < c.maxRetries {
				continue
			}
			return nil, lastErr
		}

		return json.RawMessage(respBody), nil
	}

	return nil, lastErr
}

func parseADKResponseEnvelope(defaultRequestID string, payload []byte, fallback AgentBuilderResponse) AgentBuilderResponse {
	response := fallback
	response.RawPayload = json.RawMessage(payload)

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return response
	}

	if v := firstStringField(decoded, "request_id", "requestId"); v != "" {
		response.RequestID = v
	} else {
		response.RequestID = defaultRequestID
	}

	if v := firstStringField(decoded, "trace_id", "traceId", "operation_id", "operationId"); v != "" {
		response.TraceID = v
	}

	if rawStatus, ok := decoded["status"]; ok {
		var status ResponseStatus
		if err := json.Unmarshal(rawStatus, &status); err == nil {
			if status.Transport != "" {
				response.Status.Transport = status.Transport
			}
			if status.Workflow != "" {
				response.Status.Workflow = status.Workflow
			}
		}
	}

	if investigation := extractInvestigationPayload(decoded, payload); len(investigation) > 0 {
		response.RawPayload = investigation
	}

	return response
}

func firstStringField(decoded map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := decoded[key]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err == nil {
			trimmed := strings.TrimSpace(value)
			if trimmed != "" {
				return trimmed
			}
		}
	}
	return ""
}

func extractInvestigationPayload(decoded map[string]json.RawMessage, payload []byte) json.RawMessage {
	if isInvestigationResultJSON(payload) {
		return json.RawMessage(payload)
	}

	candidates := []string{"result", "investigationResult", "output", "response"}
	for _, key := range candidates {
		raw, ok := decoded[key]
		if !ok || len(raw) == 0 {
			continue
		}
		if isInvestigationResultJSON(raw) {
			return json.RawMessage(raw)
		}
	}

	return nil
}

func isInvestigationResultJSON(raw []byte) bool {
	var probe struct {
		ProbableCause string              `json:"probableCause"`
		Confidence    *float64            `json:"confidence"`
		Actions       []RecommendedAction `json:"recommendedActions"`
		Steps         []string            `json:"validationSteps"`
		Summary       string              `json:"summary"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}

	return probe.ProbableCause != "" && probe.Confidence != nil && len(probe.Steps) > 0 && probe.Summary != ""
}
