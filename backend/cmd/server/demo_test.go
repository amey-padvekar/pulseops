package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

// newTestDemoController builds a controller with no Elastic/agent wiring and near-instant
// pacing so lifecycle tests run fast. A nil agentClient makes the async investigation handoff a
// safe no-op, so a generated incident settles in `investigating` unless the test promotes it.
func newTestDemoController(deviceStore *store.DeviceStore, incidentStore *incidents.Store) *demoController {
	c := newDemoController(deviceStore, incidentStore, ws.NewHub(), nil, nil, nil, nil)
	c.pacing = demoPacing{
		stagePause:      time.Millisecond,
		pollInterval:    time.Millisecond,
		investigateWait: 50 * time.Millisecond,
		approvalWait:    50 * time.Millisecond,
	}
	return c
}

func newTestDemoIncidentHandler(deviceStore *store.DeviceStore, incidentStore *incidents.Store) http.HandlerFunc {
	return newTestDemoController(deviceStore, incidentStore).incidentHandler()
}

func postDemoIncident(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/demo/incident", bytes.NewBufferString(body))
	resp := httptest.NewRecorder()
	h(resp, req)
	return resp
}

func TestDemoIncidentHandlerCreatesIncident(t *testing.T) {
	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()

	resp := postDemoIncident(t, newTestDemoIncidentHandler(deviceStore, incidentStore),
		`{"deviceId":"DEV-DEMO-1","scenario":"spooler"}`)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	var out map[string]string
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["incidentId"] == "" {
		t.Fatal("response incidentId is empty")
	}
	if out["serviceName"] != "Spooler" {
		t.Errorf("serviceName = %q, want Spooler", out["serviceName"])
	}
	if out["scenario"] != "spooler" {
		t.Errorf("scenario = %q, want spooler", out["scenario"])
	}
	// A newly created incident is promoted to investigating (processTelemetryIncident).
	if out["state"] != string(incidents.StateInvestigating) {
		t.Errorf("state = %q, want %q", out["state"], incidents.StateInvestigating)
	}

	// The synthetic stopped sample must have been upserted into the device store.
	dev, ok := deviceStore.Get("DEV-DEMO-1")
	if !ok {
		t.Fatal("device DEV-DEMO-1 not upserted")
	}
	if dev.ServiceStatus != "stopped" || !dev.Heartbeat {
		t.Errorf("synthetic state = status:%q heartbeat:%t, want stopped/true", dev.ServiceStatus, dev.Heartbeat)
	}
	if len(dev.RecentLogs) == 0 {
		t.Error("synthetic RecentLogs empty; scenario flavor not applied")
	}

	// Exactly one active incident opened, matching the returned ID.
	active := true
	got := incidentStore.List(incidents.IncidentFilter{DeviceID: "DEV-DEMO-1", Active: &active})
	if len(got) != 1 {
		t.Fatalf("active incidents for device = %d, want 1", len(got))
	}
	if got[0].IncidentID != out["incidentId"] {
		t.Errorf("response incidentId %q != store incident %q", out["incidentId"], got[0].IncidentID)
	}
}

func TestDemoIncidentHandlerDefaultsScenario(t *testing.T) {
	resp := postDemoIncident(t, newTestDemoIncidentHandler(store.NewDeviceStore(), incidents.NewStore()),
		`{"deviceId":"DEV-X"}`)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	// Blank scenario falls back to demo.Default() = Endpoint Security / WinDefend.
	if out["scenario"] != "defender" || out["serviceName"] != "WinDefend" {
		t.Fatalf("default scenario = %q/%q, want defender/WinDefend", out["scenario"], out["serviceName"])
	}
}

func TestDemoIncidentHandlerServiceNameOverride(t *testing.T) {
	resp := postDemoIncident(t, newTestDemoIncidentHandler(store.NewDeviceStore(), incidents.NewStore()),
		`{"deviceId":"DEV-Y","scenario":"mysql","serviceName":"MySQL"}`)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}
	var out map[string]string
	_ = json.Unmarshal(resp.Body.Bytes(), &out)
	// Explicit serviceName wins over the catalog default (MySQL80) while keeping the scenario.
	if out["serviceName"] != "MySQL" || out["scenario"] != "mysql" {
		t.Fatalf("override = %q/%q, want MySQL/mysql", out["serviceName"], out["scenario"])
	}
}

