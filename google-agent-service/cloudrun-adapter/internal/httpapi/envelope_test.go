package httpapi

import (
	"strings"
	"testing"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

func TestSuccessEnvelope(t *testing.T) {
	res := domain.InvestigationResult{
		ProbableCause:      "service stopped",
		Confidence:         0.9,
		RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service"}},
		ValidationSteps:    []string{"confirm status"},
		Summary:            "restart needed",
	}

	env := successEnvelope("req-1", "trace-1", res)
	if env.RequestID != "req-1" {
		t.Fatalf("request_id = %q, want req-1", env.RequestID)
	}
	if env.TraceID != "trace-1" {
		t.Fatalf("trace_id = %q, want trace-1", env.TraceID)
	}
	if env.Status.Transport != "success" || env.Status.Workflow != "completed" {
		t.Fatalf("unexpected status: %+v", env.Status)
	}
	if env.Result == nil || env.Result.ProbableCause != "service stopped" {
		t.Fatalf("unexpected result: %+v", env.Result)
	}
	if env.Error != nil {
		t.Fatalf("expected nil error envelope")
	}
}

func TestErrorEnvelopeSanitizesMessageAndKeepsRequestID(t *testing.T) {
	env := errorEnvelope(" req-2 ", "", errCodeValidationFailed)
	if env.RequestID != "req-2" {
		t.Fatalf("request_id = %q, want req-2", env.RequestID)
	}
	if !strings.HasPrefix(env.TraceID, "trace-") {
		t.Fatalf("expected generated trace_id, got %q", env.TraceID)
	}
	if env.Status.Transport != "error" || env.Status.Workflow != "failed" {
		t.Fatalf("unexpected status: %+v", env.Status)
	}
	if env.Error == nil || env.Error.Code != errCodeValidationFailed {
		t.Fatalf("unexpected error code: %+v", env.Error)
	}
	if env.Error.Message != "request validation failed" {
		t.Fatalf("unexpected sanitized message: %q", env.Error.Message)
	}
}
