package incidents

import (
	"testing"
	"time"
)

func TestValidationEvidence_StatusInProgressOnEnteringValidating(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	inc, _ := s.GetByID(id)
	if inc.ValidationStatus != ValidationStatusInProgress {
		t.Fatalf("validationStatus = %q, want %q", inc.ValidationStatus, ValidationStatusInProgress)
	}
	if inc.ValidationBoundaryAt == nil {
		t.Fatal("expected validation boundary (start) to be set")
	}
	if inc.ValidatedAt != nil {
		t.Fatal("expected validatedAt to be unset while validation is in progress")
	}
}

func TestValidationEvidence_SnapshotRecordedPerObservation(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	state := healthyTelemetryAt(boundary.Add(time.Second))
	state.ServiceStatus = "stopped" // unhealthy, so it records without resolving
	if _, err := s.RecordValidationObservation(id, state, DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}

	inc, _ := s.GetByID(id)
	snap := inc.LastValidationSnapshot
	if snap == nil {
		t.Fatal("expected a validation snapshot to be recorded")
	}
	if snap.Healthy {
		t.Fatal("expected snapshot to be marked unhealthy")
	}
	if snap.ServiceStatus != "stopped" {
		t.Fatalf("snapshot serviceStatus = %q, want stopped", snap.ServiceStatus)
	}
	if !snap.ObservedAt.Equal(boundary.Add(time.Second)) {
		t.Fatalf("snapshot observedAt = %v, want %v", snap.ObservedAt, boundary.Add(time.Second))
	}
	if len(snap.Checks) == 0 {
		t.Fatal("expected per-criterion checks in the snapshot")
	}
}

func TestValidationEvidence_SucceededOnResolution(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)
	s.setRequiredHealthyCyclesForTest(id, 1)

	if _, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}

	inc, _ := s.GetByID(id)
	if inc.State != StateResolved {
		t.Fatalf("state = %q, want resolved", inc.State)
	}
	if inc.ValidationStatus != ValidationStatusSucceeded {
		t.Fatalf("validationStatus = %q, want %q", inc.ValidationStatus, ValidationStatusSucceeded)
	}
	if inc.ValidatedAt == nil {
		t.Fatal("expected validatedAt to be set on resolution")
	}
	if inc.LastValidationSnapshot == nil || !inc.LastValidationSnapshot.Healthy {
		t.Fatal("expected a healthy snapshot to back the resolution")
	}
}

func TestValidationEvidence_FailedOnTimeout(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	if _, _, err := s.ExpireValidationIfTimedOut(id, boundary.Add(DefaultValidationTimeout+time.Second), DefaultValidationTimeout); err != nil {
		t.Fatalf("ExpireValidationIfTimedOut failed: %v", err)
	}

	inc, _ := s.GetByID(id)
	if inc.ValidationStatus != ValidationStatusFailed {
		t.Fatalf("validationStatus = %q, want %q", inc.ValidationStatus, ValidationStatusFailed)
	}
	if inc.ValidatedAt == nil {
		t.Fatal("expected validatedAt to be set on failure")
	}
	if inc.ValidationFailureReason == "" {
		t.Fatal("expected a failure reason recorded")
	}
}

func TestValidationEvidence_SeparateFromExecutionAndDiagnosis(t *testing.T) {
	// The validation snapshot must not depend on or overwrite execution-result or AI
	// diagnosis fields; they are independent regions of the incident record.
	s := NewStore()
	boundary := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	before, _ := s.GetByID(id)
	if _, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria()); err != nil {
		t.Fatalf("RecordValidationObservation failed: %v", err)
	}
	after, _ := s.GetByID(id)

	if after.RemediationStatus != before.RemediationStatus {
		t.Fatal("validation must not alter execution-result fields")
	}
	if len(after.RemediationResults) != len(before.RemediationResults) {
		t.Fatal("validation must not alter execution results")
	}
	if after.ProbableCause != before.ProbableCause || after.Summary != before.Summary {
		t.Fatal("validation must not alter AI diagnosis fields")
	}
}
