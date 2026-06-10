package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/agentbuilder"
	"github.com/certainelf/pulseops/backend/internal/demo"
	"github.com/certainelf/pulseops/backend/internal/elastic"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

// demoPacing controls how the self-driving lifecycle is spaced so a judge can watch each
// transition animate. Production uses visible delays; tests inject tiny values.
type demoPacing struct {
	stagePause      time.Duration // delay between visible lifecycle transitions
	pollInterval    time.Duration // poll cadence while waiting for a state change
	investigateWait time.Duration // max wait for awaiting_approval after creation
	approvalWait    time.Duration // max wait for the judge to approve (manual mode)
}

func defaultDemoPacing() demoPacing {
	return demoPacing{
		stagePause:      1500 * time.Millisecond,
		pollInterval:    500 * time.Millisecond,
		investigateWait: 90 * time.Second,
		approvalWait:    5 * time.Minute,
	}
}

// demoController owns the DEMO_MODE "Simulate Service Failure" endpoints and the goroutine
// that self-drives a generated incident to resolved. It holds the same dependencies the real
// telemetry path uses so the demo runs the identical pipeline (real detection + Gemini +
// Elastic MCP), simulating only the failure source and the remediation execution.
type demoController struct {
	deviceStore   *store.DeviceStore
	incidentStore *incidents.Store
	hub           *ws.Hub
	elasticClient elastic.Indexer
	elasticCfg    *elastic.Config
	agentClient   agentbuilder.Client
	agentCfg      *agentbuilder.Config

	pacing demoPacing
}

func newDemoController(
	deviceStore *store.DeviceStore,
	incidentStore *incidents.Store,
	hub *ws.Hub,
	elasticClient elastic.Indexer,
	elasticCfg *elastic.Config,
	agentClient agentbuilder.Client,
	agentCfg *agentbuilder.Config,
) *demoController {
	return &demoController{
		deviceStore:   deviceStore,
		incidentStore: incidentStore,
		hub:           hub,
		elasticClient: elasticClient,
		elasticCfg:    elasticCfg,
		agentClient:   agentClient,
		agentCfg:      agentCfg,
		pacing:        defaultDemoPacing(),
	}
}

// registerDemoRoutes wires the DEMO_MODE endpoints onto mux, but only when enabled, so the
// /demo/* surface simply does not exist in a normal deployment. Returns true when the routes
// were registered (for startup logging).
func registerDemoRoutes(
	mux *http.ServeMux,
	enabled bool,
	deviceStore *store.DeviceStore,
	incidentStore *incidents.Store,
	hub *ws.Hub,
	elasticClient elastic.Indexer,
	elasticCfg *elastic.Config,
	agentClient agentbuilder.Client,
	agentCfg *agentbuilder.Config,
) bool {
	if !enabled {
		return false
	}
	c := newDemoController(deviceStore, incidentStore, hub, elasticClient, elasticCfg, agentClient, agentCfg)
	mux.HandleFunc("POST /demo/incident", c.incidentHandler())
	mux.HandleFunc("POST /demo/reset", c.resetHandler())
	return true
}

// demoIncidentRequest is the body of POST /demo/incident.
//
//   - DeviceID    — the dashboard's currently-selected device (required).
//   - Scenario    — a Part A catalog key (demo.ByKey); defaults to demo.Default() when blank/unknown.
//   - ServiceName — overrides the scenario's default service short name, so the dropdown can
//     target the exact service installed on the box (e.g. "MySQL" vs "MySQL80").
//   - AutoApprove — lets the lifecycle approve itself instead of waiting for the judge to click Approve.
type demoIncidentRequest struct {
	DeviceID    string `json:"deviceId"`
	ServiceName string `json:"serviceName"`
	Scenario    string `json:"scenario"`
	AutoApprove bool   `json:"autoApprove"`
}

