package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
)

// driveToValidating builds an incident and walks it through the legal lifecycle into
// validating via the store's public API, stamping its post-remediation boundary in the
// past so subsequent live telemetry counts as fresh.
func driveToValidating(t *testing.T, s *incidents.Store, deviceID, serviceName string, boundaryAt time.Time) string {
	t.Helper()
	seed := incidents.NewIncident("", deviceID, serviceName, "stopped", incidents.SeverityHigh, "service down")
	// Use the detector's real dedupe key so still-stopped telemetry during validation
	// reuses this incident rather than spawning a duplicate, matching production flow.
	inc, _ := s.CreateOrGetActive(deviceID+"|"+serviceName+"|service_stopped", seed)
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, ""); err != nil {
		t.Fatalf("to investigating: %v", err)
	}
	if _, err := s.SaveInvestigationResult(
		inc.IncidentID, "service crashed", 0.9,
		[]incidents.RecommendedAction{{ActionID: "restart_service", Target: serviceName}},
		nil, "summary", time.Now().UTC(), "", "completed",
	); err != nil {
		t.Fatalf("save investigation: %v", err)
	}
	if _, _, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := s.Approve(inc.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, _, err := s.MarkExecuting(inc.IncidentID, "rem-1"); err != nil {
		t.Fatalf("mark executing: %v", err)
	}
	if _, err := s.SaveRemediationResult(
		inc.IncidentID,
		incidents.ExecutionOutcome{RequestID: "rem-1", Status: "succeeded"},
		boundaryAt,
		incidents.StateValidating,
	); err != nil {
		t.Fatalf("save remediation result: %v", err)
	}
	return inc.IncidentID
}

func healthyTelemetryBody(deviceID, serviceName string) string {
	return fmt.Sprintf(
		`{"schemaVersion":"1.0.0","deviceId":%q,"timestamp":"2026-06-05T10:00:00Z","heartbeat":true,"serviceName":%q,"serviceStatus":"running","networkReachable":true,"cpuUsage":10.0,"memoryUsage":40.0,"recentLogs":["service running"]}`,
		deviceID, serviceName,
	)
}

func postTelemetry(t *testing.T, handler http.HandlerFunc, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	resp := httptest.NewRecorder()
	handler(resp, req)
	if resp.Code != http.StatusAccepted {
		t.Fatalf("telemetry status = %d, want %d", resp.Code, http.StatusAccepted)
	}
}

func TestTelemetryPipeline_ResolvesValidatingIncidentAfterHealthyCycles(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "LAPTOP-22", "OpenVPNService"
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-time.Minute))

	body := healthyTelemetryBody(deviceID, serviceName)

	// First healthy cycle: still validating (default required = 2).
	postTelemetry(t, handler, body)
	inc, _ := incidentStore.GetByID(id)
	if inc.State != incidents.StateValidating {
		t.Fatalf("after 1 cycle state = %q, want validating", inc.State)
	}
	if inc.HealthyCycleCount != 1 {
		t.Fatalf("after 1 cycle count = %d, want 1", inc.HealthyCycleCount)
	}

	// Second healthy cycle: reaches the threshold and resolves.
	postTelemetry(t, handler, body)
	inc, _ = incidentStore.GetByID(id)
	if inc.State != incidents.StateResolved {
		t.Fatalf("after 2 cycles state = %q, want resolved", inc.State)
	}
	if inc.Active {
		t.Fatal("expected resolved incident to be inactive")
	}
}

func TestTelemetryPipeline_StillStoppedDoesNotResolve(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "LAPTOP-22", "OpenVPNService"
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-time.Minute))

	// Service still stopped after remediation: an unhealthy fresh observation must not
	// resolve, and the incident stays in validating (failure is timeout-driven).
	body := fmt.Sprintf(
		`{"schemaVersion":"1.0.0","deviceId":%q,"timestamp":"2026-06-05T10:00:00Z","heartbeat":true,"serviceName":%q,"serviceStatus":"stopped","networkReachable":true,"cpuUsage":10.0,"memoryUsage":40.0,"recentLogs":["still down"]}`,
		deviceID, serviceName,
	)
	postTelemetry(t, handler, body)

	inc, _ := incidentStore.GetByID(id)
	if inc.State != incidents.StateValidating {
		t.Fatalf("state = %q, want validating", inc.State)
	}
	if inc.HealthyCycleCount != 0 {
		t.Fatalf("healthy count = %d, want 0", inc.HealthyCycleCount)
	}
}

func TestTelemetryPipeline_IgnoresStaleTelemetryForValidation(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	const deviceID, serviceName = "LAPTOP-22", "OpenVPNService"
	// Boundary in the FUTURE: live telemetry (stamped ~now) is older than the boundary
	// and must be ignored as pre-remediation evidence.
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(time.Hour))

	postTelemetry(t, handler, healthyTelemetryBody(deviceID, serviceName))

	inc, _ := incidentStore.GetByID(id)
	if inc.HealthyCycleCount != 0 {
		t.Fatalf("healthy count = %d, want 0 (stale telemetry must not count)", inc.HealthyCycleCount)
	}
	if inc.LastValidationTelemetryAt != nil {
		t.Fatal("expected no validation timestamp recorded for stale telemetry")
	}
}

func TestStartValidationTimeoutSweeper_FailsTimedOutIncident(t *testing.T) {
	incidentStore := incidents.NewStore()
	const deviceID, serviceName = "LAPTOP-22", "OpenVPNService"
	// Boundary far enough in the past that the default timeout has already elapsed.
	id := driveToValidating(t, incidentStore, deviceID, serviceName, time.Now().UTC().Add(-2*incidents.DefaultValidationTimeout))

	// A short sweep interval so the test does not wait long.
	startValidationTimeoutSweeper(incidentStore, nil, nil, 20*time.Millisecond, incidents.DefaultValidationTimeout)

	deadline := time.Now().Add(2 * time.Second)
	for {
		inc, _ := incidentStore.GetByID(id)
		if inc.State == incidents.StateFailed {
			if inc.ValidationFailureReason == "" {
				t.Fatal("expected a recorded validation failure reason")
			}
			if inc.Active {
				t.Fatal("expected failed incident to be inactive")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("incident was not failed by the sweeper in time, state=%q", inc.State)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
