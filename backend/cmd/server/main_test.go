package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/agentbuilder"
	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/elastic"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

func newTestTelemetryHandler(deviceStore *store.DeviceStore, incidentStore *incidents.Store) http.HandlerFunc {
	var elasticCfg *elastic.Config
	var agentClient agentbuilder.Client
	var agentCfg *agentbuilder.Config
	return makeTelemetryHandler(deviceStore, incidentStore, ws.NewHub(), nil, elasticCfg, agentClient, agentCfg)
}

func TestTelemetryHandlerRejectsWrongMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/telemetry", nil)
	resp := httptest.NewRecorder()

	newTestTelemetryHandler(store.NewDeviceStore(), incidents.NewStore())(resp, req)

	if resp.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusMethodNotAllowed)
	}
}

func TestTelemetryHandlerRejectsInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString("not-json"))
	resp := httptest.NewRecorder()

	newTestTelemetryHandler(store.NewDeviceStore(), incidents.NewStore())(resp, req)

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

	newTestTelemetryHandler(store.NewDeviceStore(), incidents.NewStore())(resp, req)

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

	newTestTelemetryHandler(s, incidents.NewStore())(resp, req)

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

func TestTelemetryHandler_CreatesIncidentAndExposesViaAPI(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()

	handler := newTestTelemetryHandler(deviceStore, incidentStore)
	body := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":14.5,"memoryUsage":62.3,"recentLogs":["service stopped"]}`
	req := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	resp := httptest.NewRecorder()

	handler(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusAccepted)
	}

	active := true
	incidentsList := incidentStore.List(incidents.IncidentFilter{Active: &active})
	if len(incidentsList) != 1 {
		t.Fatalf("expected 1 active incident, got %d", len(incidentsList))
	}
	if incidentsList[0].State != incidents.StateInvestigating {
		t.Fatalf("state = %q, want %q", incidentsList[0].State, incidents.StateInvestigating)
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/incidents", nil)
	apiResp := httptest.NewRecorder()
	api.IncidentsHandler(incidentStore)(apiResp, apiReq)
	if apiResp.Code != http.StatusOK {
		t.Fatalf("incidents api status = %d, want %d", apiResp.Code, http.StatusOK)
	}

	var payload []incidents.Incident
	if err := json.Unmarshal(apiResp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode incidents payload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected incidents api to return 1 incident, got %d", len(payload))
	}
}

func TestTelemetryHandler_ReusesActiveIncidentForRepeatedFailure(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	body := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":14.5,"memoryUsage":62.3,"recentLogs":["service stopped"]}`
	firstReq := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	firstResp := httptest.NewRecorder()
	handler(firstResp, firstReq)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d", firstResp.Code, http.StatusAccepted)
	}

	active := true
	firstList := incidentStore.List(incidents.IncidentFilter{Active: &active})
	if len(firstList) != 1 {
		t.Fatalf("expected 1 incident after first telemetry, got %d", len(firstList))
	}
	firstID := firstList[0].IncidentID

	secondReq := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	secondResp := httptest.NewRecorder()
	handler(secondResp, secondReq)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want %d", secondResp.Code, http.StatusAccepted)
	}

	secondList := incidentStore.List(incidents.IncidentFilter{Active: &active})
	if len(secondList) != 1 {
		t.Fatalf("expected 1 incident after repeated telemetry, got %d", len(secondList))
	}
	if secondList[0].IncidentID != firstID {
		t.Fatalf("expected same incident ID on repeated failure, got %q and %q", firstID, secondList[0].IncidentID)
	}
}