func TestDemoIncidentHandlerRequiresDeviceID(t *testing.T) {
	resp := postDemoIncident(t, newTestDemoIncidentHandler(store.NewDeviceStore(), incidents.NewStore()),
		`{"scenario":"spooler"}`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestDemoIncidentHandlerRejectsInvalidJSON(t *testing.T) {
	resp := postDemoIncident(t, newTestDemoIncidentHandler(store.NewDeviceStore(), incidents.NewStore()),
		`not-json`)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

// seedAwaitingApproval opens a real incident (stopped + heartbeat detection) and drives it to
// awaiting_approval by simulating the agent's investigation result + promotion — the exact
// store path the real agent uses — so runLifecycle starts from the approval gate.
func seedAwaitingApproval(t *testing.T, incidentStore *incidents.Store, deviceID, serviceName string) incidents.Incident {
	t.Helper()

	if _, ok := ingestTelemetrySample(
		store.DeviceState{
			DeviceID: deviceID, ServiceName: serviceName, ServiceStatus: "stopped",
			NetworkReachable: true, Heartbeat: true, RecentLogs: []string{"service stopped"},
		},
		store.NewDeviceStore(), incidentStore, ws.NewHub(), nil, nil, nil, nil,
	); !ok {
		t.Fatal("seed: ingestTelemetrySample did not ingest")
	}

	inc, found := activeIncidentForService(incidentStore, deviceID, serviceName)
	if !found {
		t.Fatal("seed: no incident opened for device")
	}

	recs := []incidents.RecommendedAction{{ActionID: "restart_service", Target: serviceName, Description: "restart the stopped service"}}
	if _, err := incidentStore.SaveInvestigationResult(
		inc.IncidentID, "service stopped unexpectedly", 0.9, recs,
		[]string{"service is running"}, "diagnosis summary", time.Now().UTC(), "trace-demo", "completed",
	); err != nil {
		t.Fatalf("seed: SaveInvestigationResult: %v", err)
	}

	promoted, changed, err := incidentStore.PromoteToAwaitingApproval(inc.IncidentID)
	if err != nil || !changed {
		t.Fatalf("seed: PromoteToAwaitingApproval changed=%t err=%v", changed, err)
	}
	if promoted.State != incidents.StateAwaitingApproval {
		t.Fatalf("seed: state = %q, want awaiting_approval", promoted.State)
	}
	return promoted
}

func TestRunLifecycleAutoApproveResolves(t *testing.T) {
	incidentStore := incidents.NewStore()
	c := newTestDemoController(store.NewDeviceStore(), incidentStore)

	inc := seedAwaitingApproval(t, incidentStore, "DEV-LC-1", "Spooler")

	c.runLifecycle(inc.IncidentID, true) // synchronous, hands-off auto-approve

	final, ok := incidentStore.GetByID(inc.IncidentID)
	if !ok {
		t.Fatal("incident missing after lifecycle")
	}
	if final.State != incidents.StateResolved {
		t.Fatalf("final state = %q, want resolved", final.State)
	}
	if final.ValidationStatus != incidents.ValidationStatusSucceeded {
		t.Errorf("validation status = %q, want succeeded", final.ValidationStatus)
	}
	if final.RemediationStatus != "succeeded" || len(final.RemediationResults) != 1 {
		t.Errorf("remediation: status=%q results=%d, want succeeded/1", final.RemediationStatus, len(final.RemediationResults))
	}
	if final.RemediationResults[0].ActionID != "restart_service" || final.RemediationResults[0].Target != "Spooler" {
		t.Errorf("remediation action = %+v, want restart_service/Spooler", final.RemediationResults[0])
	}
	if final.ApprovedBy != "demo-auto-approve" {
		t.Errorf("approvedBy = %q, want demo-auto-approve", final.ApprovedBy)
	}
}

func TestRunLifecycleManualGateWaitsWithoutApproval(t *testing.T) {
	incidentStore := incidents.NewStore()
	c := newTestDemoController(store.NewDeviceStore(), incidentStore)

	inc := seedAwaitingApproval(t, incidentStore, "DEV-LC-2", "Spooler")

	// autoApprove=false and no one ever approves → the lifecycle must NOT force execution; it
	// times out and leaves the incident at the approval gate (governance is respected).
	c.runLifecycle(inc.IncidentID, false)

	final, _ := incidentStore.GetByID(inc.IncidentID)
	if final.State != incidents.StateAwaitingApproval {
		t.Fatalf("final state = %q, want awaiting_approval (governance gate respected)", final.State)
	}
}

func TestDemoResetDeletesDeviceIncidents(t *testing.T) {
	incidentStore := incidents.NewStore()
	c := newTestDemoController(store.NewDeviceStore(), incidentStore)

	seedAwaitingApproval(t, incidentStore, "DEV-RESET-1", "Spooler")
	seedAwaitingApproval(t, incidentStore, "DEV-OTHER", "WinDefend")

	resp := httptest.NewRecorder()
	c.resetHandler()(resp, httptest.NewRequest(http.MethodPost, "/demo/reset", bytes.NewBufferString(`{"deviceId":"DEV-RESET-1"}`)))

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	if got := len(incidentStore.List(incidents.IncidentFilter{DeviceID: "DEV-RESET-1"})); got != 0 {
		t.Fatalf("incidents for DEV-RESET-1 after reset = %d, want 0", got)
	}
	// A different device's incident must be untouched.
	if got := len(incidentStore.List(incidents.IncidentFilter{DeviceID: "DEV-OTHER"})); got != 1 {
		t.Fatalf("incidents for DEV-OTHER after reset = %d, want 1 (unaffected)", got)
	}
	// Dedupe cleared → the same device can open a fresh incident again.
	if again := seedAwaitingApproval(t, incidentStore, "DEV-RESET-1", "Spooler"); again.IncidentID == "" {
		t.Fatal("expected a fresh incident after reset")
	}
}

func TestDemoResetRequiresDeviceID(t *testing.T) {
	c := newTestDemoController(store.NewDeviceStore(), incidents.NewStore())
	resp := httptest.NewRecorder()
	c.resetHandler()(resp, httptest.NewRequest(http.MethodPost, "/demo/reset", bytes.NewBufferString(`{}`)))
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.Code)
	}
}

func TestDemoRoutesAbsentWhenDisabled(t *testing.T) {
	mux := http.NewServeMux()
	if registerDemoRoutes(mux, false, store.NewDeviceStore(), incidents.NewStore(), ws.NewHub(), nil, nil, nil, nil) {
		t.Fatal("registerDemoRoutes returned true when disabled")
	}
	for _, path := range []string{"/demo/incident", "/demo/reset"} {
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusNotFound {
			t.Errorf("GET %s (demo disabled) = %d, want 404", path, resp.Code)
		}
	}
}

func TestDemoRoutesPresentWhenEnabled(t *testing.T) {
	mux := http.NewServeMux()
	if !registerDemoRoutes(mux, true, store.NewDeviceStore(), incidents.NewStore(), ws.NewHub(), nil, nil, nil, nil) {
		t.Fatal("registerDemoRoutes returned false when enabled")
	}
	// A wrong method on a registered POST route yields 405 (present), not 404 (absent); this
	// asserts presence without invoking the POST handler (and its lifecycle goroutine).
	for _, path := range []string{"/demo/incident", "/demo/reset"} {
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s (demo enabled) = %d, want 405 (route present)", path, resp.Code)
		}
	}
}

