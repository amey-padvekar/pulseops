package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/certainelf/pulseops/backend/internal/store"
)

// DevicesHandler returns an http.HandlerFunc for GET /devices.
// It responds with a JSON array of all known device states (may be empty).
func DevicesHandler(s *store.DeviceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		states := s.List()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(states)
	}
}

// DeviceByIDHandler returns an http.HandlerFunc for GET /devices/{deviceId}.
// It extracts the device ID from the URL path, looks it up in the store, and
// returns 200 with the state JSON or 404 with a JSON error body.
func DeviceByIDHandler(s *store.DeviceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		deviceID := strings.TrimPrefix(r.URL.Path, "/devices/")
		if deviceID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "device not found"})
			return
		}

		state, ok := s.Get(deviceID)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "device not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(state)
	}
}
