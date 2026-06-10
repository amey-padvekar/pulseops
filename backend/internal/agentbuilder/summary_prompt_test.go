package agentbuilder

import (
	"encoding/json"
	"strings"
	"testing"
)

func sampleSummaryRequest() FinalSummaryRequest {
	return FinalSummaryRequest{
		SchemaVersion: "v1",
		RequestID:     "req-1",
		IncidentID:    "inc-1",
		DeviceID:      "dev-1",
		ServiceName:   "OpenVPNService",
		Severity:      "high",
		State:         "resolved",
		Outcome:       OutcomeResolved,
		ProbableCause: "OpenVPN service unexpectedly stopped",
		Confidence:    0.9,
		DiagnosisSummary: "service stopped while heartbeat remained",
		RecommendedActions: []SummaryAction{
			{ActionID: "restart_service", Target: "OpenVPNService"},
		},
		ApprovedActions:   []string{"restart_service"},
		RemediationStatus: "succeeded",
		ExecutionResults: []SummaryExecutionResult{
			{ActionID: "restart_service", Status: "succeeded", Detail: "stdout: service restarted ok"},
		},
		ValidationStatus: "succeeded",
		ValidationReason: "service status is running",
		Evidence: []string{
			"Detection: service status is stopped",
			"Validation telemetry: serviceStatus=running heartbeat=true",
		},
	}
}

func TestBuildSummaryPrompt_IncludesFactsAndRules(t *testing.T) {
	prompt, err := BuildSummaryPrompt(sampleSummaryRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mustContain := []string{
		"inc-1", "dev-1", "OpenVPNService", "resolved",
		"OpenVPN service unexpectedly stopped",
		"recommended: restart_service for OpenVPNService",
		"approved: restart_service",
		"executed: restart_service -> succeeded",
		"validation reason: service status is running",
		"Detection: service status is stopped",
		"Reason ONLY from the facts above",
		"IncidentSummary",
	}
	for _, want := range mustContain {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing %q\n---\n%s", want, prompt)
		}
	}
}

func TestBuildSummaryPrompt_EmptySectionsUsePlaceholder(t *testing.T) {
	req := FinalSummaryRequest{
		IncidentID: "inc-2",
		Outcome:    OutcomeFailed,
	}
	prompt, err := BuildSummaryPrompt(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(prompt, summaryEmptyBlock) < 2 {
		t.Fatalf("expected placeholder for empty diagnosis/actions/evidence, got:\n%s", prompt)
	}
	// Missing identifiers fall back to a placeholder rather than rendering blank.
	if !strings.Contains(prompt, "deviceId: unknown") {
		t.Fatalf("expected unknown deviceId placeholder, got:\n%s", prompt)
	}
}

func TestBuildSummaryPrompt_FailedOutcomeWording(t *testing.T) {
	req := sampleSummaryRequest()
	req.State = "failed"
	req.Outcome = OutcomeFailed
	req.ValidationStatus = "failed"
	req.ValidationFailureReason = "validation timed out: endpoint did not return to healthy"

	prompt, err := BuildSummaryPrompt(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(prompt, "failure reason: validation timed out") {
		t.Fatalf("expected failure reason in prompt:\n%s", prompt)
	}
	if !strings.Contains(prompt, `outcome: failed`) {
		t.Fatalf("expected failed outcome in prompt:\n%s", prompt)
	}
}

func TestBuildSummaryADKRequestPayload(t *testing.T) {
	req := sampleSummaryRequest()
	payload, err := BuildSummaryADKRequestPayload(req, "idem-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.Task != summaryTaskName {
		t.Errorf("task = %q, want %q", payload.Task, summaryTaskName)
	}
	if payload.Prompt == "" {
		t.Errorf("expected rendered prompt")
	}
	if payload.Metadata.IncidentID != "inc-1" || payload.Metadata.RequestID != "req-1" {
		t.Errorf("metadata mismatch: %+v", payload.Metadata)
	}
	if payload.Metadata.IdempotencyToken != "idem-123" {
		t.Errorf("idempotency token not propagated: %q", payload.Metadata.IdempotencyToken)
	}
	if payload.SummaryRequest.IncidentID != "inc-1" {
		t.Errorf("structured request not embedded for traceability")
	}

	// Envelope must marshal cleanly for transport.
	if _, err := json.Marshal(payload); err != nil {
		t.Fatalf("payload does not marshal: %v", err)
	}
}
