package agentbuilder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildADKRequestPayload_IncludesTraceMetadata(t *testing.T) {
	req := AgentBuilderRequest{
		RequestID:   "req-100",
		IncidentID:  "inc-200",
		DeviceID:    "dev-300",
		ServiceName: "OpenVPNService",
		TimeWindow: TimeWindow{
			Start: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 6, 1, 12, 5, 0, 0, time.UTC),
		},
		EvidenceSummary: "heartbeat=true; serviceStatus=stopped",
		AvailableActions: []ActionOption{
			{ActionID: ActionRestartService, Target: "OpenVPNService"},
		},
	}

	payload, err := BuildADKRequestPayload(req, "idem-1")
	if err != nil {
		t.Fatalf("BuildADKRequestPayload error: %v", err)
	}

	if payload.Metadata.RequestID != "req-100" {
		t.Fatalf("request_id = %q, want %q", payload.Metadata.RequestID, "req-100")
	}
	if payload.Metadata.IncidentID != "inc-200" {
		t.Fatalf("incident_id = %q, want %q", payload.Metadata.IncidentID, "inc-200")
	}
	if payload.Metadata.DeviceID != "dev-300" {
		t.Fatalf("device_id = %q, want %q", payload.Metadata.DeviceID, "dev-300")
	}
	if payload.Metadata.IdempotencyToken != "idem-1" {
		t.Fatalf("idempotency_token = %q, want %q", payload.Metadata.IdempotencyToken, "idem-1")
	}
	if !strings.Contains(payload.Prompt, "incidentId: inc-200") {
		t.Fatalf("prompt missing incident metadata: %s", payload.Prompt)
	}
	if !strings.Contains(payload.Prompt, "service stopped") {
		t.Fatalf("prompt missing expected heuristic context: %s", payload.Prompt)
	}
}

func TestADKClient_SubmitInvestigation_ExtractsTraceAndResultPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-1","trace_id":"trace-9","status":{"transport":"success","workflow":"accepted"},"result":{"probableCause":"service stopped","confidence":0.91,"recommendedActions":[{"actionId":"restart_service","target":"OpenVPNService","reason":"service is stopped while heartbeat is present"}],"validationSteps":["verify service status is running"],"summary":"Service appears stopped and should be restarted."}}`))
	}))
	defer server.Close()

	client, err := NewADKClient(ADKClientOptions{
		Endpoint: server.URL,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewADKClient error: %v", err)
	}

	resp, err := client.SubmitInvestigation(context.Background(), AgentBuilderRequest{
		RequestID:  "req-1",
		IncidentID: "inc-1",
		DeviceID:   "dev-1",
		TimeWindow: TimeWindow{Start: time.Now().Add(-time.Minute), End: time.Now()},
	})
	if err != nil {
		t.Fatalf("SubmitInvestigation error: %v", err)
	}

	if resp.TraceID != "trace-9" {
		t.Fatalf("traceId = %q, want %q", resp.TraceID, "trace-9")
	}

	var parsed InvestigationResult
	if err := json.Unmarshal(resp.RawPayload, &parsed); err != nil {
		t.Fatalf("raw payload is not investigation json: %v", err)
	}
	if parsed.ProbableCause != "service stopped" {
		t.Fatalf("probableCause = %q, want %q", parsed.ProbableCause, "service stopped")
	}
	if len(parsed.RecommendedActions) != 1 || parsed.RecommendedActions[0].ActionID != ActionRestartService {
		t.Fatalf("unexpected recommendedActions: %+v", parsed.RecommendedActions)
	}
}

func TestADKClient_SubmitInvestigation_PropagatesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"request_id":"req-timeout"}`))
	}))
	defer server.Close()

	client, err := NewADKClient(ADKClientOptions{
		Endpoint: server.URL,
		Timeout:  5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewADKClient error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err = client.SubmitInvestigation(ctx, AgentBuilderRequest{
		RequestID:  "req-timeout",
		IncidentID: "inc-timeout",
		DeviceID:   "dev-timeout",
		TimeWindow: TimeWindow{Start: time.Now().Add(-time.Minute), End: time.Now()},
	})
	if err == nil {
		t.Fatalf("expected timeout error")
	}

	if !strings.Contains(strings.ToLower(err.Error()), "deadline") && !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("expected timeout/deadline error, got: %v", err)
	}
}
