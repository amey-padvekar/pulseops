package incidents

import (
	"errors"
	"testing"
	"time"
)

// approvedIncidentInStore seeds an approved incident and returns its ID.
func approvedIncidentInStore(t *testing.T, s *Store) string {
	t.Helper()
	seed := NewIncident("INC-1", "dev-1", "OpenVPNService", "stopped", SeverityHigh, "service down")
	inc, _ := s.CreateOrGetActive("dev-1:OpenVPNService", seed)
	// Drive it to approved via the legal path: detected -> investigating -> awaiting -> approved.
	if _, err := s.UpdateState(inc.IncidentID, StateInvestigating, ""); err != nil {
		t.Fatalf("to investigating: %v", err)
	}
	p := s.byID[inc.IncidentID]
	p.RecommendedActions = []RecommendedAction{{ActionID: "restart_service", Target: "OpenVPNService"}}
	if _, _, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := s.Approve(inc.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return inc.IncidentID
}

func TestMarkExecuting_FromApproved(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)

	updated, changed, err := s.MarkExecuting(id, "rem-1")
	if err != nil {
		t.Fatalf("MarkExecuting: %v", err)
	}
	if !changed || updated.State != StateExecuting {
		t.Fatalf("expected transition to executing, got changed=%v state=%q", changed, updated.State)
	}

	// Idempotent: already executing -> no change, no error.
	_, changed2, err := s.MarkExecuting(id, "rem-1")
	if err != nil || changed2 {
		t.Fatalf("second MarkExecuting should be a no-op: changed=%v err=%v", changed2, err)
	}
}

func TestMarkExecuting_NotFound(t *testing.T) {
	s := NewStore()
	if _, _, err := s.MarkExecuting("nope", "rem-1"); !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}

func TestSaveRemediationResult_SuccessMovesToValidating(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting: %v", err)
	}

	exit := 0
	outcome := ExecutionOutcome{
		RequestID:  "rem-1",
		Status:     "succeeded",
		StartedAt:  time.Date(2026, 5, 23, 22, 10, 6, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 23, 22, 10, 8, 0, time.UTC),
		Actions:    []RemediationActionResult{{ActionID: "restart_service", Status: "succeeded", Stdout: "ok", ExitCode: &exit, DurationMs: 1500}},
	}
	updated, err := s.SaveRemediationResult(id, outcome, time.Now().UTC(), StateValidating)
	if err != nil {
		t.Fatalf("SaveRemediationResult: %v", err)
	}
	if updated.State != StateValidating {
		t.Fatalf("state: got %q want validating", updated.State)
	}
	if updated.RemediationStatus != "succeeded" || updated.RemediationRequestID != "rem-1" {
		t.Fatalf("outcome not persisted: %+v", updated)
	}
	if updated.RemediationStartedAt == nil || updated.RemediationFinishedAt == nil || updated.RemediationReceivedAt == nil {
		t.Fatalf("timestamps not persisted: %+v", updated)
	}
	if len(updated.RemediationResults) != 1 || updated.RemediationResults[0].DurationMs != 1500 {
		t.Fatalf("per-action results not persisted: %+v", updated.RemediationResults)
	}
	if !updated.Active {
		t.Fatal("validating incident should remain active")
	}
}

func TestSaveRemediationResult_FailureMovesToFailedAndDeactivates(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting: %v", err)
	}

	outcome := ExecutionOutcome{RequestID: "rem-1", Status: "failed"}
	updated, err := s.SaveRemediationResult(id, outcome, time.Now().UTC(), StateFailed)
	if err != nil {
		t.Fatalf("SaveRemediationResult: %v", err)
	}
	if updated.State != StateFailed || updated.Active {
		t.Fatalf("expected failed+inactive, got state=%q active=%v", updated.State, updated.Active)
	}
}

func TestSaveRemediationResult_ImplicitExecutingFromApproved(t *testing.T) {
	// If dispatch never marked executing, the result still applies: approved -> executing
	// -> validating in one call.
	s := NewStore()
	id := approvedIncidentInStore(t, s)

	updated, err := s.SaveRemediationResult(id, ExecutionOutcome{RequestID: "rem-1", Status: "succeeded"}, time.Now().UTC(), StateValidating)
	if err != nil {
		t.Fatalf("SaveRemediationResult: %v", err)
	}
	if updated.State != StateValidating {
		t.Fatalf("state: got %q want validating", updated.State)
	}
}

func TestSaveRemediationResult_RejectsIllegalTransition(t *testing.T) {
	// A resolved incident cannot accept a result that moves it to validating; the
	// incident must be left untouched.
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	s.Resolve(id)

	if _, err := s.SaveRemediationResult(id, ExecutionOutcome{RequestID: "rem-1", Status: "succeeded"}, time.Now().UTC(), StateValidating); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
	got, _ := s.GetByID(id)
	if got.RemediationStatus != "" {
		t.Fatalf("result should not have been persisted on illegal transition: %+v", got)
	}
}