// TestDemoCoexistenceRunningTelemetryDoesNotDisturbPreValidating locks the B6 guarantee: a
// competing real agent's healthy "running" telemetry must not advance, resolve, or otherwise
// disturb a demo incident while it is pre-validating (detection acts only on `stopped`,
// validation only on `validating`).
func TestDemoCoexistenceRunningTelemetryDoesNotDisturbPreValidating(t *testing.T) {
	incidentStore := incidents.NewStore()
	c := newTestDemoController(store.NewDeviceStore(), incidentStore)

	inc := seedAwaitingApproval(t, incidentStore, "DEV-COEX-1", "Spooler")

	for i := 0; i < 3; i++ {
		c.ingestHealthySample("DEV-COEX-1", "Spooler")
	}

	after, ok := incidentStore.GetByID(inc.IncidentID)
	if !ok {
		t.Fatal("incident missing after running telemetry")
	}
	if after.State != incidents.StateAwaitingApproval {
		t.Fatalf("state = %q, want awaiting_approval (undisturbed by running telemetry)", after.State)
	}
	if after.HealthyCycleCount != 0 {
		t.Errorf("healthyCycleCount = %d, want 0 (validation must not run pre-validating)", after.HealthyCycleCount)
	}
	if got := len(incidentStore.List(incidents.IncidentFilter{DeviceID: "DEV-COEX-1"})); got != 1 {
		t.Fatalf("incident count = %d, want 1 (running telemetry opened no new incident)", got)
	}
}
