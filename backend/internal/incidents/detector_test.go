package incidents

import (
	"testing"

	"github.com/certainelf/pulseops/backend/internal/store"
)

func TestEvaluateTelemetry_StoppedAndHeartbeatTrue_ReturnsDetection(t *testing.T) {
	result := EvaluateTelemetry(store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "stopped",
		Heartbeat:     true,
	})

	if !result.ShouldCreateOrUpdate {
		t.Fatal("expected ShouldCreateOrUpdate=true")
	}
	if result.DedupeKey != "LAPTOP-22|OpenVPNService|service_stopped" {
		t.Fatalf("dedupe key: got %q", result.DedupeKey)
	}
	if result.Reason != "service stopped while heartbeat is present" {
		t.Fatalf("reason: got %q", result.Reason)
	}
	if result.Severity != SeverityHigh {
		t.Fatalf("severity: got %q, want %q", result.Severity, SeverityHigh)
	}
}

func TestEvaluateTelemetry_RunningService_NoDetection(t *testing.T) {
	result := EvaluateTelemetry(store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
		Heartbeat:     true,
	})

	if result.ShouldCreateOrUpdate {
		t.Fatal("expected ShouldCreateOrUpdate=false")
	}
	if result.DedupeKey != "" || result.Reason != "" || result.Severity != "" {
		t.Fatalf("expected empty result fields, got %+v", result)
	}
}

func TestEvaluateTelemetry_StoppedHeartbeatFalse_NoDetection(t *testing.T) {
	result := EvaluateTelemetry(store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "stopped",
		Heartbeat:     false,
	})

	if result.ShouldCreateOrUpdate {
		t.Fatal("expected ShouldCreateOrUpdate=false")
	}
}
