package agentbuilder

import (
	"strings"
	"testing"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func joined(ev []string) string { return strings.Join(ev, " || ") }

func TestSelectEvidence_DrawsFromAllSources(t *testing.T) {
	inc := resolvedIncident() // from summary_request_test.go
	inc.ServiceStatus = "stopped"
	logs := []string{"error: openvpn exited", "info: restart issued"}

	ev := selectEvidence(inc, logs, defaultMaxEvidence, defaultMaxSnippetLen)
	j := joined(ev)

	wantSubstrings := []string{
		"Detection:",
		"Initial telemetry: serviceStatus=stopped",
		"Diagnosis: OpenVPN service unexpectedly stopped (confidence 0.90)",
		"Log: error: openvpn exited",
		"Approved remediation: restart_service",
		"Execution: restart_service succeeded",
		"Validation telemetry: serviceStatus=running heartbeat=true",
		"Outcome: resolved",
	}
	for _, want := range wantSubstrings {
		if !strings.Contains(j, want) {
			t.Errorf("evidence missing %q\n---\n%s", want, j)
		}
	}
}

func TestSelectEvidence_CapsLogs(t *testing.T) {
	inc := incidents.Incident{State: incidents.StateResolved, Reason: "svc stopped"}
	logs := []string{"l1", "l2", "l3", "l4", "l5", "l6"}

	ev := selectEvidence(inc, logs, defaultMaxEvidence, defaultMaxSnippetLen)
	logCount := 0
	for _, e := range ev {
		if strings.HasPrefix(e, "Log: ") {
			logCount++
		}
	}
	if logCount > defaultMaxLogEvidence {
		t.Fatalf("log evidence not capped: got %d, want <= %d (%v)", logCount, defaultMaxLogEvidence, ev)
	}
	// Most recent logs are kept (tail).
	if !strings.Contains(joined(ev), "Log: l6") {
		t.Fatalf("expected most-recent log retained: %v", ev)
	}
}

func TestSelectEvidence_OutcomeRetainedUnderCap(t *testing.T) {
	inc := resolvedIncident()
	inc.ServiceStatus = "stopped"
	logs := []string{"a", "b", "c"}

	ev := selectEvidence(inc, logs, 3, defaultMaxSnippetLen)
	if len(ev) != 3 {
		t.Fatalf("expected cap of 3, got %d: %v", len(ev), ev)
	}
	// Even with many higher-priority points, the outcome line must survive.
	if !strings.Contains(ev[len(ev)-1], "Outcome: resolved") {
		t.Fatalf("outcome line not retained as last evidence under cap: %v", ev)
	}
}

func TestSelectEvidence_FailedReflectsFailure(t *testing.T) {
	inc := resolvedIncident()
	inc.State = incidents.StateFailed
	inc.ValidationFailureReason = "validation timed out: endpoint did not return to healthy"

	ev := selectEvidence(inc, nil, defaultMaxEvidence, defaultMaxSnippetLen)
	j := joined(ev)
	if !strings.Contains(j, "Validation failure: validation timed out") {
		t.Fatalf("failed incident evidence must surface failure reason: %s", j)
	}
	if strings.Contains(j, "Outcome: resolved") {
		t.Fatalf("failed incident must not claim resolution: %s", j)
	}
}

func TestSelectEvidence_CompactsAndDropsEmptyLabels(t *testing.T) {
	// No reason, no status -> the "Detection:" / "Initial telemetry:" labels must not
	// appear as empty entries.
	inc := incidents.Incident{State: incidents.StateResolved}
	ev := selectEvidence(inc, nil, defaultMaxEvidence, defaultMaxSnippetLen)
	for _, e := range ev {
		if strings.HasSuffix(e, ":") {
			t.Fatalf("empty-label evidence leaked: %q (%v)", e, ev)
		}
	}
	// Still produces the outcome line so evidence is never empty for a terminal incident.
	if len(ev) == 0 {
		t.Fatalf("expected at least the outcome line")
	}
}

func TestSelectEvidence_TruncatesLongLines(t *testing.T) {
	inc := incidents.Incident{
		State:  incidents.StateResolved,
		Reason: strings.Repeat("x", 1000),
	}
	ev := selectEvidence(inc, nil, defaultMaxEvidence, 80)
	for _, e := range ev {
		if len(e) > 80 {
			t.Fatalf("evidence line exceeds snippet bound: len=%d (%q)", len(e), e)
		}
	}
}

func TestSelectEvidence_ExecutionStatusFallback(t *testing.T) {
	inc := incidents.Incident{
		State:             incidents.StateResolved,
		Reason:            "svc stopped",
		RemediationStatus: "succeeded",
		// no per-action RemediationResults
	}
	ev := selectEvidence(inc, nil, defaultMaxEvidence, defaultMaxSnippetLen)
	if !strings.Contains(joined(ev), "Execution status: succeeded") {
		t.Fatalf("expected execution status fallback: %v", ev)
	}
}
