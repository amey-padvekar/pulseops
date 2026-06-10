package incidents

import (
	"errors"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/store"
)

// validatingIncidentInStore returns an incident that has executed and entered validating,
// with its post-remediation boundary stamped at boundaryAt.
func validatingIncidentInStore(t *testing.T, s *Store, boundaryAt time.Time) string {
	t.Helper()
	id := approvedIncidentInStore(t, s)
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("MarkExecuting failed: %v", err)
	}
	if _, err := s.SaveRemediationResult(
		id,
		ExecutionOutcome{RequestID: "rem-1", Status: "succeeded"},
		boundaryAt,
		StateValidating,
	); err != nil {
		t.Fatalf("SaveRemediationResult failed: %v", err)
	}
	return id
}

func healthyTelemetryAt(seenAt time.Time) store.DeviceState {
	return store.DeviceState{
		DeviceID:         "dev-1",
		ServiceName:      "vpn",
		ServiceStatus:    "running",
		Heartbeat:        true,
		NetworkReachable: true,
		LastSeenAt:       seenAt,
	}
}

func TestRecordValidationObservation_ResolvesAfterRequiredHealthyCycles(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// First healthy cycle: counts but does not resolve (default required = 2).
	out, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("first observation failed: %v", err)
	}
	if !out.Admitted || !out.Healthy {
		t.Fatalf("expected admitted healthy observation, got %+v", out)
	}
	if out.Resolved {
		t.Fatal("expected incident NOT to resolve after a single healthy cycle")
	}
	if out.Incident.State != StateValidating {
		t.Fatalf("expected still validating, got %q", out.Incident.State)
	}
	if out.Incident.HealthyCycleCount != 1 {
		t.Fatalf("expected count 1, got %d", out.Incident.HealthyCycleCount)
	}

	// Second consecutive healthy cycle: reaches the threshold and resolves.
	out, err = s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(2*time.Second)), DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("second observation failed: %v", err)
	}
	if !out.Resolved {
		t.Fatalf("expected resolution after %d healthy cycles, got %+v", DefaultRequiredHealthyCycles, out)
	}
	if out.Incident.State != StateResolved {
		t.Fatalf("expected resolved, got %q", out.Incident.State)
	}
	if out.Incident.Active {
		t.Fatal("expected resolved incident to be inactive")
	}
}

func TestRecordValidationObservation_UnhealthyResetsCounter(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// One healthy cycle, then an unhealthy one resets progress to 0.
	if _, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria()); err != nil {
		t.Fatalf("healthy observation failed: %v", err)
	}

	unhealthy := healthyTelemetryAt(boundary.Add(2 * time.Second))
	unhealthy.ServiceStatus = "stopped"
	out, err := s.RecordValidationObservation(id, unhealthy, DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("unhealthy observation failed: %v", err)
	}
	if !out.Admitted || out.Healthy {
		t.Fatalf("expected admitted unhealthy observation, got %+v", out)
	}
	if out.Incident.HealthyCycleCount != 0 {
		t.Fatalf("expected counter reset to 0, got %d", out.Incident.HealthyCycleCount)
	}
	if out.Incident.State != StateValidating {
		t.Fatalf("expected still validating after unhealthy cycle, got %q", out.Incident.State)
	}

	// A subsequent lone healthy cycle must not resolve — the run restarts at 1.
	out, err = s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(3*time.Second)), DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("post-reset observation failed: %v", err)
	}
	if out.Resolved {
		t.Fatal("expected no resolution after reset + single healthy cycle")
	}
	if out.Incident.HealthyCycleCount != 1 {
		t.Fatalf("expected count 1 after reset, got %d", out.Incident.HealthyCycleCount)
	}
}

func TestRecordValidationObservation_IgnoresStaleTelemetry(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// Telemetry at/just before the boundary is pre-remediation evidence and ignored.
	stale := healthyTelemetryAt(boundary.Add(-time.Second))
	out, err := s.RecordValidationObservation(id, stale, DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("stale observation failed: %v", err)
	}
	if out.Admitted {
		t.Fatal("expected stale telemetry to be rejected")
	}
	if out.Incident.HealthyCycleCount != 0 {
		t.Fatalf("expected counter untouched, got %d", out.Incident.HealthyCycleCount)
	}
	if out.Incident.LastValidationTelemetryAt != nil {
		t.Fatal("expected no validation timestamp recorded for stale telemetry")
	}
}

func TestRecordValidationObservation_IgnoredWhenNotValidating(t *testing.T) {
	s := NewStore()
	// Incident sits in approved (never entered validating).
	id := approvedIncidentInStore(t, s)

	out, err := s.RecordValidationObservation(id, healthyTelemetryAt(time.Now().UTC()), DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("observation failed: %v", err)
	}
	if out.Admitted {
		t.Fatal("expected observation to be ignored when not validating")
	}
	if out.Incident.State != StateApproved {
		t.Fatalf("expected state untouched, got %q", out.Incident.State)
	}
}

func TestRecordValidationObservation_RequiredCyclesOneResolvesImmediately(t *testing.T) {
	s := NewStore()
	boundary := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	id := validatingIncidentInStore(t, s, boundary)

	// Demo timing override: lower the threshold to a single healthy cycle.
	s.setRequiredHealthyCyclesForTest(id, 1)

	out, err := s.RecordValidationObservation(id, healthyTelemetryAt(boundary.Add(time.Second)), DefaultHealthCriteria())
	if err != nil {
		t.Fatalf("observation failed: %v", err)
	}
	if !out.Resolved {
		t.Fatalf("expected immediate resolution with required=1, got %+v", out)
	}
}

func TestRecordValidationObservation_NotFound(t *testing.T) {
	s := NewStore()
	if _, err := s.RecordValidationObservation("missing", healthyTelemetryAt(time.Now().UTC()), DefaultHealthCriteria()); !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}

// setRequiredHealthyCyclesForTest is a tiny test helper to exercise the demo override
// without exposing a production setter prematurely.
func (s *Store) setRequiredHealthyCyclesForTest(incidentID string, n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.byID[incidentID]; ok {
		p.RequiredHealthyCycles = n
	}
}
