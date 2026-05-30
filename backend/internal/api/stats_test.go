package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/store"
)

type statsBody struct {
	ActiveAgents       int      `json:"activeAgents"`
	ActiveAgentDevices []string `json:"activeAgentDevices"`
}

func TestStatsHandler_ReturnsActiveAgentsCount(t *testing.T) {
	s := store.NewDeviceStore()
	s.Upsert(store.DeviceState{DeviceID: "DEV-AGENT-01"})
	s.Upsert(store.DeviceState{DeviceID: "DEV-AGENT-02"})

	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rr := httptest.NewRecorder()

	api.StatsHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body statsBody
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if got := body.ActiveAgents; got != 2 {
		t.Fatalf("activeAgents = %d, want 2", got)
	}

	wantDevices := []string{"DEV-AGENT-01", "DEV-AGENT-02"}
	if !reflect.DeepEqual(body.ActiveAgentDevices, wantDevices) {
		t.Fatalf("activeAgentDevices = %v, want %v", body.ActiveAgentDevices, wantDevices)
	}
}

func TestStatsHandler_MethodNotAllowed(t *testing.T) {
	s := store.NewDeviceStore()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/stats", nil)
		rr := httptest.NewRecorder()

		api.StatsHandler(s)(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("method %s: expected 405, got %d", method, rr.Code)
		}
	}
}
