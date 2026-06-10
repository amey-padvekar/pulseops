package agentbuilder

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// seedIncidentInState inserts a richly-populated incident in the given lifecycle state
// into a real *incidents.Store, so summary generation can be exercised against actual
// storage and retrieval rather than a fake (Phase 11 step 4.11).
func seedIncidentInState(store *incidents.Store, id string, state incidents.IncidentState) incidents.Incident {
	seed := incidents.Incident{
		IncidentID:        id,
		DeviceID:          "dev-1",
		ServiceName:       "OpenVPNService",
		ServiceStatus:     "stopped",
		State:             state,
		Severity:          incidents.SeverityHigh,
		Reason:            "service status is stopped",
		ProbableCause:     "OpenVPN service unexpectedly stopped",
		Confidence:        0.9,
		Summary:           "service stopped while heartbeat remained",
		ApprovedActions:   []string{"restart_service"},
		RemediationStatus: "succeeded",
		RemediationResults: []incidents.RemediationActionResult{
			{ActionID: "restart_service", Status: "succeeded"},
		},
	}
	inc, _ := store.CreateOrGetActive("key|"+id, seed)
	return inc
}

// TestSummaryGeneration_OnlyAfterClosure_RealStore drives the orchestrator against a real
// incident store across every lifecycle state and asserts a summary is produced only for
// the terminal (resolved/failed) states (step 4.11 task 2). It uses a nil submitter so the
// deterministic fallback is exercised — proving the stored summary is grounded entirely in
// stored incident data with no network dependency.
func TestSummaryGeneration_OnlyAfterClosure_RealStore(t *testing.T) {
	nonTerminal := []incidents.IncidentState{
		incidents.StateHealthy,
		incidents.StateDetected,
		incidents.StateInvestigating,
		incidents.StateAwaitingApproval,
		incidents.StateApproved,
		incidents.StateExecuting,
		incidents.StateValidating,
	}

	for i, state := range nonTerminal {
		t.Run("non_terminal_"+string(state), func(t *testing.T) {
			store := incidents.NewStore()
			id := "inc-nt-" + string(rune('a'+i))
			seedIncidentInState(store, id, state)

			_, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, id, SummaryGenerationConfig{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if changed {
				t.Fatalf("summary must not be generated in state %q", state)
			}

			// Retrieval confirms nothing was persisted.
			got, ok := store.GetByID(id)
			if !ok {
				t.Fatal("incident vanished")
			}
			if got.FinalSummary != nil || got.SummaryStatus != "" {
				t.Fatalf("no summary should be stored for %q, got status=%q summary=%v", state, got.SummaryStatus, got.FinalSummary)
			}
		})
	}

	terminal := []struct {
		state          incidents.IncidentState
		wantResultWord string
	}{
		{incidents.StateResolved, "resolved"},
		{incidents.StateFailed, "fail"},
	}

	for i, tc := range terminal {
		t.Run("terminal_"+string(tc.state), func(t *testing.T) {
			store := incidents.NewStore()
			id := "inc-t-" + string(rune('a'+i))
			seedIncidentInState(store, id, tc.state)

			updated, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, id, SummaryGenerationConfig{Now: time.Unix(2000, 0).UTC()})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !changed {
				t.Fatalf("summary must be generated in terminal state %q", tc.state)
			}

			// No live backend -> deterministic fallback persisted.
			if updated.SummaryStatus != SummaryStatusFallback {
				t.Fatalf("status = %q, want fallback", updated.SummaryStatus)
			}

			// Retrieval after save: the summary survives as durable incident truth.
			got, ok := store.GetByID(id)
			if !ok || got.FinalSummary == nil {
				t.Fatalf("summary not retrievable after generation: ok=%v summary=%v", ok, got.FinalSummary)
			}
			fs := got.FinalSummary

			// Grounded in stored incident data: root cause mirrors the stored probable cause.
			if fs.RootCause != "OpenVPN service unexpectedly stopped" {
				t.Fatalf("root cause not grounded in stored data: %q", fs.RootCause)
			}
			if len(fs.Evidence) == 0 || len(fs.ActionsTaken) == 0 || fs.Result == "" {
				t.Fatalf("summary fields incomplete: %+v", fs)
			}
			// Result reflects the actual terminal outcome (resolved vs failed).
			if !strings.Contains(strings.ToLower(fs.Result), tc.wantResultWord) {
				t.Fatalf("result %q does not reflect %q outcome", fs.Result, tc.state)
			}
			if got.SummaryGeneratedAt == nil {
				t.Fatal("expected summaryGeneratedAt set")
			}
		})
	}
}

// TestSummaryGeneration_IdempotentAcrossRefresh_RealStore proves that re-running automatic
// generation (e.g. a duplicate closure event or page refresh) against the real store does
// not overwrite an existing summary (step 4.11 task 2 / idempotency).
func TestSummaryGeneration_IdempotentAcrossRefresh_RealStore(t *testing.T) {
	store := incidents.NewStore()
	seedIncidentInState(store, "inc-1", incidents.StateResolved)

	first, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, "inc-1", SummaryGenerationConfig{})
	if err != nil || !changed {
		t.Fatalf("first generation should succeed: changed=%v err=%v", changed, err)
	}
	firstGenAt := first.SummaryGeneratedAt

	// Second automatic pass must be a no-op.
	_, changed2, err := GenerateAndStoreSummary(context.Background(), store, nil, "inc-1", SummaryGenerationConfig{Mode: SummaryTriggerAuto})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed2 {
		t.Fatal("auto regeneration after refresh must be a no-op")
	}

	got, _ := store.GetByID("inc-1")
	if got.SummaryGeneratedAt == nil || firstGenAt == nil || !got.SummaryGeneratedAt.Equal(*firstGenAt) {
		t.Fatal("summary should be unchanged after idempotent no-op")
	}
}