// incidentHandler builds the POST /demo/incident handler. It synthesizes a STOPPED telemetry
// sample for the selected device from the Part A scenario catalog and runs it through the SAME
// pipeline as real telemetry (ingestTelemetrySample): real detection, real Gemini + Elastic MCP
// investigation, and live broadcasts. It returns the opened incident's ID and spawns the
// self-driving lifecycle (B4). Registered only when DEMO_MODE is true.
func (c *demoController) incidentHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req demoIncidentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid demo incident payload", http.StatusBadRequest)
			return
		}

		deviceID := strings.TrimSpace(req.DeviceID)
		if deviceID == "" {
			http.Error(w, "deviceId is required", http.StatusBadRequest)
			return
		}

		// Resolve the scenario from the Part A catalog (default when blank/unknown). An
		// explicit serviceName overrides the scenario's default short name.
		scenario, ok := demo.ByKey(strings.TrimSpace(req.Scenario))
		if !ok {
			scenario = demo.Default()
		}
		serviceName := strings.TrimSpace(req.ServiceName)
		if serviceName == "" {
			serviceName = scenario.ServiceName
		}

		// Synthetic STOPPED sample — the detector trigger is stopped + heartbeat
		// (incidents.EvaluateTelemetry). NetworkReachable stays true so the failure reads as
		// a service crash, not a connectivity loss.
		now := time.Now().UTC()
		synthetic := store.DeviceState{
			DeviceID:         deviceID,
			Timestamp:        now.Format(time.RFC3339),
			ServiceName:      serviceName,
			ServiceStatus:    "stopped",
			NetworkReachable: true,
			Heartbeat:        true,
			RecentLogs:       scenario.RecentLogs(now),
		}

		state, ingested := ingestTelemetrySample(
			synthetic,
			c.deviceStore,
			c.incidentStore,
			c.hub,
			c.elasticClient,
			c.elasticCfg,
			c.agentClient,
			c.agentCfg,
		)
		if !ingested {
			http.Error(w, "failed to ingest synthetic telemetry", http.StatusInternalServerError)
			return
		}

		incident, found := activeIncidentForService(c.incidentStore, state.DeviceID, serviceName)
		if !found {
			http.Error(w, "synthetic telemetry did not open an incident", http.StatusInternalServerError)
			return
		}

		log.Printf(
			"🧪 demo incident created incident_id=%s device_id=%s service=%s scenario=%s mode=%s auto_approve=%t state=%s",
			incident.IncidentID, incident.DeviceID, serviceName, scenario.Key, scenario.Mode, req.AutoApprove, incident.State,
		)

		// Self-drive the incident to resolved (waits for the real investigation, then the
		// approval gate, then simulates execution + recovery) with paced, broadcast transitions.
		go c.runLifecycle(incident.IncidentID, req.AutoApprove)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"incidentId":  incident.IncidentID,
			"deviceId":    incident.DeviceID,
			"serviceName": serviceName,
			"scenario":    scenario.Key,
			"state":       string(incident.State),
		})
	}
}

// demoResetRequest is the body of POST /demo/reset.
type demoResetRequest struct {
	DeviceID string `json:"deviceId"`
}

// resetHandler builds the POST /demo/reset handler. It hard-deletes every incident for the
// given device (clearing the dedupe mappings) so a judge can re-run a scenario from a clean
// slate. Registered only when DEMO_MODE is true.
func (c *demoController) resetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req demoResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid demo reset payload", http.StatusBadRequest)
			return
		}

		deviceID := strings.TrimSpace(req.DeviceID)
		if deviceID == "" {
			http.Error(w, "deviceId is required", http.StatusBadRequest)
			return
		}

		// Delete every incident for the device (active and historical) so the dashboard list
		// and the dedupe map are clean for the next run. List returns a snapshot, so deleting
		// while ranging over it is safe.
		deleted := make([]string, 0)
		for _, inc := range c.incidentStore.List(incidents.IncidentFilter{DeviceID: deviceID}) {
			if c.incidentStore.Delete(inc.IncidentID) {
				deleted = append(deleted, inc.IncidentID)
			}
		}

		log.Printf("🧪 demo reset device_id=%s deleted_incidents=%d", deviceID, len(deleted))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"deviceId":    deviceID,
			"deleted":     len(deleted),
			"incidentIds": deleted,
		})
	}
}

