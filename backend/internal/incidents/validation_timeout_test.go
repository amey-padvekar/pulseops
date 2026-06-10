package incidents

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// validatingIncidentInStoreWithDevice seeds a second incident on a distinct device/key,
// drives it through the legal lifecycle into validating, and stamps its boundary.
func validatingIncidentInStoreWithDevice(t *testing.T, s *Store, deviceID string, boundaryAt time.Time) string {
	t.Helper()
	seed := NewIncident("", deviceID, "OpenVPNService", "stopped", SeverityHigh, "service down")
	inc, _ := s.CreateOrGetActive(deviceID+":OpenVPNService", seed)
	if _, err := s.UpdateState(inc.IncidentID, StateInvestigating, ""); err != nil {
		t.Fatalf("to investigating: %v", err)
	}
	s.byID[inc.IncidentID].RecommendedActions = []RecommendedAction{{ActionID: "restart_service", Target: "OpenVPNService"}}
	if _, _, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := s.Approve(inc.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, _, err := s.MarkExecuting(inc.IncidentID, "rem-"+deviceID); err != nil {
		t.Fatalf("mark executing: %v", err)
	}
	if _, err := s.SaveRemediationResult(
		inc.IncidentID,
		ExecutionOutcome{RequestID: "rem-" + deviceID, Status: "succeeded"},
		boundaryAt,
		StateValidating,
	); err != nil {
		t.Fatalf("save remediation result: %v", err)
	}
	return inc.IncidentID
}

func TestExpireValidation_NotTimedOutIsNoOp(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// One second past the boundary, well inside the window: no change.
	updated, changed, err := s.ExpireValidationIfTimedOut(id, boundary.Add(time.Second), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if changed {
		t.Fatal("expected no change before the timeout elapses")
	}
	if updated.State != StateValidating {
		t.Fatalf("expected still validating, got %q", updated.State)
	}
}

func TestExpireValidation_TimesOutWithNoFreshTelemetry(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	updated, changed, err := s.ExpireValidationIfTimedOut(id, boundary.Add(DefaultValidationTimeout+time.Second), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if !changed {
		t.Fatal("expected the incident to be failed on timeout")
	}
	if updated.State != StateFailed {
		t.Fatalf("expected failed, got %q", updated.State)
	}
	if updated.Active {
		t.Fatal("expected failed incident to be inactive")
	}
	if !strings.Contains(updated.ValidationFailureReason, "no fresh telemetry") {
		t.Fatalf("expected 'no fresh telemetry' reason, got %q", updated.ValidationFailureReason)
	}
	if updated.Reason != updated.ValidationFailureReason {
		t.Fatal("expected the incident reason to surface the failure cause")
	}
}

func TestExpireValidation_ReasonCitesLastUnhealthyObservation(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// A fresh but unhealthy observation: service still stopped.
	unhealthy := healthyTelemetryAt(boundary.Add(time.Second))
	unhealthy.ServiceStatus = "stopped"
	if _, err := s.RecordValidationObservation(id, unhealthy, DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}

	updated, changed, err := s.ExpireValidationIfTimedOut(id, boundary.Add(DefaultValidationTimeout+time.Second), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if !changed || updated.State != StateFailed {
		t.Fatalf("expected failed incident, got changed=%v state=%q", changed, updated.State)
	}
	if !strings.Contains(updated.ValidationFailureReason, "stopped") {
		t.Fatalf("expected reason to cite stopped service, got %q", updated.ValidationFailureReason)
	}
}

func TestExpireValidation_ReasonCitesPartialHealthyRun(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// One healthy cycle (required is 2), then the next never arrives before timeout.
	if _, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}

	updated, changed, err := s.ExpireValidationIfTimedOut(id, boundary.Add(DefaultValidationTimeout+time.Second), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if !changed || updated.State != StateFailed {
		t.Fatalf("expected failed incident, got changed=%v state=%q", changed, updated.State)
	}
	if !strings.Contains(updated.ValidationFailureReason, "1 of 2") {
		t.Fatalf("expected reason to cite partial healthy run, got %q", updated.ValidationFailureReason)
	}
}

func TestExpireValidation_IgnoresNonValidatingIncidents(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s) // never entered validating

	updated, changed, err := s.ExpireValidationIfTimedOut(id, time.Now().UTC().Add(time.Hour), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if changed {
		t.Fatal("expected non-validating incident to be untouched")
	}
	if updated.State != StateApproved {
		t.Fatalf("expected approved, got %q", updated.State)
	}
}

func TestExpireValidation_DoesNotFailAlreadyResolved(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)
	s.setRequiredHealthyCyclesForTest(id, 1)
	if _, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}

	// Long after the would-be window, a resolved incident must not be reopened/failed.
	updated, changed, err := s.ExpireValidationIfTimedOut(id, boundary.Add(time.Hour), DefaultValidationTimeout)
	if err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}
	if changed {
		t.Fatal("expected resolved incident to be untouched by timeout sweep")
	}
	if updated.State != StateResolved {
		t.Fatalf("expected resolved, got %q", updated.State)
	}
}

func TestExpireTimedOutValidations_SweepsOnlyExpired(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

	expiredID := validatingIncidentInStore(t, s, boundary)
	// A second incident with a much later boundary (different dedupe key via device).
	freshID := validatingIncidentInStoreWithDevice(t, s, "dev-2", boundary.Add(DefaultValidationTimeout))

	now := boundary.Add(DefaultValidationTimeout + time.Second)
	failed := s.ExpireTimedOutValidations(now, DefaultValidationTimeout)

	if len(failed) != 1 {
		t.Fatalf("expected exactly 1 failed incident, got %d", len(failed))
	}
	if failed[0].IncidentID != expiredID {
		t.Fatalf("expected %s to fail, got %s", expiredID, failed[0].IncidentID)
	}

	fresh, _ := s.GetByID(freshID)
	if fresh.State != StateValidating {
		t.Fatalf("expected the fresh incident to remain validating, got %q", fresh.State)
	}
}

func TestExpireTimedOutValidations_DefaultsTimeout(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// Passing a non-positive timeout falls back to DefaultValidationTimeout.
	failed := s.ExpireTimedOutValidations(boundary.Add(DefaultValidationTimeout+time.Second), 0)
	if len(failed) != 1 || failed[0].IncidentID != id {
		t.Fatalf("expected the incident to fail under the default timeout, got %+v", failed)
	}
}

func TestExpireValidation_NotFound(t *testing.T) {
	s := NewStore()
	if _, _, err := s.ExpireValidationIfTimedOut("missing", time.Now().UTC(), DefaultValidationTimeout); !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}
