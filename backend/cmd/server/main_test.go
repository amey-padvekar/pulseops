package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTelemetryHandlerRejectsWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/telemetry", nil)
	resp := httptest.NewRecorder()

	telemetryHandler(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusMethodNotAllowed)
	}
}

func TestTelemetryHandlerRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString("not-json"))
	resp := httptest.NewRecorder()

	telemetryHandler(resp, req)

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

	telemetryHandler(resp, req)

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
