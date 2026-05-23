package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

func TestTelemetryHandlerRejectsWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/telemetry", nil)
	resp := httptest.NewRecorder()

	makeTelemetryHandler(store.NewDeviceStore(), ws.NewHub())(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusMethodNotAllowed)
	}
}

func TestTelemetryHandlerRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString("not-json"))
	resp := httptest.NewRecorder()

	makeTelemetryHandler(store.NewDeviceStore(), ws.NewHub())(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestTelemetryHandlerAcceptsValidPayload(t *testing.T) {
	body := `{"schemaVersion":"1.0.0","deviceId":"DEV-AGENT-01","timestamp":"2026-05-21T12:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"unknown","networkReachable":true,"cpuUsage":0,"memoryUsage":0,"recentLogs":["heartbeat cycle=1"]}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	req.Header.Set("X-PulseOps-Request-ID", "req-123")
	req.Header.Set("X-PulseOps-Request-Attempt", "2")
	req.Header.Set("X-PulseOps-Device-ID", "DEV-AGENT-01")
	resp := httptest.NewRecorder()

	makeTelemetryHandler(store.NewDeviceStore(), ws.NewHub())(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusAccepted)
	}

	var response map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if response["requestId"] != "req-123" {
		t.Fatalf("requestId = %q, want %q", response["requestId"], "req-123")
	}
}

func TestTelemetryHandlerUpdatesStore(t *testing.T) {
	s := store.NewDeviceStore()
	body := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":14.5,"memoryUsage":62.3,"recentLogs":["service stopped"]}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	resp := httptest.NewRecorder()

	makeTelemetryHandler(s, ws.NewHub())(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusAccepted)
	}

	state, ok := s.Get("LAPTOP-22")
	if !ok {
		t.Fatal("expected store to contain LAPTOP-22 after telemetry POST")
	}
	if state.ServiceStatus != "stopped" {
		t.Errorf("ServiceStatus: got %q, want %q", state.ServiceStatus, "stopped")
	}
	if state.CPUUsage != 14.5 {
		t.Errorf("CPUUsage: got %v, want 14.5", state.CPUUsage)
	}
	if state.ServiceName != "OpenVPNService" {
		t.Errorf("ServiceName: got %q, want %q", state.ServiceName, "OpenVPNService")
	}
	if state.LastSeenAt.IsZero() {
		t.Error("LastSeenAt should be set after upsert")
	}
}
