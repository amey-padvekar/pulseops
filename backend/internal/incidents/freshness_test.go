package incidents

import (
	"testing"
	"time"
)

func TestIsTelemetryFresh_StrictlyAfterBoundary(t *testing.T) {
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)

	if !IsTelemetryFresh(boundary, boundary.Add(time.Second)) {
		t.Fatal("expected telemetry after the boundary to be fresh")
	}
	if IsTelemetryFresh(boundary, boundary.Add(-time.Second)) {
		t.Fatal("expected telemetry before the boundary to be stale")
	}
	if IsTelemetryFresh(boundary, boundary) {
		t.Fatal("expected telemetry exactly at the boundary to be stale")
	}
}

func TestIsTelemetryFresh_NormalizesToUTC(t *testing.T) {
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	// Same instant expressed in a +05:30 zone must not read as "after".
	loc := time.FixedZone("IST", 5*3600+30*60)
	sameInstant := boundary.In(loc)
	if IsTelemetryFresh(boundary, sameInstant) {
		t.Fatal("expected identical instant in another zone to be stale, not fresh")
	}

	laterInstant := boundary.Add(time.Minute).In(loc)
	if !IsTelemetryFresh(boundary, laterInstant) {
		t.Fatal("expected a later instant in another zone to be fresh")
	}
}

func TestAcceptsTelemetryAt_RequiresBoundary(t *testing.T) {
	now := time.Now().UTC()

	var noBoundary Incident
	if noBoundary.AcceptsTelemetryAt(now) {
		t.Fatal("expected no telemetry to be accepted before a validation boundary is set")
	}

	b := now
	withBoundary := Incident{ValidationBoundaryAt: &b}
	if withBoundary.AcceptsTelemetryAt(now.Add(-time.Second)) {
		t.Fatal("expected pre-boundary telemetry to be rejected")
	}
	if !withBoundary.AcceptsTelemetryAt(now.Add(time.Second)) {
		t.Fatal("expected post-boundary telemetry to be accepted")
	}
}

func TestSaveRemediationResult_StampsValidationBoundary(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting failed: %v", err)
	}

	receivedAt := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	updated, err := s.SaveRemediationResult(
		id,
		ExecutionOutcome{RequestID: "rem-1", Status: "succeeded"},
		receivedAt,
		StateValidating,
	)
	if err != nil {
		t.Fatalf("SaveRemediationResult failed: %v", err)
	}
	if updated.State != StateValidating {
		t.Fatalf("expected validating, got %q", updated.State)
	}
	if updated.ValidationBoundaryAt == nil {
		t.Fatal("expected validation boundary to be stamped on entering validating")
	}
	if !updated.ValidationBoundaryAt.Equal(receivedAt) {
		t.Fatalf("expected boundary %v, got %v", receivedAt, *updated.ValidationBoundaryAt)
	}

	// Fresh telemetry after the boundary is admissible; stale telemetry is not.
	if !updated.AcceptsTelemetryAt(receivedAt.Add(time.Second)) {
		t.Fatal("expected post-boundary telemetry to be accepted")
	}
	if updated.AcceptsTelemetryAt(receivedAt.Add(-time.Second)) {
		t.Fatal("expected pre-boundary telemetry to be rejected")
	}
}

func TestSaveRemediationResult_FailedDoesNotStampBoundary(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting failed: %v", err)
	}

	updated, err := s.SaveRemediationResult(
		id,
		ExecutionOutcome{RequestID: "rem-1", Status: "failed"},
		time.Now().UTC(),
		StateFailed,
	)
	if err != nil {
		t.Fatalf("SaveRemediationResult failed: %v", err)
	}
	if updated.ValidationBoundaryAt != nil {
		t.Fatalf("expected no validation boundary on the failure path, got %v", *updated.ValidationBoundaryAt)
	}
}

func TestUpdateState_StampsValidationBoundaryOnValidating(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting failed: %v", err)
	}

	updated, err := s.UpdateState(id, StateValidating, "begin validation")
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}
	if updated.ValidationBoundaryAt == nil {
		t.Fatal("expected validation boundary to be stamped via UpdateState")
	}
}

func TestSetValidationBoundary_Idempotent(t *testing.T) {
	first := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	inc := &Incident{}

	setValidationBoundaryLocked(inc, StateValidating, first)
	if inc.ValidationBoundaryAt == nil || !inc.ValidationBoundaryAt.Equal(first) {
		t.Fatalf("expected boundary %v, got %v", first, inc.ValidationBoundaryAt)
	}

	// A second entry into validating must not move the window.
	setValidationBoundaryLocked(inc, StateValidating, first.Add(time.Hour))
	if !inc.ValidationBoundaryAt.Equal(first) {
		t.Fatalf("expected boundary to remain %v, got %v", first, *inc.ValidationBoundaryAt)
	}
}
