package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/agentbuilder"
	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/elastic"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
	"github.com/joho/godotenv"
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

func makeTelemetryHandler(
	deviceStore *store.DeviceStore,
	incidentStore *incidents.Store,
	hub *ws.Hub,
	elasticClient elastic.Indexer,
	elasticCfg *elastic.Config,
	agentClient agentbuilder.Client,
	agentCfg *agentbuilder.Config,
) http.HandlerFunc {
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
				if incident, shouldBroadcast, shouldHandoff := processTelemetryIncident(incidentStore, state); shouldBroadcast {

					if elasticClient != nil && elasticClient.Enabled() {

						incidentDoc := elastic.IncidentEventDocument{
							SchemaVersion: "v1",

							EventType: "incident_updated",
							Timestamp: time.Now().UTC(),

							IncidentID: incident.IncidentID,

							DeviceID:      incident.DeviceID,
							ServiceName:   incident.ServiceName,
							ServiceStatus: incident.ServiceStatus,

							Severity: string(incident.Severity),
							State:    string(incident.State),

							Reason: incident.Reason,
							Active: incident.Active,
						}

						go func(doc elastic.IncidentEventDocument) {

							ctx, cancel := context.WithTimeout(
								context.Background(),
								5*time.Second,
							)
							defer cancel()

							if err := elasticClient.IndexIncidentEvent(ctx, doc); err != nil {
								log.Printf(
									"elastic incident indexing failed incident_id=%s error=%v",
									doc.IncidentID,
									err,
								)
							}

						}(incidentDoc)
					}

					ws.BroadcastIncidentUpdated(hub, incident)

					if shouldHandoff {
						go submitAgentBuilderRequest(
							agentClient,
							agentCfg,
							incidentStore,
							deviceStore,
							incident,
							elasticCfg,
						)
					}
				}
			}
			ws.BroadcastTelemetryUpdated(hub, state)
			// -- phase 5: step 4.7: add elastic log ingestion
			if elasticClient != nil && elasticClient.Enabled() {

				telemetryDoc := elastic.TelemetryEventDocument{
					SchemaVersion: "v1",

					EventType: "telemetry_received",
					Timestamp: state.LastSeenAt,

					DeviceID:      state.DeviceID,
					ServiceName:   state.ServiceName,
					ServiceStatus: state.ServiceStatus,

					Heartbeat:        state.Heartbeat,
					NetworkReachable: state.NetworkReachable,

					CPUUsage:    state.CPUUsage,
					MemoryUsage: state.MemoryUsage,

					RecentLogs: state.RecentLogs,
				}

				go func(doc elastic.TelemetryEventDocument) {

					ctx, cancel := context.WithTimeout(
						context.Background(),
						5*time.Second,
					)
					defer cancel()

					if err := elasticClient.IndexTelemetryEvent(ctx, doc); err != nil {
						log.Printf(
							"elastic telemetry indexing failed device_id=%s error=%v",
							doc.DeviceID,
							err,
						)
					}

				}(telemetryDoc)

				go func() {

					ctx, cancel := context.WithTimeout(
						context.Background(),
						5*time.Second,
					)
					defer cancel()

					err := elasticClient.IndexRecentLogs(
						ctx,
						state.DeviceID,
						state.ServiceName,
						"",
						state.RecentLogs,
					)

					if err != nil {
						log.Printf(
							"elastic log indexing failed device_id=%s error=%v",
							state.DeviceID,
							err,
						)
					}

				}()
			}
			// -- end of phase 5: step 4.7
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

