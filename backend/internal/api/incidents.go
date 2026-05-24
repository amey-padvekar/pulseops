package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// IncidentsHandler returns an http.HandlerFunc for GET /incidents.
// Supports optional filters via query params: active, deviceId, state.
func IncidentsHandler(s *incidents.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		filter := incidents.IncidentFilter{}
		query := r.URL.Query()

		if activeRaw := strings.TrimSpace(query.Get("active")); activeRaw != "" {
			active, err := strconv.ParseBool(activeRaw)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid active filter"})
				return
			}
			filter.Active = &active
		}

		filter.DeviceID = strings.TrimSpace(query.Get("deviceId"))
		if stateRaw := strings.TrimSpace(query.Get("state")); stateRaw != "" {
			filter.State = incidents.IncidentState(stateRaw)
		}

		result := s.List(filter)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// IncidentByIDHandler returns an http.HandlerFunc for GET /incidents/{incidentId}.
func IncidentByIDHandler(s *incidents.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		incidentID := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/incidents/"))
		if incidentID == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "incident not found"})
			return
		}

		incident, ok := s.GetByID(incidentID)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "incident not found"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(incident)
	}
}
