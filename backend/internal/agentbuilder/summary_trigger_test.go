package agentbuilder

import (
	"testing"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func TestIsSummaryTriggerState(t *testing.T) {
	allowed := []incidents.IncidentState{incidents.StateResolved, incidents.StateFailed}
	for _, s := range allowed {
		if !IsSummaryTriggerState(s) {
			t.Errorf("state %q should be a trigger state", s)
		}
	}

	blocked := []incidents.IncidentState{
		incidents.StateHealthy,
		incidents.StateDetected,
		incidents.StateInvestigating,
		incidents.StateAwaitingApproval,
		incidents.StateApproved,
		incidents.StateExecuting,
		incidents.StateValidating,
	}
	for _, s := range blocked {
		if IsSummaryTriggerState(s) {
			t.Errorf("state %q must not be a trigger state", s)
		}
	}
}

func TestEvaluateSummaryTrigger_NonTerminalRefused(t *testing.T) {
	for _, s := range []incidents.IncidentState{
		incidents.StateInvestigating,
		incidents.StateAwaitingApproval,
		incidents.StateApproved,
		incidents.StateExecuting,
		incidents.StateValidating,
	} {
		d := EvaluateSummaryTrigger(s, SummaryStatusNone, SummaryTriggerAuto)
		if d.Allowed {
			t.Errorf("state %q should not allow generation", s)
		}
		if d.Reason == "" {
			t.Errorf("expected a reason for refusal in state %q", s)
		}
	}
}

func TestEvaluateSummaryTrigger_TerminalFreshAllowed(t *testing.T) {
	for _, s := range []incidents.IncidentState{incidents.StateResolved, incidents.StateFailed} {
		d := EvaluateSummaryTrigger(s, SummaryStatusNone, SummaryTriggerAuto)
		if !d.Allowed {
			t.Errorf("fresh terminal state %q should allow generation: %s", s, d.Reason)
		}
	}
}

func TestEvaluateSummaryTrigger_Idempotency(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		mode        SummaryTriggerMode
		wantAllowed bool
	}{
		{"auto skips generated", SummaryStatusGenerated, SummaryTriggerAuto, false},
		{"auto skips pending", SummaryStatusPending, SummaryTriggerAuto, false},
		{"auto retries failed", SummaryStatusFailed, SummaryTriggerAuto, true},
		{"auto generates none", SummaryStatusNone, SummaryTriggerAuto, true},
		{"operator regenerates generated", SummaryStatusGenerated, SummaryTriggerOperator, true},
		{"operator overrides pending", SummaryStatusPending, SummaryTriggerOperator, true},
		{"operator retries failed", SummaryStatusFailed, SummaryTriggerOperator, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := EvaluateSummaryTrigger(incidents.StateResolved, tc.status, tc.mode)
			if d.Allowed != tc.wantAllowed {
				t.Fatalf("allowed = %v, want %v (reason: %s)", d.Allowed, tc.wantAllowed, d.Reason)
			}
			if d.Reason == "" {
				t.Fatalf("expected a non-empty reason")
			}
		})
	}
}

func TestEvaluateSummaryTrigger_TerminalRequiredEvenForOperator(t *testing.T) {
	// An explicit operator trigger still cannot summarize a non-terminal incident.
	d := EvaluateSummaryTrigger(incidents.StateValidating, SummaryStatusNone, SummaryTriggerOperator)
	if d.Allowed {
		t.Fatalf("operator trigger must still require a terminal state: %s", d.Reason)
	}
}