func processTelemetryIncident(incidentStore *incidents.Store, state store.DeviceState) (incidents.Incident, bool, bool) {
	detection := incidents.EvaluateTelemetry(state)
	if !detection.ShouldCreateOrUpdate {
		return incidents.Incident{}, false, false
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
		return incident, true, true
	}

	seenAt := state.LastSeenAt
	if seenAt.IsZero() {
		seenAt = time.Now().UTC()
	}
	touched, err := incidentStore.Touch(incident.IncidentID, seenAt)
	if err == nil {
		incident = touched
	}

	return incident, true, false
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

func submitAgentBuilderRequest(
	agentClient agentbuilder.Client,
	agentCfg *agentbuilder.Config,
	incidentStore *incidents.Store,
	deviceStore *store.DeviceStore,
	incident incidents.Incident,
	elasticCfg *elastic.Config,
) {
	if agentClient == nil || agentCfg == nil || !agentCfg.Enabled {
		return
	}

	opts := agentbuilder.BuildRequestOptions{
		RequestedAt:        time.Now().UTC(),
		ElasticIndexConfig: elasticIndexConfigFromConfig(elasticCfg),
	}

	req, err := agentbuilder.BuildRequest(incident.IncidentID, incidentStore, deviceStore, opts)
	if err != nil {
		log.Printf(
			"agent builder request build failed incident_id=%s device_id=%s service_name=%s error=%v",
			incident.IncidentID,
			incident.DeviceID,
			incident.ServiceName,
			err,
		)
		return
	}

	log.Printf(agentbuilder.FormatRequestLog(req, agentCfg.Endpoint))

	ctx, cancel := context.WithTimeout(context.Background(), agentCfg.Timeout)
	defer cancel()

	resp, err := agentClient.SubmitInvestigation(ctx, req)
	if err != nil {
		traceID := resp.TraceID
		log.Printf(
			"agent builder submit failed request_id=%s incident_id=%s device_id=%s service_name=%s trace_id=%s error=%v",
			req.RequestID,
			incident.IncidentID,
			incident.DeviceID,
			incident.ServiceName,
			traceID,
			err,
		)
		return
	}

	log.Printf(agentbuilder.FormatResponseLog(resp))
}

func elasticIndexConfigFromConfig(cfg *elastic.Config) *agentbuilder.ElasticIndexConfig {
	if cfg == nil {
		return nil
	}

	return &agentbuilder.ElasticIndexConfig{
		Telemetry: cfg.IndexTelemetry,
		Incidents: cfg.IndexIncidents,
		Logs:      cfg.IndexLogs,
	}
}

func main() {
	// Load backend/.env when running locally; environment variables still take precedence.
	if err := godotenv.Load(); err != nil {
		log.Printf("dotenv not loaded: %v", err)
	}

	cfg := loadConfig()

	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	hub := ws.NewHub()
	go hub.Run()

	//-- phase 5: step 4.7: add elastic log ingestion endpoint
	elasticCfg, err := elastic.NewConfig()
	if err != nil {
		log.Fatalf("elastic config error: %v", err)
	}

	elasticClient, err := elastic.NewClient(elasticCfg)
	if err != nil {
		log.Fatalf("elastic client error: %v", err)
	}

	if elasticClient.Enabled() {
		log.Printf(
			"elastic enabled endpoint=%s index_telemetry=%s index_incidents=%s index_logs=%s",
			elasticCfg.Endpoint,
			elasticCfg.IndexTelemetry,
			elasticCfg.IndexIncidents,
			elasticCfg.IndexLogs,
		)

		if err := elasticClient.Ping(context.Background()); err != nil {
			log.Printf("elastic ping failed: %v", err)
		} else {
			log.Println("elastic ping successful")
		}

		if err := elasticClient.EnsureIndexTemplates(context.Background()); err != nil {
			log.Printf("elastic template setup failed: %v", err)
		} else {
			log.Println("elastic template setup successful")
		}
	} else {
		missingEndpoint := elasticCfg.Endpoint == ""
		missingKey := elasticCfg.APIKey == ""
		log.Printf(
			"elastic disabled endpoint_set=%t api_key_set=%t",
			!missingEndpoint,
			!missingKey,
		)
	}
	//-- end of phase 5: step 4.7

	agentCfg, err := agentbuilder.NewConfig()
	if err != nil {
		log.Fatalf("agent builder config error: %v", err)
	}

	var agentClient agentbuilder.Client
	if agentCfg.Enabled {
		httpClient, err := agentbuilder.NewHTTPClient(agentbuilder.HTTPClientOptions{
			Endpoint:   agentCfg.Endpoint,
			AuthToken:  agentCfg.AuthToken,
			Timeout:    agentCfg.Timeout,
			MaxRetries: 1,
		})
		if err != nil {
			log.Fatalf("agent builder client error: %v", err)
		}
		agentClient = httpClient
		log.Printf(
			"agent builder enabled endpoint=%s timeout_ms=%d retries=%d",
			agentCfg.Endpoint,
			agentCfg.Timeout.Milliseconds(),
			1,
		)
	} else {
		log.Printf(
			"agent builder disabled endpoint_set=%t",
			agentCfg.Endpoint != "",
		)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc(
		"/telemetry",
		makeTelemetryHandler(deviceStore, incidentStore, hub, elasticClient, elasticCfg, agentClient, agentCfg),
	)
	mux.HandleFunc("/devices", api.DevicesHandler(deviceStore))
	mux.HandleFunc("/devices/", api.DeviceByIDHandler(deviceStore))
	mux.HandleFunc("/stats", api.StatsHandler(deviceStore))
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
