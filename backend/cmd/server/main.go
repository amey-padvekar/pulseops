package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

type Config struct {
	Port string
}

type TelemetryPayload struct {
	SchemaVersion    string   `json:"schemaVersion"`
	DeviceID         string   `json:"deviceId"`
	Timestamp        string   `json:"timestamp"`
	Heartbeat        bool     `json:"heartbeat"`
	ServiceName      string   `json:"serviceName"`
	ServiceStatus    string   `json:"serviceStatus"`
	NetworkReachable bool     `json:"networkReachable"`
	CPUUsage         float64  `json:"cpuUsage"`
	MemoryUsage      float64  `json:"memoryUsage"`
	RecentLogs       []string `json:"recentLogs"`
}

func loadConfig() Config {
	port := os.Getenv("BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	return Config{Port: port}
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

func makeTelemetryHandler(deviceStore *store.DeviceStore, incidentStore *incidents.Store, hub *ws.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		defer r.Body.Close()

		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "failed to read telemetry body", http.StatusBadRequest)
			return
		}

		var payload TelemetryPayload
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid telemetry payload", http.StatusBadRequest)
			return
		}

		if err := validateTelemetryPayload(payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		deviceStore.Upsert(store.DeviceState{
			DeviceID:         payload.DeviceID,
			Timestamp:        payload.Timestamp,
			ServiceName:      payload.ServiceName,
			ServiceStatus:    payload.ServiceStatus,
			NetworkReachable: payload.NetworkReachable,
			CPUUsage:         payload.CPUUsage,
			MemoryUsage:      payload.MemoryUsage,
			RecentLogs:       payload.RecentLogs,
			Heartbeat:        payload.Heartbeat,
		})

		state, ok := deviceStore.Get(payload.DeviceID)
		if ok {
			if incidentStore != nil {
				if incident, shouldBroadcast := processTelemetryIncident(incidentStore, state); shouldBroadcast {
					ws.BroadcastIncidentUpdated(hub, incident)
				}
			}
			ws.BroadcastTelemetryUpdated(hub, state)
		}

		requestID := strings.TrimSpace(r.Header.Get("X-PulseOps-Request-ID"))
		if requestID == "" {
			requestID = "missing"
		}

		requestAttempt := strings.TrimSpace(r.Header.Get("X-PulseOps-Request-Attempt"))
		if requestAttempt == "" {
			requestAttempt = "1"
		}

		deviceHeader := strings.TrimSpace(r.Header.Get("X-PulseOps-Device-ID"))

		log.Printf(
			"telemetry received request_id=%s request_attempt=%s device_id=%s device_header=%s timestamp=%s service=%s service_status=%s heartbeat=%t network_reachable=%t cpu_usage=%.2f memory_usage=%.2f logs=%d state_updated=true",
			requestID,
			requestAttempt,
			payload.DeviceID,
			deviceHeader,
			payload.Timestamp,
			payload.ServiceName,
			payload.ServiceStatus,
			payload.Heartbeat,
			payload.NetworkReachable,
			payload.CPUUsage,
			payload.MemoryUsage,
			len(payload.RecentLogs),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status":    "accepted",
			"requestId": requestID,
		})
	}
}

func processTelemetryIncident(incidentStore *incidents.Store, state store.DeviceState) (incidents.Incident, bool) {
	detection := incidents.EvaluateTelemetry(state)
	if !detection.ShouldCreateOrUpdate {
		return incidents.Incident{}, false
	}

	seed := incidents.NewIncident(
		"",
		state.DeviceID,
		state.ServiceName,
		state.ServiceStatus,
		detection.Severity,
		detection.Reason,
	)

	incident, created := incidentStore.CreateOrGetActive(detection.DedupeKey, seed)
	if created {
		next, err := incidentStore.UpdateState(incident.IncidentID, incidents.StateInvestigating, detection.Reason)
		if err == nil {
			incident = next
		}
		return incident, true
	}

	seenAt := state.LastSeenAt
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	touched, err := incidentStore.Touch(incident.IncidentID, seenAt)
	if err == nil {
		incident = touched
	}

	return incident, true
}

func validateTelemetryPayload(payload TelemetryPayload) error {
	if strings.TrimSpace(payload.SchemaVersion) == "" {
		return fmt.Errorf("schemaVersion is required")
	}
	if strings.TrimSpace(payload.DeviceID) == "" {
		return fmt.Errorf("deviceId is required")
	}
	if strings.TrimSpace(payload.Timestamp) == "" {
		return fmt.Errorf("timestamp is required")
	}
	if strings.TrimSpace(payload.ServiceName) == "" {
		return fmt.Errorf("serviceName is required")
	}
	if strings.TrimSpace(payload.ServiceStatus) == "" {
		return fmt.Errorf("serviceStatus is required")
	}
	if payload.RecentLogs == nil {
		return fmt.Errorf("recentLogs is required")
	}
	return nil
}

func main() {
	cfg := loadConfig()

	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	hub := ws.NewHub()
	go hub.Run()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/telemetry", makeTelemetryHandler(deviceStore, incidentStore, hub))
	mux.HandleFunc("/devices", api.DevicesHandler(deviceStore))
	mux.HandleFunc("/devices/", api.DeviceByIDHandler(deviceStore))
	mux.HandleFunc("/incidents", api.IncidentsHandler(incidentStore))
	mux.HandleFunc("/incidents/", api.IncidentByIDHandler(incidentStore))
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWs(hub, w, r)
	})
	handler := api.CORSMiddleware(mux)

	addr := ":" + cfg.Port
	log.Printf("backend starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "backend server error: %v\n", err)
		os.Exit(1)
	}
}
