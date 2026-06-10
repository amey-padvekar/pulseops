package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
)

// Phase 10 step 4.10 acceptance suite. These tests assert the recovery-validation rules
// end-to-end through the real telemetry HTTP handler and store, locking the closure
// contract before Phase 11 summary logic depends on it. Each required scenario maps to a
// test here or to the named unit test noted alongside it:
//
//	remediation success + healthy telemetry resolves ....... TestAcceptance_SuccessThenHealthyTelemetryResolves
//	one healthy cycle does not resolve (two required) ...... TestAcceptance_SuccessThenHealthyTelemetryResolves (mid-assertion)
//	stale telemetry is ignored ............................ TestTelemetryPipeline_IgnoresStaleTelemetryForValidation
//	unhealthy telemetry -> failure/timeout path ........... TestAcceptance_UnhealthyDuringValidationTimesOutToFailed
//	no fresh telemetry -> failure after timeout ........... TestAcceptance_NoFreshTelemetryTimesOutToFailed
//	command success without healthy telemetry no resolve .. TestAcceptance_CommandSuccessWithoutHealthyTelemetryDoesNotResolve
//	valid lifecycle transitions ........................... internal/incidents/state_machine_test.go (Phase10 tests)

func stoppedTelemetryBody(deviceID, serviceName string) string {
	return fmt.Sprintf(
		`{"schemaVersion":"1.0.0","deviceId":%q,"timestamp":"2026-06-05T10:00:00Z","heartbeat":true,"serviceName":%q,"serviceStatus":"stopped","networkReachable":true,"cpuUsage":12.0,"memoryUsage":40.0,"recentLogs":["still down"]}`,
		deviceID, serviceName,
	)
}

func TestAcceptance_SuccessThenHealthyTelemetryResolves(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "ACPT-01", "OpenVPNService"
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-time.Minute))

	body := healthyTelemetryBody(deviceID, serviceName)

	// One healthy cycle must NOT resolve when two are required.
	postTelemetry(t, handler, body)
	if inc, _ := incidentStore.GetByID(id); inc.State != incidents.StateValidating || inc.HealthyCycleCount != 1 {
		t.Fatalf("after one healthy cycle: state=%q count=%d, want validating/1", inc.State, inc.HealthyCycleCount)
	}

	// The second consecutive healthy cycle resolves the incident.
	postTelemetry(t, handler, body)
	inc, _ := incidentStore.GetByID(id)
	if inc.State != incidents.StateResolved {
		t.Fatalf("after two healthy cycles: state=%q, want resolved", inc.State)
	}
	if inc.ValidationStatus != incidents.ValidationStatusSucceeded || inc.ValidatedAt == nil {
		t.Fatalf("expected succeeded validation evidence, got status=%q validatedAt=%v", inc.ValidationStatus, inc.ValidatedAt)
	}
	if inc.Active {
		t.Fatal("expected resolved incident to be inactive")
	}
}

func TestAcceptance_CommandSuccessWithoutHealthyTelemetryDoesNotResolve(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "ACPT-02", "OpenVPNService"
	// Remediation reported success (incident is validating), but the service never
	// actually recovers — every post-remediation cycle is still unhealthy.
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-time.Minute))

	for i := 0; i < 3; i++ {
		postTelemetry(t, handler, stoppedTelemetryBody(deviceID, serviceName))
		inc, _ := incidentStore.GetByID(id)
		if inc.State != incidents.StateValidating {
			t.Fatalf("cycle %d: state=%q, want validating (command success must not resolve without healthy telemetry)", i, inc.State)
		}
		if inc.HealthyCycleCount != 0 {
			t.Fatalf("cycle %d: healthyCycleCount=%d, want 0", i, inc.HealthyCycleCount)
		}
	}
}

func TestAcceptance_UnhealthyDuringValidationTimesOutToFailed(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "ACPT-03", "OpenVPNService"
	// Boundary far enough in the past that the validation window has already elapsed,
	// while live telemetry (stamped ~now) still counts as fresh post-remediation evidence.
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-2*incidents.DefaultValidationTimeout))

	// A fresh but unhealthy observation is recorded (service still stopped).
	postTelemetry(t, handler, stoppedTelemetryBody(deviceID, serviceName))
	if inc, _ := incidentStore.GetByID(id); inc.State != incidents.StateValidating {
		t.Fatalf("before timeout sweep: state=%q, want validating", inc.State)
	}

	// The timeout sweep then fails the incident, citing the unhealthy evidence.
	failed := incidentStore.ExpireTimedOutValidations(time.Now().UTC(), incidents.DefaultValidationTimeout)
	if len(failed) != 1 || failed[0].IncidentID != id {
		t.Fatalf("expected the incident to be failed by the sweep, got %+v", failed)
	}

	inc, _ := incidentStore.GetByID(id)
	if inc.State != incidents.StateFailed {
		t.Fatalf("state=%q, want failed", inc.State)
	}
	if inc.ValidationStatus != incidents.ValidationStatusFailed || inc.ValidatedAt == nil {
		t.Fatalf("expected failed validation evidence, got status=%q validatedAt=%v", inc.ValidationStatus, inc.ValidatedAt)
	}
	if !strings.Contains(inc.ValidationFailureReason, "stopped") {
		t.Fatalf("expected failure reason to cite the stopped service, got %q", inc.ValidationFailureReason)
	}
	if inc.Active {
		t.Fatal("expected failed incident to be inactive")
	}
}

func TestAcceptance_NoFreshTelemetryTimesOutToFailed(t *testing.T) {
	incidentStore := incidents.NewStore()

	const deviceID, serviceName = "ACPT-04", "OpenVPNService"
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-2*incidents.DefaultValidationTimeout))

	// No telemetry arrives at all after remediation; the sweep must still fail it.
	failed := incidentStore.ExpireTimedOutValidations(time.Now().UTC(), incidents.DefaultValidationTimeout)
	if len(failed) != 1 || failed[0].IncidentID != id {
		t.Fatalf("expected the stalled incident to be failed by the sweep, got %+v", failed)
	}

	inc, _ := incidentStore.GetByID(id)
	if inc.State != incidents.StateFailed {
		t.Fatalf("state=%q, want failed", inc.State)
	}
	if !strings.Contains(inc.ValidationFailureReason, "no fresh telemetry") {
		t.Fatalf("expected 'no fresh telemetry' reason, got %q", inc.ValidationFailureReason)
	}
}