// runLifecycle advances a generated incident from awaiting_approval to resolved with paced,
// broadcast transitions a judge can watch. The diagnosis is real (the agent already ran during
// creation); execution and recovery are simulated server-side so the incident always resolves
// with no endpoint agent. Intended to run in its own goroutine.
func (c *demoController) runLifecycle(incidentID string, autoApprove bool) {
	// 1. Wait for the real investigation to land (awaiting_approval).
	inc, ok := c.waitForState(incidentID, incidents.StateAwaitingApproval, c.pacing.investigateWait)
	if !ok {
		log.Printf("🧪 demo lifecycle: incident %s did not reach awaiting_approval within %s (need a real agent or AGENT_BUILDER_FALLBACK_MODE=local_stub_actions); leaving as-is", incidentID, c.pacing.investigateWait)
		return
	}

	// 2. Approval — the governance gate, kept real.
	if autoApprove {
		approved, err := c.incidentStore.Approve(incidentID, "demo-auto-approve", recommendedActionIDs(inc), "auto-approved by demo panel", time.Now().UTC())
		if err != nil {
			log.Printf("🧪 demo lifecycle: auto-approve failed incident=%s error=%v", incidentID, err)
			return
		}
		publishIncidentUpdate(c.hub, c.elasticClient, approved)
		inc = approved
	} else {
		// Default: wait for the judge to click Approve in the existing UI.
		inc, ok = c.waitForState(incidentID, incidents.StateApproved, c.pacing.approvalWait)
		if !ok {
			log.Printf("🧪 demo lifecycle: incident %s not approved within %s; leaving as-is", incidentID, c.pacing.approvalWait)
			return
		}
	}

	time.Sleep(c.pacing.stagePause)

	// 3. Execute (simulated): approved -> executing.
	requestID := "demo-" + incidentID
	if executing, changed, err := c.incidentStore.MarkExecuting(incidentID, requestID); err != nil {
		log.Printf("🧪 demo lifecycle: mark executing failed incident=%s error=%v", incidentID, err)
		return
	} else if changed {
		publishIncidentUpdate(c.hub, c.elasticClient, executing)
	}

	time.Sleep(c.pacing.stagePause)

	// 4. Remediation result (simulated success): executing -> validating.
	finished := time.Now().UTC()
	exit := 0
	outcome := incidents.ExecutionOutcome{
		RequestID:  requestID,
		Status:     "succeeded", // incidents stores this verbatim as RemediationStatus
		StartedAt:  finished.Add(-1500 * time.Millisecond),
		FinishedAt: finished,
		Actions: []incidents.RemediationActionResult{{
			ActionID:   "restart_service",
			Target:     inc.ServiceName,
			Status:     "succeeded",
			Stdout:     "Restart-Service -Name " + inc.ServiceName + " -Force   # simulated by demo panel",
			ExitCode:   &exit,
			DurationMs: 1500,
		}},
	}
	// Stamp the post-remediation freshness boundary slightly in the past so the healthy samples
	// we feed next are strictly newer than it (validation admits only strictly-newer telemetry).
	validating, err := c.incidentStore.SaveRemediationResult(incidentID, outcome, finished.Add(-2*time.Second), incidents.StateValidating)
	if err != nil {
		log.Printf("🧪 demo lifecycle: save remediation result failed incident=%s error=%v", incidentID, err)
		return
	}
	publishIncidentUpdate(c.hub, c.elasticClient, validating)

	// 5. Validate & resolve: feed healthy "running" samples through the REAL validation path
	// (ingestTelemetrySample -> processTelemetryValidation), which advances the healthy-cycle
	// counter and, on the final cycle, resolves the incident + fires the closing summary.
	required := validating.RequiredHealthyCycles
	if required <= 0 {
		required = incidents.DefaultRequiredHealthyCycles
	}
	for i := 0; i < required; i++ {
		time.Sleep(c.pacing.stagePause)
		c.ingestHealthySample(inc.DeviceID, inc.ServiceName)
	}

	if final, ok := c.incidentStore.GetByID(incidentID); ok {
		log.Printf("🧪 demo lifecycle complete incident=%s device_id=%s final_state=%s", incidentID, final.DeviceID, final.State)
	}
}

// ingestHealthySample drives one healthy "running" telemetry sample for the device through the
// shared pipeline, providing recovery evidence to any validating incident.
func (c *demoController) ingestHealthySample(deviceID, serviceName string) {
	now := time.Now().UTC()
	ingestTelemetrySample(
		store.DeviceState{
			DeviceID:         deviceID,
			Timestamp:        now.Format(time.RFC3339),
			ServiceName:      serviceName,
			ServiceStatus:    "running",
			NetworkReachable: true,
			Heartbeat:        true,
			RecentLogs: []string{
				fmt.Sprintf("%s|Service Control Manager|7036|Information|The %s service entered the running state.", now.Format(time.RFC3339Nano), serviceName),
			},
		},
		c.deviceStore,
		c.incidentStore,
		c.hub,
		c.elasticClient,
		c.elasticCfg,
		c.agentClient,
		c.agentCfg,
	)
}

// waitForState polls until the incident reaches target, hits a non-target terminal state, the
// incident disappears (e.g. demo reset), or timeout elapses. The bool is true only when target
// was reached.
func (c *demoController) waitForState(incidentID string, target incidents.IncidentState, timeout time.Duration) (incidents.Incident, bool) {
	deadline := time.Now().Add(timeout)
	for {
		inc, ok := c.incidentStore.GetByID(incidentID)
		if !ok {
			return incidents.Incident{}, false
		}
		if inc.State == target {
			return inc, true
		}
		if inc.State == incidents.StateResolved || inc.State == incidents.StateFailed {
			return inc, false
		}
		if time.Now().After(deadline) {
			return inc, false
		}
		time.Sleep(c.pacing.pollInterval)
	}
}

// activeIncidentForService returns the active incident for the given device + service, if any.
// Detection dedups one active incident per device|service|signature, so at most one matches.
func activeIncidentForService(incidentStore *incidents.Store, deviceID, serviceName string) (incidents.Incident, bool) {
	active := true
	for _, inc := range incidentStore.List(incidents.IncidentFilter{DeviceID: deviceID, Active: &active}) {
		if inc.ServiceName == serviceName {
			return inc, true
		}
	}
	return incidents.Incident{}, false
}

// recommendedActionIDs collects the action IDs from an incident's AI recommendation, used to
// auto-approve every recommended action in the hands-off demo path.
func recommendedActionIDs(inc incidents.Incident) []string {
	ids := make([]string, 0, len(inc.RecommendedActions))
	for _, a := range inc.RecommendedActions {
		ids = append(ids, a.ActionID)
	}
	return ids
}
