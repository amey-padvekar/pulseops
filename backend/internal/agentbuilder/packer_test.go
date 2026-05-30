package agentbuilder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
)

func TestBuildRequest_PopulatesFieldsAndBoundsLogs(t *testing.T) {
	incidentStore := incidents.NewStore()
	deviceStore := store.NewDeviceStore()

	deviceStore.Upsert(store.DeviceState{
		DeviceID:         "DEV-01",
		ServiceName:      "OpenVPNService",
		ServiceStatus:    "stopped",
		NetworkReachable: true,
		CPUUsage:         42.5,
		MemoryUsage:      78.1,
		RecentLogs:       []string{"log-1", "log-2", "log-3", "log-4"},
		Heartbeat:        true,
	})

	seed := incidents.NewIncident(
		"",
		"DEV-01",
		"OpenVPNService",
		"stopped",
		incidents.SeverityHigh,
		"service stopped",
	)

	incident, _ := incidentStore.CreateOrGetActive("dev-01|vpn|service_stopped", seed)

	requestedAt := time.Date(2026, 5, 30, 10, 0, 0, 0, time.UTC)
	req, err := BuildRequest(incident.IncidentID, incidentStore, deviceStore, BuildRequestOptions{
		RequestedAt: requestedAt,
		MaxLogs:     2,
	})
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	if req.RequestID == "" {
		t.Fatal("expected requestId to be populated")
	}
	if req.IncidentID != incident.IncidentID {
		t.Fatalf("incidentId = %q, want %q", req.IncidentID, incident.IncidentID)
	}
	if req.DeviceID != "DEV-01" {
		t.Fatalf("deviceId = %q, want %q", req.DeviceID, "DEV-01")
	}
	if req.ServiceName != "OpenVPNService" {
		t.Fatalf("serviceName = %q, want %q", req.ServiceName, "OpenVPNService")
	}
	if req.TimeWindow.Start.IsZero() || req.TimeWindow.End.IsZero() {
		t.Fatal("expected time window to be populated")
	}
	if req.TimeWindow.End.Before(req.TimeWindow.Start) {
		t.Fatal("expected time window end to be >= start")
	}
	if len(req.RecentLogs) != 2 {
		t.Fatalf("recentLogs length = %d, want 2", len(req.RecentLogs))
	}
	if len(req.AvailableActions) == 0 {
		t.Fatal("expected availableActions to be populated")
	}
	if !strings.Contains(req.IncidentSummary, incident.IncidentID) {
		t.Fatalf("incidentSummary missing incidentId: %q", req.IncidentSummary)
	}
	if req.TelemetrySnapshot.ServiceStatus != "stopped" {
		t.Fatalf("telemetrySnapshot.serviceStatus = %q, want %q", req.TelemetrySnapshot.ServiceStatus, "stopped")
	}
	if req.RequestedAt.IsZero() || !req.RequestedAt.Equal(requestedAt) {
		t.Fatalf("requestedAt = %s, want %s", req.RequestedAt, requestedAt)
	}
}

func TestBuildRequest_SamplePayload(t *testing.T) {
	incidentStore := incidents.NewStore()
	deviceStore := store.NewDeviceStore()

	deviceStore.Upsert(store.DeviceState{
		DeviceID:         "DEV-LOCAL",
		ServiceName:      "OpenVPNService",
		ServiceStatus:    "stopped",
		NetworkReachable: true,
		CPUUsage:         12.3,
		MemoryUsage:      45.6,
		RecentLogs:       []string{"log-a", "log-b"},
		Heartbeat:        true,
	})

	seed := incidents.NewIncident(
		"",
		"DEV-LOCAL",
		"OpenVPNService",
		"stopped",
		incidents.SeverityMedium,
		"service stopped",
	)

	incident, _ := incidentStore.CreateOrGetActive("dev-local|vpn|service_stopped", seed)

	req, err := BuildRequest(incident.IncidentID, incidentStore, deviceStore, BuildRequestOptions{})
	if err != nil {
		t.Fatalf("BuildRequest error: %v", err)
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request error: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty request payload")
	}

	t.Logf("sample_request=%s", string(data))
}
