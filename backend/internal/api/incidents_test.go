package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func seedIncidentStore(t *testing.T) *incidents.Store {
	t.Helper()

	s := incidents.NewStore()

	oldIncident, _ := s.CreateOrGetActive(
		"dev-1|OpenVPNService|service_stopped",
		incidents.NewIncidentAt(
			"inc-1",
			"dev-1",
			"OpenVPNService",
			"stopped",
			incidents.SeverityHigh,
			"service stopped while heartbeat is present",
			time.Now().UTC().Add(-2*time.Minute),
		),
	)

	_, err := s.UpdateState(oldIncident.IncidentID, incidents.StateResolved, "fixed")
	if err != nil {
		t.Fatalf("resolve seed incident: %v", err)
	}

	_, _ = s.CreateOrGetActive(
		"dev-2|OpenVPNService|service_stopped",
		incidents.NewIncidentAt(
			"inc-2",
			"dev-2",
			"OpenVPNService",
			"stopped",
			incidents.SeverityHigh,
			"service stopped while heartbeat is present",
			time.Now().UTC(),
		),
	)

	return s
}

func TestIncidentsHandler_ListAll(t *testing.T) {
	s := seedIncidentStore(t)
	req := httptest.NewRequest(http.MethodGet, "/incidents", nil)
	rr := httptest.NewRecorder()

	api.IncidentsHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []incidents.Incident
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(result))
	}
}

func TestIncidentsHandler_FilterActive(t *testing.T) {
	s := seedIncidentStore(t)
	req := httptest.NewRequest(http.MethodGet, "/incidents?active=true", nil)
	rr := httptest.NewRecorder()

	api.IncidentsHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []incidents.Incident
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 active incident, got %d", len(result))
	}
	if !result[0].Active {
		t.Fatal("expected active incident")
	}
}

func TestIncidentsHandler_FilterDeviceAndState(t *testing.T) {
	s := seedIncidentStore(t)
	req := httptest.NewRequest(http.MethodGet, "/incidents?deviceId=dev-1&state=resolved", nil)
	rr := httptest.NewRecorder()

	api.IncidentsHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []incidents.Incident
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 filtered incident, got %d", len(result))
	}
	if result[0].IncidentID != "inc-1" {
		t.Fatalf("incidentId: got %q, want %q", result[0].IncidentID, "inc-1")
	}
}

func TestIncidentsHandler_InvalidActiveFilter(t *testing.T) {
	s := incidents.NewStore()
	req := httptest.NewRequest(http.MethodGet, "/incidents?active=not-bool", nil)
	rr := httptest.NewRecorder()

	api.IncidentsHandler(s)(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body["error"] != "invalid active filter" {
		t.Fatalf("error body: got %q", body["error"])
	}
}

func TestIncidentsHandler_MethodNotAllowed(t *testing.T) {
	s := incidents.NewStore()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/incidents", nil)
		rr := httptest.NewRecorder()
		api.IncidentsHandler(s)(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rr.Code)
		}
	}
}

func TestIncidentByIDHandler_Found(t *testing.T) {
	s := seedIncidentStore(t)
	req := httptest.NewRequest(http.MethodGet, "/incidents/inc-2", nil)
	rr := httptest.NewRecorder()

	api.IncidentByIDHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result incidents.Incident
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.IncidentID != "inc-2" {
		t.Fatalf("incidentId: got %q, want %q", result.IncidentID, "inc-2")
	}
}

func TestIncidentByIDHandler_NotFound(t *testing.T) {
	s := incidents.NewStore()
	req := httptest.NewRequest(http.MethodGet, "/incidents/missing", nil)
	rr := httptest.NewRecorder()

	api.IncidentByIDHandler(s)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body["error"] != "incident not found" {
		t.Fatalf("error body: got %q", body["error"])
	}
}

func TestIncidentByIDHandler_EmptyIDReturns404(t *testing.T) {
	s := incidents.NewStore()
	req := httptest.NewRequest(http.MethodGet, "/incidents/", nil)
	rr := httptest.NewRecorder()

	api.IncidentByIDHandler(s)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty incident ID, got %d", rr.Code)
	}
}

func TestIncidentByIDHandler_MethodNotAllowed(t *testing.T) {
	s := incidents.NewStore()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/incidents/inc-1", nil)
		rr := httptest.NewRecorder()
		api.IncidentByIDHandler(s)(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rr.Code)
		}
	}
}
