package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
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

func telemetryHandler(w http.ResponseWriter, r *http.Request) {
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
		"telemetry received request_id=%s request_attempt=%s device_id=%s device_header=%s timestamp=%s service=%s service_status=%s heartbeat=%t network_reachable=%t cpu_usage=%.2f memory_usage=%.2f logs=%d",
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

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/telemetry", telemetryHandler)

	addr := ":" + cfg.Port
	log.Printf("backend starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "backend server error: %v\n", err)
		os.Exit(1)
	}
}
