package agentbuilder

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPClient_RetriesOnServerError(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&calls, 1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		resp := AgentBuilderResponse{
			RequestID:  "req-123",
			TraceID:    "trace-abc",
			Status:     ResponseStatus{Transport: "success", Workflow: "accepted"},
			ReceivedAt: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, err := NewHTTPClient(HTTPClientOptions{
		Endpoint:   server.URL,
		Timeout:    2 * time.Second,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("NewHTTPClient error: %v", err)
	}

	resp, err := client.SubmitInvestigation(context.Background(), AgentBuilderRequest{RequestID: "req-123"})
	if err != nil {
		t.Fatalf("SubmitInvestigation error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("calls = %d, want 2", got)
	}
	if resp.TraceID != "trace-abc" {
		t.Fatalf("traceId = %q, want %q", resp.TraceID, "trace-abc")
	}
}

func TestStubClient_FillsDefaults(t *testing.T) {
	stub := &StubClient{}
	resp, err := stub.SubmitInvestigation(context.Background(), AgentBuilderRequest{RequestID: "req-xyz"})
	if err != nil {
		t.Fatalf("SubmitInvestigation error: %v", err)
	}
	if resp.RequestID != "req-xyz" {
		t.Fatalf("requestId = %q, want %q", resp.RequestID, "req-xyz")
	}
	if resp.Status.Transport == "" || resp.Status.Workflow == "" {
		t.Fatalf("expected status defaults, got %+v", resp.Status)
	}
}
