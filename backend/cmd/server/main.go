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
	"github.com/certainelf/pulseops/backend/internal/remediation"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
	"github.com/joho/godotenv"
)

type Config struct {
	Port     string
	DemoMode bool
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
	// Cloud Run (and most PaaS) inject PORT; prefer it, then BACKEND_PORT, then default.
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("BACKEND_PORT")
	}
	if port == "" {
		port = "8080"
	}

	// DEMO_MODE gates the additive, judge-facing "Simulate Service Failure" endpoints
	// (Part B3/B5). Same env-bool idiom as AGENT_BUILDER_ENABLED; defaults off so the
	// normal (non-demo) flow is unaffected unless explicitly enabled.
	demoMode := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("DEMO_MODE"))) {
	case "true", "1", "yes":
		demoMode = true
	}

	return Config{Port: port, DemoMode: demoMode}
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

		state, ok := ingestTelemetrySample(
			store.DeviceState{
				DeviceID:         payload.DeviceID,
				Timestamp:        payload.Timestamp,
				ServiceName:      payload.ServiceName,
				ServiceStatus:    payload.ServiceStatus,
				NetworkReachable: payload.NetworkReachable,
				CPUUsage:         payload.CPUUsage,
				MemoryUsage:      payload.MemoryUsage,
				RecentLogs:       payload.RecentLogs,
				Heartbeat:        payload.Heartbeat,
			},
			deviceStore,
			incidentStore,
			hub,
			elasticClient,
			elasticCfg,
			agentClient,
			agentCfg,
		)

		if ok {
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

// ingestTelemetrySample runs the shared telemetry → incident pipeline for one device state
// and returns the stored (LastSeenAt-stamped) copy. It is the single core shared by the real
// /telemetry handler and the DEMO_MODE /demo/* endpoints (Part B), so both drive the identical
// path: upsert the state, run detection (broadcasting and, for a newly opened incident, handing
// off to the Gemini + Elastic MCP investigation), feed any validating incident the fresh
// observation (advancing recovery and triggering the closing summary on resolution), and
// broadcast the telemetry update. HTTP concerns and raw-telemetry Elastic indexing stay with
// the caller. The bool is false only when the just-upserted state cannot be read back.
func ingestTelemetrySample(
	state store.DeviceState,
	deviceStore *store.DeviceStore,
	incidentStore *incidents.Store,
	hub *ws.Hub,
	elasticClient elastic.Indexer,
	elasticCfg *elastic.Config,
	agentClient agentbuilder.Client,
	agentCfg *agentbuilder.Config,
) (store.DeviceState, bool) {
	deviceStore.Upsert(state)

	stored, ok := deviceStore.Get(state.DeviceID)
	if !ok {
		return store.DeviceState{}, false
	}

	if incidentStore != nil {
		if incident, shouldBroadcast, shouldHandoff := processTelemetryIncident(incidentStore, stored); shouldBroadcast {
			publishIncidentUpdate(hub, elasticClient, incident)

			if shouldHandoff {
				go submitAgentBuilderRequest(
					agentClient,
					agentCfg,
					incidentStore,
					deviceStore,
					incident,
					elasticClient,
					hub,
					elasticCfg,
				)
			}
		}

		// Phase 10 step 4.6: recovery validation is part of the telemetry pipeline. Any
		// incident for this device that is validating consumes the fresh telemetry as
		// recovery evidence; admitted observations advance the healthy-cycle counter and
		// may resolve the incident.
		for _, updated := range processTelemetryValidation(incidentStore, stored) {
			publishIncidentUpdate(hub, elasticClient, updated)
			// A resolved incident triggers final-summary generation (Phase 11).
			triggerSummaryGeneration(incidentStore, hub, elasticClient, updated)
		}
	}

	ws.BroadcastTelemetryUpdated(hub, stored)

	return stored, true
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

// processTelemetryValidation feeds a fresh telemetry snapshot into recovery validation
// for any incident on the same device that is currently validating (Phase 10 step 4.6).
// It returns the incidents whose validation actually advanced (a fresh observation was
// admitted), so the caller can broadcast them. Stale or out-of-phase telemetry yields no
// updates. incidentStore must not be nil.
func processTelemetryValidation(incidentStore *incidents.Store, state store.DeviceState) []incidents.Incident {
	validating := incidentStore.List(incidents.IncidentFilter{
		DeviceID: state.DeviceID,
		State:    incidents.StateValidating,
	})
	if len(validating) == 0 {
		return nil
	}

	var updates []incidents.Incident
	for _, inc := range validating {
		outcome, err := incidentStore.RecordValidationObservation(inc.IncidentID, state, incidents.DefaultHealthCriteria())
		if err != nil {
			log.Printf(
				"validation observation failed incident_id=%s device_id=%s error=%v",
				inc.IncidentID, state.DeviceID, err,
			)
			continue
		}
		if !outcome.Admitted {
			continue
		}
		log.Printf(
			"validation observation incident_id=%s device_id=%s healthy=%t cycles=%d/%d resolved=%t reason=%q",
			outcome.Incident.IncidentID, state.DeviceID, outcome.Healthy,
			outcome.Incident.HealthyCycleCount, outcome.Incident.RequiredHealthyCycles,
			outcome.Resolved, outcome.Evaluation.Reason,
		)
		updates = append(updates, outcome.Incident)
	}
	return updates
}

// publishIncidentUpdate indexes an incident lifecycle change to Elastic (best effort,
// asynchronous) and broadcasts it to connected dashboard clients. It centralizes the
// index+broadcast pattern shared by detection (Phase 4) and validation (Phase 10), so an
// incident change reaches the frontend the same way regardless of what caused it. hub and
// elasticClient may be nil.
func publishIncidentUpdate(hub *ws.Hub, elasticClient elastic.Indexer, incident incidents.Incident) {
	if elasticClient != nil && elasticClient.Enabled() {
		doc := elastic.IncidentEventDocument{
			SchemaVersion: "v1",
			EventType:     "incident_updated",
			Timestamp:     time.Now().UTC(),
			IncidentID:    incident.IncidentID,
			DeviceID:      incident.DeviceID,
			ServiceName:   incident.ServiceName,
			ServiceStatus: incident.ServiceStatus,
			Severity:      string(incident.Severity),
			State:         string(incident.State),
			Reason:        incident.Reason,
			Active:        incident.Active,
		}
		go func(doc elastic.IncidentEventDocument) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := elasticClient.IndexIncidentEvent(ctx, doc); err != nil {
				log.Printf("elastic incident indexing failed incident_id=%s error=%v", doc.IncidentID, err)
			}
		}(doc)
	}
	ws.BroadcastIncidentUpdated(hub, incident)
}

// Summary generation runtime (Phase 11 step 4.10). Configured once in main(); left disabled
// by default so terminal incidents in tests are unaffected by summary generation.
var (
	summaryGenEnabled   bool
	summaryGenSubmitter agentbuilder.SummarySubmitter
	summaryGenTimeout   time.Duration
)

// configureSummaryGeneration enables automatic final-summary generation at incident
// closure. submitter may be nil, in which case every terminal incident still receives a
// deterministic fallback summary synthesized from the stored record — so the closing report
// never becomes a single point of demo failure.
func configureSummaryGeneration(submitter agentbuilder.SummarySubmitter, timeout time.Duration) {
	summaryGenEnabled = true
	summaryGenSubmitter = submitter
	summaryGenTimeout = timeout
}

// triggerSummaryGeneration asynchronously generates and stores the final summary for a
// terminal incident, then re-broadcasts the enriched incident once the report is ready. It
// is a no-op when generation is disabled or the incident is not resolved/failed. Running
// asynchronously keeps the (possibly multi-second) generation off the telemetry and sweeper
// hot paths; the dashboard shows a "generating…" state until the follow-up broadcast lands.
func triggerSummaryGeneration(incidentStore *incidents.Store, hub *ws.Hub, elasticClient elastic.Indexer, inc incidents.Incident) {
	if !summaryGenEnabled || incidentStore == nil {
		return
	}
	if inc.State != incidents.StateResolved && inc.State != incidents.StateFailed {
		return
	}

	go func() {
		updated, changed, err := agentbuilder.GenerateAndStoreSummary(
			context.Background(),
			incidentStore,
			summaryGenSubmitter,
			inc.IncidentID,
			agentbuilder.SummaryGenerationConfig{
				Timeout: summaryGenTimeout,
				Mode:    agentbuilder.SummaryTriggerAuto,
			},
		)
		if err != nil {
			log.Printf("summary generation error incident_id=%s error=%v", inc.IncidentID, err)
			return
		}
		if changed {
			publishIncidentUpdate(hub, elasticClient, updated)
		}
	}()
}

// validationSweepInterval is how often the background sweeper checks for incidents that
// have exceeded the validation timeout. It is shorter than the timeout so a timed-out
// incident is failed promptly rather than at most one full timeout late.
const validationSweepInterval = 10 * time.Second

// startValidationTimeoutSweeper launches a background ticker that periodically fails
// incidents stuck in validating past the timeout (Phase 10 step 4.5/4.6) and broadcasts
// each failure to the dashboard. It runs for the lifetime of the process. It is a no-op
// when incidentStore is nil. interval and timeout fall back to sane defaults if
// non-positive.
func startValidationTimeoutSweeper(incidentStore *incidents.Store, hub *ws.Hub, elasticClient elastic.Indexer, interval, timeout time.Duration) {
	if incidentStore == nil {
		return
	}
	if interval <= 0 {
		interval = validationSweepInterval
	}
	if timeout <= 0 {
		timeout = incidents.DefaultValidationTimeout
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			failed := incidentStore.ExpireTimedOutValidations(time.Now().UTC(), timeout)
			for _, inc := range failed {
				log.Printf(
					"validation timed out incident_id=%s device_id=%s reason=%q",
					inc.IncidentID, inc.DeviceID, inc.ValidationFailureReason,
				)
				publishIncidentUpdate(hub, elasticClient, inc)
				// A failed incident triggers final-summary generation (Phase 11).
				triggerSummaryGeneration(incidentStore, hub, elasticClient, inc)
			}
		}
	}()
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
	elasticClient elastic.Indexer,
	hub *ws.Hub,
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

	// Attach a compact evidence summary from the direct-ES retriever (the offline
	// hints/evidence producer). The meaningful Elastic MCP access happens inside the
	// agent (W2/W3), which calls Elastic's Agent Builder MCP tools during Gemini reasoning.
	if elasticClient != nil && elasticClient.Enabled() {
		// RetrieveAndSummarizeEvidence requires a concrete *elastic.Client (uses Search).
		if concrete, ok := elasticClient.(*elastic.Client); ok {
			evidence, err := agentbuilder.RetrieveAndSummarizeEvidence(context.Background(), concrete, req.ElasticContextHints)
			if err != nil {
				log.Printf("agent builder evidence retrieval failed incident_id=%s device_id=%s error=%v", req.IncidentID, req.DeviceID, err)
			} else {
				req.EvidenceSummary = evidence
			}
		} else {
			log.Printf("elastic client not concrete; skipping evidence retrieval")
		}
	}

	log.Print(agentbuilder.FormatRequestLog(req, agentCfg.Endpoint))

	ctx, cancel := context.WithTimeout(context.Background(), agentCfg.Timeout)
	defer cancel()

	resp, err := agentClient.SubmitInvestigation(ctx, req)
	if err != nil {
		traceID := resp.TraceID
		evidenceLines := 0
		if strings.TrimSpace(req.EvidenceSummary) != "" {
			evidenceLines = len(strings.Split(strings.TrimSpace(req.EvidenceSummary), "\n"))
		}
		log.Printf(
			"agent builder submit failed request_id=%s incident_id=%s device_id=%s service_name=%s trace_id=%s error=%v",
			req.RequestID,
			incident.IncidentID,
			incident.DeviceID,
			incident.ServiceName,
			traceID,
			err,
		)

		// Determine failure cause (timeout vs other)
		status := "failed"
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				status = "timeout"
			} else {
				status = "cancelled"
			}
		}
		log.Printf(
			"agent builder submit status request_id=%s incident_id=%s device_id=%s status=%s timeout_ms=%d",
			req.RequestID,
			incident.IncidentID,
			incident.DeviceID,
			status,
			agentCfg.Timeout.Milliseconds(),
		)

		// Persist failure metadata on incident without changing lifecycle
		if incidentStore != nil {
			updatedFailure, serr := incidentStore.SaveInvestigationFailure(incident.IncidentID, status, err.Error(), traceID)
			if serr != nil {
				log.Printf("failed to save investigation failure incident_id=%s error=%v", incident.IncidentID, serr)
			} else if hub != nil {
				ws.BroadcastIncidentUpdated(hub, updatedFailure)
			}
		}

		log.Printf("agent_builder_trace request_id=%s incident_id=%s device_id=%s agent_builder_trace_id=%s investigation_status=%s evidence_lines=%d confidence=%.2f actions=%s",
			req.RequestID,
			incident.IncidentID,
			incident.DeviceID,
			traceID,
			status,
			evidenceLines,
			0.0,
			"",
		)

		// Fallback: if configured, synthesize a local stub result for demo/dev.
		//   local_stub         -> empty recommendation; incident stays investigating
		//                         (never becomes approvable), matching the safety design.
		//   local_stub_actions -> deterministic, catalog-valid recommendation so the
		//                         Phase 8 approval gate can be exercised offline. The
		//                         incident is promoted to awaiting_approval.
		fallbackMode := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BUILDER_FALLBACK_MODE")))
		if fallbackMode == "local_stub" || fallbackMode == "local_stub_actions" {
			withActions := fallbackMode == "local_stub_actions"

			cause := "fallback: investigation unavailable"
			summary := "Investigation unavailable; fallback placeholder used."
			status := "fallback"
			confidence := 0.0
			recs := []incidents.RecommendedAction{}
			steps := []string{}

			if withActions {
				cause = "fallback stub: monitored service stopped while heartbeat present"
				summary = "Local stub recommendation for demo/rehearsal; restart the stopped service."
				status = "completed"
				confidence = 0.5
				recs = []incidents.RecommendedAction{
					{ActionID: "restart_service", Target: incident.ServiceName, Description: "restart the stopped service"},
				}
				steps = []string{"confirm the service returns to running", "verify heartbeat remains healthy"}
			}

			if incidentStore != nil {
				updatedFallback, serr := incidentStore.SaveInvestigationResult(
					incident.IncidentID,
					cause,
					confidence,
					recs,
					steps,
					summary,
					time.Now().UTC(),
					"",
					status,
				)
				if serr != nil {
					log.Printf("failed to save fallback investigation result incident_id=%s error=%v", incident.IncidentID, serr)
				} else {
					// Only the actions stub carries a concrete recommendation, so only it
					// is eligible for the investigating -> awaiting_approval handoff.
					if withActions {
						if promoted, ok, perr := incidentStore.PromoteToAwaitingApproval(incident.IncidentID); perr != nil {
							log.Printf("failed to promote fallback incident to awaiting_approval incident_id=%s error=%v", incident.IncidentID, perr)
						} else if ok {
							updatedFallback = promoted
							log.Printf("incident promoted to awaiting_approval incident_id=%s device_id=%s actions=%d (local_stub_actions)", promoted.IncidentID, promoted.DeviceID, len(promoted.RecommendedActions))
						}
					}
					if hub != nil {
						ws.BroadcastIncidentUpdated(hub, updatedFallback)
					}
				}
			}

			actionIDs := make([]string, 0, len(recs))
			for _, a := range recs {
				actionIDs = append(actionIDs, a.ActionID)
			}

			log.Printf("agent_builder_trace request_id=%s incident_id=%s device_id=%s agent_builder_trace_id=%s investigation_status=%s evidence_lines=%d confidence=%.2f actions=%s",
				req.RequestID,
				incident.IncidentID,
				incident.DeviceID,
				"",
				status,
				evidenceLines,
				confidence,
				strings.Join(actionIDs, ","),
			)
		}

		return
	}

	log.Print(agentbuilder.FormatResponseLog(resp))

	// If the workflow returned a payload, attempt to parse and persist it.
	if len(resp.RawPayload) > 0 {
		parsed, perr := agentbuilder.ParseInvestigationResultWithAllowedActions(resp.RawPayload, req.AvailableActions)
		if perr != nil {
			log.Printf("agent builder parse failed incident_id=%s request_id=%s error=%v", incident.IncidentID, req.RequestID, perr)
			return
		}
		// Map recommended actions into incident-local representation
		recs := make([]incidents.RecommendedAction, 0, len(parsed.RecommendedActions))
		for _, a := range parsed.RecommendedActions {
			recs = append(recs, incidents.RecommendedAction{ActionID: a.ActionID, Target: a.Target, Description: a.Reason})
		}

		// Save investigation result to incident store (persist trace id and status)
		updated, err := incidentStore.SaveInvestigationResult(
			incident.IncidentID,
			parsed.ProbableCause,
			parsed.Confidence,
			recs,
			parsed.ValidationSteps,
			parsed.Summary,
			resp.ReceivedAt,
			resp.TraceID,
			"completed",
		)
		if err != nil {
			log.Printf("failed to save investigation result incident_id=%s error=%v", incident.IncidentID, err)
			return
		}

		// Phase 7 -> Phase 8 handoff: a completed investigation with a concrete
		// recommendation makes the incident approvable. The agent service keeps
		// lifecycle out of scope, so the backend owns this transition. The store
		// guards against fallback/empty results, so this is a no-op for stubs.
		if promoted, ok, perr := incidentStore.PromoteToAwaitingApproval(incident.IncidentID); perr != nil {
			log.Printf("failed to promote incident to awaiting_approval incident_id=%s error=%v", incident.IncidentID, perr)
		} else if ok {
			log.Printf("incident promoted to awaiting_approval incident_id=%s device_id=%s actions=%d", promoted.IncidentID, promoted.DeviceID, len(promoted.RecommendedActions))
			updated = promoted
		}

		// Log redacted traceability and a compact summary for demo debugging
		evidenceLines := 0
		if strings.TrimSpace(req.EvidenceSummary) != "" {
			evidenceLines = len(strings.Split(strings.TrimSpace(req.EvidenceSummary), "\n"))
		}

		var actionIDs []string
		for _, a := range parsed.RecommendedActions {
			actionIDs = append(actionIDs, a.ActionID)
		}

		log.Printf("agent_builder_trace request_id=%s incident_id=%s device_id=%s agent_builder_trace_id=%s investigation_status=%s evidence_lines=%d confidence=%.2f actions=%s",
			req.RequestID,
			incident.IncidentID,
			incident.DeviceID,
			resp.TraceID,
			updated.InvestigationStatus,
			evidenceLines,
			updated.Confidence,
			strings.Join(actionIDs, ","),
		)

		// Broadcast updated incident to websocket clients
		if hub != nil {
			ws.BroadcastIncidentUpdated(hub, updated)
		}
	}
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
	log.Printf("==================== PulseOps backend starting ====================")

	// Load backend/.env when running locally; environment variables still take precedence.
	if err := godotenv.Load(); err != nil {
		log.Printf("dotenv not loaded (using process environment only): %v", err)
	} else {
		log.Printf("dotenv loaded from backend/.env")
	}

	cfg := loadConfig()
	log.Printf("config loaded port=%s demo_mode=%t", cfg.Port, cfg.DemoMode)

	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()
	remediationQueue := remediation.NewQueue()
	hub := ws.NewHub()
	go hub.Run()
	log.Printf("core services initialized device_store=ok incident_store=ok remediation_queue=ok ws_hub=running")

	//-- phase 5: step 4.7: add elastic log ingestion endpoint
	log.Printf("initializing elastic integration...")
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
			"✅ ELASTIC ENABLED endpoint=%s index_telemetry=%s index_incidents=%s index_logs=%s",
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
			"⚠️  ELASTIC DISABLED endpoint_set=%t api_key_set=%t (set ELASTIC_ENDPOINT and ELASTIC_API_KEY to enable)",
			!missingEndpoint,
			!missingKey,
		)
	}
	//-- end of phase 5: step 4.7

	log.Printf("initializing agent builder integration...")
	agentCfg, err := agentbuilder.NewConfig()
	if err != nil {
		log.Fatalf("agent builder config error: %v", err)
	}

	var agentClient agentbuilder.Client
	// summarySubmitter is set only for the ADK transport, which implements SubmitSummary.
	// When it stays nil (agent disabled or HTTP transport), summary generation still runs
	// and produces a deterministic fallback report.
	var summarySubmitter agentbuilder.SummarySubmitter
	if agentCfg.Enabled {
		switch agentCfg.Transport {
		case "agent_engine":
			tokenProvider, err := newGoogleADCTokenProvider(context.Background())
			if err != nil {
				log.Fatalf("agent engine auth init error: %v", err)
			}
			aeClient, err := agentbuilder.NewAgentEngineClient(agentbuilder.AgentEngineClientOptions{
				Resource:   agentCfg.AgentEngineResource,
				Location:   agentCfg.GoogleLocation,
				Token:      tokenProvider,
				Timeout:    agentCfg.Timeout,
				MaxRetries: 3,
			})
			if err != nil {
				log.Fatalf("agent engine client error: %v", err)
			}
			agentClient = aeClient
			// summarySubmitter is intentionally nil for agent_engine: AgentEngineClient
			// does not implement SubmitSummary, so closure produces the deterministic
			// fallback summary (the agent's instruction targets InvestigationResult).
			log.Printf(
				"✅ AGENT BUILDER ENABLED transport=agent_engine resource=%s location=%s timeout_ms=%d retries=%d",
				agentCfg.AgentEngineResource,
				agentCfg.GoogleLocation,
				agentCfg.Timeout.Milliseconds(),
				1,
			)
		case "adk":
			adkClient, err := agentbuilder.NewADKClient(agentbuilder.ADKClientOptions{
				Endpoint:   agentCfg.ADKEndpoint,
				AuthToken:  agentCfg.AuthToken,
				Timeout:    agentCfg.Timeout,
				MaxRetries: 1,
			})
			if err != nil {
				log.Fatalf("agent builder ADK client error: %v", err)
			}
			agentClient = adkClient
			summarySubmitter = adkClient
			log.Printf(
				"✅ AGENT BUILDER ENABLED transport=adk endpoint=%s timeout_ms=%d retries=%d",
				agentCfg.ADKEndpoint,
				agentCfg.Timeout.Milliseconds(),
				1,
			)
		default:
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
				"✅ AGENT BUILDER ENABLED transport=http endpoint=%s timeout_ms=%d retries=%d",
				agentCfg.Endpoint,
				agentCfg.Timeout.Milliseconds(),
				1,
			)
		}
	} else {
		fallbackMode := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BUILDER_FALLBACK_MODE")))
		if fallbackMode == "" {
			fallbackMode = "none"
		}
		log.Printf(
			"⚠️  AGENT BUILDER DISABLED endpoint_set=%t fallback_mode=%s (set AGENT_BUILDER_ENABLED=true to enable)",
			agentCfg.Endpoint != "",
			fallbackMode,
		)
	}

	// Phase 11 step 4.10: enable automatic final-summary generation at incident closure.
	// A nil submitter (agent disabled or HTTP transport) still yields a deterministic
	// fallback summary, so a completed incident always has a readable closing report.
	configureSummaryGeneration(summarySubmitter, agentCfg.SummaryTimeout)
	log.Printf(
		"summary generation enabled live_backend=%t timeout_ms=%d",
		summarySubmitter != nil,
		agentCfg.SummaryTimeout.Milliseconds(),
	)

	log.Printf("registering HTTP routes...")
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
	mux.HandleFunc("POST /incidents/{incidentId}/approve", api.IncidentApprovalHandler(incidentStore, func(updated incidents.Incident) {
		// Approval audit evidence: who approved which actions, on which device, when.
		// Emitted first so the record exists regardless of downstream queuing outcome.
		log.Print(incidents.ApprovalAuditLine(updated))

		// Build a queued remediation command from the approved snapshot so Phase 9
		// can consume a trustworthy payload without reconstructing intent.
		if cmd, err := remediation.NewCommand(updated); err != nil {
			log.Printf("failed to queue remediation command incident_id=%s error=%v", updated.IncidentID, err)
		} else {
			remediationQueue.Enqueue(cmd)
			log.Printf("remediation command queued incident_id=%s device_id=%s approved_by=%s actions=%d status=%s",
				cmd.IncidentID, cmd.DeviceID, cmd.ApprovedBy, len(cmd.Actions), cmd.Status)

			// Record the command_queued milestone on the execution timeline (task 4.8.2)
			// and broadcast the timeline-enriched incident.
			if enriched, terr := incidentStore.AppendTimelineEvent(cmd.IncidentID, incidents.EventCommandQueued, time.Now().UTC(), ""); terr != nil {
				log.Printf("failed to record queued timeline event incident_id=%s error=%v", cmd.IncidentID, terr)
			} else {
				updated = enriched
			}
		}
		if hub != nil {
			ws.BroadcastIncidentUpdated(hub, updated)
		}
	}))
	mux.HandleFunc("/incidents/", api.IncidentByIDHandler(incidentStore))
	// Phase 9 step 4.3: device-scoped remediation command dispatch. The agent polls
	// this to fetch commands approved for its device; fetching transitions each command
	// queued -> dispatched and the incident -> executing (step 4.7.3). The pattern is
	// more specific than "/devices/", so it takes precedence over DeviceByIDHandler.
	mux.HandleFunc("GET /devices/{deviceId}/commands", api.PendingCommandsHandler(remediationQueue, incidentStore, func(updated incidents.Incident) {
		ws.BroadcastIncidentUpdated(hub, updated)
	}))
	// Phase 9 step 4.7: remediation result ingestion. The agent posts execution
	// outcomes here; the backend persists them and advances the incident lifecycle.
	mux.HandleFunc("POST /remediation/results", api.RemediationResultHandler(incidentStore, remediationQueue, func(updated incidents.Incident) {
		ws.BroadcastIncidentUpdated(hub, updated)
	}))
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWs(hub, w, r)
	})

	// Demo Mode (DEMO_MODE=true): additive, judge-facing "Simulate Service Failure"
	// endpoints (Part B). Registered ONLY when enabled so the normal (non-demo) flow is
	// never affected. The /demo/* handlers are added inside this gate in B3/B5.
	if registerDemoRoutes(mux, cfg.DemoMode, deviceStore, incidentStore, hub, elasticClient, elasticCfg, agentClient, agentCfg) {
		log.Printf("🧪 DEMO MODE ENABLED routes=POST /demo/incident, POST /demo/reset (judge-facing simulation)")
	} else {
		log.Printf("demo mode disabled (set DEMO_MODE=true to enable /demo/* endpoints)")
	}

	handler := api.CORSMiddleware(mux)
	log.Printf("HTTP routes registered (telemetry, devices, stats, incidents, remediation, ws) cors=enabled")

	// Phase 10 step 4.6: a periodic sweep fails incidents that linger in validating past
	// the timeout, so the failure threshold fires even when telemetry stops arriving
	// entirely (a dead endpoint never produces a fresh observation to drive resolution).
	startValidationTimeoutSweeper(incidentStore, hub, elasticClient, validationSweepInterval, incidents.DefaultValidationTimeout)
	log.Printf(
		"validation timeout sweeper started interval=%s timeout=%s",
		validationSweepInterval,
		incidents.DefaultValidationTimeout,
	)

	addr := ":" + cfg.Port
	log.Printf("==================== PulseOps backend ready, listening on %s ====================", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		fmt.Fprintf(os.Stderr, "backend server error: %v\n", err)
		os.Exit(1)
	}
}