func TestTelemetryToIncidentEndpoints_EndToEnd(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	hub := ws.NewHub()

	mux := http.NewServeMux()
	mux.HandleFunc("/telemetry", makeTelemetryHandler(deviceStore, incidentStore, hub, nil, nil, nil, nil))
	mux.HandleFunc("/incidents", api.IncidentsHandler(incidentStore))
	mux.HandleFunc("/incidents/", api.IncidentByIDHandler(incidentStore))

	body := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":14.5,"memoryUsage":62.3,"recentLogs":["service stopped"]}`
	telemetryReq := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(body))
	telemetryResp := httptest.NewRecorder()
	mux.ServeHTTP(telemetryResp, telemetryReq)

	if telemetryResp.Code != http.StatusAccepted {
		t.Fatalf("telemetry status = %d, want %d", telemetryResp.Code, http.StatusAccepted)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/incidents?active=true", nil)
	listResp := httptest.NewRecorder()
	mux.ServeHTTP(listResp, listReq)

	if listResp.Code != http.StatusOK {
		t.Fatalf("list status = %d, want %d", listResp.Code, http.StatusOK)
	}

	var listed []incidents.Incident
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed incident, got %d", len(listed))
	}
	if listed[0].State != incidents.StateInvestigating {
		t.Fatalf("state = %q, want %q", listed[0].State, incidents.StateInvestigating)
	}
	if listed[0].Severity != incidents.SeverityHigh {
		t.Fatalf("severity = %q, want %q", listed[0].Severity, incidents.SeverityHigh)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/incidents/"+listed[0].IncidentID, nil)
	detailResp := httptest.NewRecorder()
	mux.ServeHTTP(detailResp, detailReq)

	if detailResp.Code != http.StatusOK {
		t.Fatalf("detail status = %d, want %d", detailResp.Code, http.StatusOK)
	}

	var detail incidents.Incident
	if err := json.Unmarshal(detailResp.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.IncidentID != listed[0].IncidentID {
		t.Fatalf("detail incidentID = %q, want %q", detail.IncidentID, listed[0].IncidentID)
	}
	if !detail.Active {
		t.Fatal("expected created incident to remain active")
	}
	if detail.Reason != "service stopped while heartbeat is present" {
		t.Fatalf("reason = %q", detail.Reason)
	}
}

func TestTelemetryHandler_RepeatedFailureRefreshesIncidentTimestamps(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	handler := newTestTelemetryHandler(deviceStore, incidentStore)

	firstBody := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:00:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":14.5,"memoryUsage":62.3,"recentLogs":["service stopped"]}`
	firstReq := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(firstBody))
	firstResp := httptest.NewRecorder()
	handler(firstResp, firstReq)
	if firstResp.Code != http.StatusAccepted {
		t.Fatalf("first status = %d, want %d", firstResp.Code, http.StatusAccepted)
	}

	active := true
	firstList := incidentStore.List(incidents.IncidentFilter{Active: &active})
	if len(firstList) != 1 {
		t.Fatalf("expected 1 incident after first telemetry, got %d", len(firstList))
	}
	first := firstList[0]
	time.Sleep(5 * time.Millisecond)

	secondBody := `{"schemaVersion":"1.0.0","deviceId":"LAPTOP-22","timestamp":"2026-05-23T10:01:00Z","heartbeat":true,"serviceName":"OpenVPNService","serviceStatus":"stopped","networkReachable":true,"cpuUsage":18.0,"memoryUsage":63.1,"recentLogs":["service still stopped"]}`
	secondReq := httptest.NewRequest(http.MethodPost, "/telemetry", bytes.NewBufferString(secondBody))
	secondResp := httptest.NewRecorder()
	handler(secondResp, secondReq)
	if secondResp.Code != http.StatusAccepted {
		t.Fatalf("second status = %d, want %d", secondResp.Code, http.StatusAccepted)
	}

	secondList := incidentStore.List(incidents.IncidentFilter{Active: &active})
	if len(secondList) != 1 {
		t.Fatalf("expected 1 incident after repeated telemetry, got %d", len(secondList))
	}
	second := secondList[0]
	if second.IncidentID != first.IncidentID {
		t.Fatalf("expected same incident ID, got %q and %q", first.IncidentID, second.IncidentID)
	}
	if !second.LastSeenAt.After(first.LastSeenAt) {
		t.Fatalf("expected LastSeenAt to advance: first=%v second=%v", first.LastSeenAt, second.LastSeenAt)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("expected UpdatedAt to advance: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}
}
