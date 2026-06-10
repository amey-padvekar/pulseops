package agentbuilder

import (
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func intPtr(i int) *int { return &i }

func resolvedIncident() incidents.Incident {
	detected := time.Date(2026, 6, 5, 10, 0, 0, 0, time.UTC)
	validated := time.Date(2026, 6, 5, 10, 5, 0, 0, time.UTC)
	return incidents.Incident{
		IncidentID:    "inc-1",
		DeviceID:      "dev-1",
		ServiceName:   "OpenVPNService",
		Severity:      incidents.SeverityHigh,
		State:         incidents.StateResolved,
		Reason:        "service status is running",
		DetectedAt:    detected,
		ProbableCause: "OpenVPN service unexpectedly stopped",
		Confidence:    0.9,
		Summary:       "service stopped while heartbeat remained",
		RecommendedActions: []incidents.RecommendedAction{
			{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart it"},
		},
		ApprovedActions: []string{"restart_service"},
		ApprovedBy:      "operator@pulseops",
		ApprovalNote:    "looks safe",
		RemediationStatus: "succeeded",
		RemediationResults: []incidents.RemediationActionResult{
			{ActionID: "restart_service", Target: "OpenVPNService", Status: "succeeded", ExitCode: intPtr(0), DurationMs: 1200, Stdout: "service restarted ok"},
		},
		Timeline: []incidents.TimelineEvent{
			{Type: incidents.EventCommandFinished, At: validated, Detail: "req-1 succeeded"},
		},
		ValidationStatus:      incidents.ValidationStatusSucceeded,
		LastValidationReason:  "service status is running",
		HealthyCycleCount:     2,
		RequiredHealthyCycles: 2,
		ValidatedAt:           &validated,
		LastValidationSnapshot: &incidents.ValidationSnapshot{
			ObservedAt:       validated,
			Healthy:          true,
			Reason:           "service status is running",
			ServiceStatus:    "running",
			Heartbeat:        true,
			NetworkReachable: true,
		},
	}
}

func TestBuildFinalSummaryRequest_Resolved(t *testing.T) {
	req, err := BuildFinalSummaryRequest(resolvedIncident(), FinalSummaryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Outcome != OutcomeResolved {
		t.Fatalf("outcome = %q, want %q", req.Outcome, OutcomeResolved)
	}
	if req.IncidentID != "inc-1" || req.DeviceID != "dev-1" || req.ServiceName != "OpenVPNService" {
		t.Fatalf("metadata mismatch: %+v", req)
	}
	if req.ProbableCause == "" || req.DiagnosisSummary == "" {
		t.Fatalf("expected diagnosis fields populated: %+v", req)
	}
	if len(req.RecommendedActions) != 1 || req.RecommendedActions[0].ActionID != "restart_service" {
		t.Fatalf("recommended actions mismatch: %+v", req.RecommendedActions)
	}
	if len(req.ApprovedActions) != 1 || req.ApprovedActions[0] != "restart_service" {
		t.Fatalf("approved actions mismatch: %+v", req.ApprovedActions)
	}
	if len(req.ExecutionResults) != 1 || req.ExecutionResults[0].Status != "succeeded" {
		t.Fatalf("execution results mismatch: %+v", req.ExecutionResults)
	}
	if req.ValidationStatus != incidents.ValidationStatusSucceeded {
		t.Fatalf("validation status mismatch: %q", req.ValidationStatus)
	}
	if req.ResolvedAt == nil {
		t.Fatalf("expected resolvedAt set")
	}
	if len(req.Evidence) == 0 {
		t.Fatalf("expected evidence snippets")
	}
	// Defaults applied.
	if req.SchemaVersion != defaultSchemaVersion || req.RequestID == "" || req.RequestedAt.IsZero() {
		t.Fatalf("expected defaults applied: %+v", req)
	}
}

func TestBuildFinalSummaryRequest_FailedOutcome(t *testing.T) {
	inc := resolvedIncident()
	inc.State = incidents.StateFailed
	inc.ValidationStatus = incidents.ValidationStatusFailed
	inc.ValidationFailureReason = "validation timed out: endpoint did not return to healthy"

	req, err := BuildFinalSummaryRequest(inc, FinalSummaryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Outcome != OutcomeFailed {
		t.Fatalf("outcome = %q, want %q", req.Outcome, OutcomeFailed)
	}
	if req.ValidationFailureReason == "" {
		t.Fatalf("expected failure reason preserved")
	}
	joined := strings.Join(req.Evidence, " | ")
	if !strings.Contains(joined, "Validation failure:") {
		t.Fatalf("expected failure evidence, got: %s", joined)
	}
}

func TestBuildFinalSummaryRequest_NonTerminalIsIncomplete(t *testing.T) {
	inc := resolvedIncident()
	inc.State = incidents.StateValidating
	req, err := BuildFinalSummaryRequest(inc, FinalSummaryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Outcome != OutcomeIncomplete {
		t.Fatalf("outcome = %q, want %q", req.Outcome, OutcomeIncomplete)
	}
}

func TestBuildFinalSummaryRequest_RequiresIncidentID(t *testing.T) {
	_, err := BuildFinalSummaryRequest(incidents.Incident{}, FinalSummaryOptions{})
	if err == nil {
		t.Fatalf("expected error for missing incidentId")
	}
}

func TestBuildFinalSummaryRequest_BoundsEvidence(t *testing.T) {
	inc := resolvedIncident()
	logs := make([]string, 0, 50)
	for i := 0; i < 50; i++ {
		logs = append(logs, "log line that is some text")
	}
	req, err := BuildFinalSummaryRequest(inc, FinalSummaryOptions{RecentLogs: logs, MaxEvidence: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Evidence) != 5 {
		t.Fatalf("evidence not bounded to 5: got %d", len(req.Evidence))
	}
}

func TestBuildFinalSummaryRequest_SummarizesLargeExecutionOutput(t *testing.T) {
	inc := resolvedIncident()
	big := strings.Repeat("x", 5000)
	inc.RemediationResults = []incidents.RemediationActionResult{
		{ActionID: "restart_service", Status: "failed", Stderr: big},
	}
	req, err := BuildFinalSummaryRequest(inc, FinalSummaryOptions{MaxSnippetLen: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	detail := req.ExecutionResults[0].Detail
	if len(detail) > 100 {
		t.Fatalf("execution detail not bounded: len=%d", len(detail))
	}
	if !strings.HasPrefix(detail, "stderr:") {
		t.Fatalf("expected stderr prefix, got: %q", detail)
	}
}

func TestBuildFinalSummaryRequest_RespectsExplicitRequestID(t *testing.T) {
	req, err := BuildFinalSummaryRequest(resolvedIncident(), FinalSummaryOptions{
		RequestID:     "custom-req",
		SchemaVersion: "v2",
		RequestedAt:   time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.RequestID != "custom-req" || req.SchemaVersion != "v2" {
		t.Fatalf("explicit options not honored: %+v", req)
	}
}
