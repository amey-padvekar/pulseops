package agentbuilder

import (
	"strings"
	"testing"
	"time"
)

func TestFormatRequestLog_RedactsLogLines(t *testing.T) {
	req := AgentBuilderRequest{
		RequestID:     "req-1",
		IncidentID:    "inc-1",
		DeviceID:      "DEV-01",
		ServiceName:   "svc",
		IncidentState: "investigating",
		Severity:      "high",
		TimeWindow: TimeWindow{
			Start: time.Date(2026, 5, 30, 9, 0, 0, 0, time.UTC),
			End:   time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC),
		},
		RecentLogs:       []string{"secret-line"},
		AvailableActions: []ActionOption{{ActionID: "restart_service"}},
	}

	msg := FormatRequestLog(req, "https://agent-builder.example.com")
	if !strings.Contains(msg, "request_id=req-1") {
		t.Fatalf("expected request_id in log: %s", msg)
	}
	if !strings.Contains(msg, "logs=1") {
		t.Fatalf("expected logs count in log: %s", msg)
	}
	if strings.Contains(msg, "secret-line") {
		t.Fatalf("expected log lines to be redacted: %s", msg)
	}
}

func TestFormatResponseLog_IncludesTraceAndStatus(t *testing.T) {
	resp := AgentBuilderResponse{
		RequestID:  "req-1",
		TraceID:    "trace-1",
		Status:     ResponseStatus{Transport: "success", Workflow: "accepted"},
		ReceivedAt: time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC),
	}

	msg := FormatResponseLog(resp)
	if !strings.Contains(msg, "trace_id=trace-1") {
		t.Fatalf("expected trace_id in log: %s", msg)
	}
	if !strings.Contains(msg, "status_transport=success") {
		t.Fatalf("expected status_transport in log: %s", msg)
	}
}
