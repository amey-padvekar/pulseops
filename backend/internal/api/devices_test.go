package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/store"
)

func seedStore() *store.DeviceStore {
	s := store.NewDeviceStore()
	s.Upsert(store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
		Heartbeat:     true,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	})
	return s
}

// --- GET /devices ---

func TestDevicesHandler_ReturnsAll(t *testing.T) {
	s := seedStore()
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	rr := httptest.NewRecorder()

	api.DevicesHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []store.DeviceState
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 device, got %d", len(result))
	}
	if result[0].DeviceID != "LAPTOP-22" {
		t.Errorf("DeviceID: got %q, want %q", result[0].DeviceID, "LAPTOP-22")
	}
}

func TestDevicesHandler_EmptyStore(t *testing.T) {
	s := store.NewDeviceStore()
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	rr := httptest.NewRecorder()

	api.DevicesHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result []store.DeviceState
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d entries", len(result))
	}
}

func TestDevicesHandler_MethodNotAllowed(t *testing.T) {
	s := store.NewDeviceStore()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/devices", nil)
		rr := httptest.NewRecorder()
		api.DevicesHandler(s)(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rr.Code)
		}
	}
}

// --- GET /devices/{deviceId} ---

func TestDeviceByIDHandler_Found(t *testing.T) {
	s := seedStore()
	req := httptest.NewRequest(http.MethodGet, "/devices/LAPTOP-22", nil)
	rr := httptest.NewRecorder()

	api.DeviceByIDHandler(s)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var result store.DeviceState
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if result.DeviceID != "LAPTOP-22" {
		t.Errorf("DeviceID: got %q, want %q", result.DeviceID, "LAPTOP-22")
	}
	if result.ServiceStatus != "running" {
		t.Errorf("ServiceStatus: got %q, want %q", result.ServiceStatus, "running")
	}
}

func TestDeviceByIDHandler_NotFound(t *testing.T) {
	s := store.NewDeviceStore()
	req := httptest.NewRequest(http.MethodGet, "/devices/GHOST-99", nil)
	rr := httptest.NewRecorder()

	api.DeviceByIDHandler(s)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body["error"] != "device not found" {
		t.Errorf("error body: got %q, want %q", body["error"], "device not found")
	}
}

func TestDeviceByIDHandler_EmptyIDReturns404(t *testing.T) {
	s := store.NewDeviceStore()
	// bare /devices/ with no ID after the slash
	req := httptest.NewRequest(http.MethodGet, "/devices/", nil)
	rr := httptest.NewRecorder()

	api.DeviceByIDHandler(s)(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for empty device ID, got %d", rr.Code)
	}
}

func TestDeviceByIDHandler_MethodNotAllowed(t *testing.T) {
	s := store.NewDeviceStore()
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/devices/LAPTOP-22", nil)
		rr := httptest.NewRecorder()
		api.DeviceByIDHandler(s)(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected 405, got %d", method, rr.Code)
		}
	}
}
