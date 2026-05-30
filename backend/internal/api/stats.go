package api

import (
	"encoding/json"
	"net/http"

	"github.com/certainelf/pulseops/backend/internal/store"
)

type statsResponse struct {
	ActiveAgents       int      `json:"activeAgents"`
	ActiveAgentDevices []string `json:"activeAgentDevices"`
}

// StatsHandler returns an http.HandlerFunc for GET /stats.
// It currently exposes the number of active agents seen by the backend.
func StatsHandler(deviceStore *store.DeviceStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		states := deviceStore.List()
		deviceIDs := make([]string, 0, len(states))
		for _, state := range states {
			if state.DeviceID == "" {
				continue
			}
			deviceIDs = append(deviceIDs, state.DeviceID)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(statsResponse{
			ActiveAgents:       len(deviceIDs),
			ActiveAgentDevices: deviceIDs,
		})
	}
}
