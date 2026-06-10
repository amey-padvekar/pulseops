package agentbuilder

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func staticToken(string) TokenProvider {
	return func(context.Context) (string, error) { return "test-token", nil }
}

func sampleRequest() AgentBuilderRequest {
	return AgentBuilderRequest{
		RequestID:   "req-1",
		IncidentID:  "INC-42",
		DeviceID:    "dev-300",
		ServiceName: "OpenVPNService",
		AvailableActions: []ActionOption{
			{ActionID: "restart_service", Target: "OpenVPNService"},
		},
	}
}

const investigationJSON = `{"probableCause":"OpenVPNService stopped","confidence":0.9,` +
	`"recommendedActions":[{"actionId":"restart_service","target":"OpenVPNService"}],` +
	`"validationSteps":["confirm service running"],"summary":"restart recommended"}`

func TestAgentEngineClient_ExtractsInvestigationFromEventArray(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		// JSON array of ADK events; final event carries the InvestigationResult text.
		events := []map[string]any{
			{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": "investigating..."}}}},
			{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": investigationJSON}}}},
		}
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer srv.Close()

	client, err := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:  srv.URL,
		Token:    staticToken(""),
	})
	if err != nil {
		t.Fatalf("NewAgentEngineClient: %v", err)
	}

	resp, err := client.SubmitInvestigation(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("SubmitInvestigation: %v", err)
	}

	if !strings.HasSuffix(gotPath, ":streamQuery") {
		t.Fatalf("path = %q, want suffix :streamQuery", gotPath)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("auth = %q, want Bearer test-token", gotAuth)
	}
	if !strings.Contains(gotBody, `"classMethod":"stream_query"`) {
		t.Fatalf("body missing classMethod: %s", gotBody)
	}
	if !strings.Contains(gotBody, "INC-42") || !strings.Contains(gotBody, `"user_id"`) {
		t.Fatalf("body missing message/user_id: %s", gotBody)
	}

	if !isInvestigationResultJSON(resp.RawPayload) {
		t.Fatalf("RawPayload is not a valid InvestigationResult: %s", string(resp.RawPayload))
	}
	var probe struct {
		ProbableCause string `json:"probableCause"`
	}
	if err := json.Unmarshal(resp.RawPayload, &probe); err != nil || probe.ProbableCause == "" {
		t.Fatalf("RawPayload parse: err=%v cause=%q", err, probe.ProbableCause)
	}
}

func TestAgentEngineClient_ExtractsFromSSE(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"content\":{\"parts\":[{\"text\":\"thinking\"}]}}\n")
		io.WriteString(w, "data: {\"content\":{\"parts\":[{\"text\":"+strconvQuote(investigationJSON)+"}]}}\n")
		io.WriteString(w, "data: [DONE]\n")
	}))
	defer srv.Close()

	client, _ := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:  srv.URL,
		Token:    staticToken(""),
	})
	resp, err := client.SubmitInvestigation(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("SubmitInvestigation: %v", err)
	}
	if !isInvestigationResultJSON(resp.RawPayload) {
		t.Fatalf("SSE: RawPayload not InvestigationResult: %s", string(resp.RawPayload))
	}
}

func TestAgentEngineClient_Non2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":429,"message":"quota"}}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client, _ := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:  srv.URL,
		Token:    staticToken(""),
		// no retries to keep the test fast/deterministic
	})
	if _, err := client.SubmitInvestigation(context.Background(), sampleRequest()); err == nil {
		t.Fatal("expected error on 429")
	}
}

func TestAgentEngineClient_TokenErrorIsReturned(t *testing.T) {
	client, _ := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:  "http://127.0.0.1:0",
		Token:    func(context.Context) (string, error) { return "", io.ErrUnexpectedEOF },
	})
	if _, err := client.SubmitInvestigation(context.Background(), sampleRequest()); err == nil {
		t.Fatal("expected auth error")
	}
}

func TestNewAgentEngineClient_Validation(t *testing.T) {
	if _, err := NewAgentEngineClient(AgentEngineClientOptions{Token: staticToken("")}); err == nil {
		t.Fatal("expected error without resource")
	}
	if _, err := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
	}); err == nil {
		t.Fatal("expected error without token provider")
	}
	// location inferred from the resource path
	c, err := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/asia-south1/reasoningEngines/9",
		Token:    staticToken(""),
	})
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if c.location != "asia-south1" {
		t.Fatalf("location = %q, want asia-south1", c.location)
	}
}

func TestAgentEngineClient_RetriesWhenNoParseableResult(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// First turn mirrors a Gemini Flash MALFORMED_FUNCTION_CALL: 200, but no usable text.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model_version": "gemini-2.5-flash",
				"finish_reason": "MALFORMED_FUNCTION_CALL",
				"content":       map[string]any{"role": "model", "parts": []any{map[string]any{"text": ""}}},
			})
			return
		}
		// The retry re-samples and the model returns a clean InvestigationResult.
		events := []map[string]any{
			{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": investigationJSON}}}},
		}
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer srv.Close()

	client, _ := NewAgentEngineClient(AgentEngineClientOptions{
		Resource:   "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:    srv.URL,
		Token:      staticToken(""),
		MaxRetries: 1,
	})

	resp, err := client.SubmitInvestigation(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("SubmitInvestigation: %v", err)
	}
	if calls < 2 {
		t.Fatalf("server calls = %d, want >= 2 (should retry after the malformed first turn)", calls)
	}
	if !isInvestigationResultJSON(resp.RawPayload) {
		t.Fatalf("RawPayload not InvestigationResult after retry: %s", string(resp.RawPayload))
	}
}

func TestAgentEngineClient_UnparseableReturnsRawForFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"finish_reason": "MALFORMED_FUNCTION_CALL",
			"content":       map[string]any{"role": "model", "parts": []any{map[string]any{"text": ""}}},
		})
	}))
	defer srv.Close()

	// No retries (default) → the raw payload is returned so the parse layer can fall back
	// deterministically (unchanged behavior).
	client, _ := NewAgentEngineClient(AgentEngineClientOptions{
		Resource: "projects/p/locations/us-central1/reasoningEngines/123",
		BaseURL:  srv.URL,
		Token:    staticToken(""),
	})

	resp, err := client.SubmitInvestigation(context.Background(), sampleRequest())
	if err != nil {
		t.Fatalf("SubmitInvestigation: %v", err)
	}
	if isInvestigationResultJSON(resp.RawPayload) {
		t.Fatalf("expected non-InvestigationResult raw payload for fallback, got: %s", string(resp.RawPayload))
	}
}

// strconvQuote is a tiny helper to JSON-quote a string for inline SSE bodies.
func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
