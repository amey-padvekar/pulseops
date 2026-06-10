package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/remediation"
)

// PendingCommandsResponse is the body a polling agent receives. Commands is always a
// (possibly empty) array so the agent can decode a single stable shape.
type PendingCommandsResponse struct {
	DeviceID string                        `json:"deviceId"`
	Commands []remediation.DispatchCommand `json:"commands"`
}

// PendingCommandsHandler returns an http.HandlerFunc for
// GET /devices/{deviceId}/commands.
//
// This is the Phase 9 dispatch mechanism (step 4.3). The endpoint agent already polls
// the backend over HTTP for its heartbeat, so a device-scoped command-fetch endpoint
// is the simplest delivery path that fits the existing architecture and stays
// observable for the demo.
//
// Fetching is the dispatch act: each returned command transitions queued -> dispatched
// and is stamped with a fresh requestId, so a command is delivered exactly once unless
// retry is explicitly armed (Queue.RequeueForRetry). Only commands built from approved
// incidents ever reach the queue, so this endpoint only dispatches approved work.
//
// When a command is dispatched, the incident is moved to executing (Phase 9 task
// 4.7.3): dispatch is when execution begins. store and onIncidentUpdated may be nil.
func PendingCommandsHandler(q *remediation.Queue, store *incidents.Store, onIncidentUpdated func(incidents.Incident)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		deviceID := strings.TrimSpace(r.PathValue("deviceId"))
		if deviceID == "" {
			writeJSONError(w, http.StatusNotFound, "device not found")
			return
		}

		commands := q.ClaimPendingForDevice(deviceID, time.Now().UTC(), newDispatchRequestID)
		if commands == nil {
			commands = []remediation.DispatchCommand{}
		}

		for _, c := range commands {
			log.Printf(
				"remediation command dispatched incident_id=%s device_id=%s request_id=%s actions=%d",
				c.IncidentID, c.DeviceID, c.RequestID, len(c.Actions),
			)
			if store == nil {
				continue
			}
			updated, changed, err := store.MarkExecuting(c.IncidentID, c.RequestID)
			if err != nil {
				log.Printf("failed to mark incident executing incident_id=%s error=%v", c.IncidentID, err)
				continue
			}
			if changed && onIncidentUpdated != nil {
				onIncidentUpdated(updated)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(PendingCommandsResponse{DeviceID: deviceID, Commands: commands})
	}
}

// RemediationResultHandler returns an http.HandlerFunc for POST /remediation/results.
//
// It is the Phase 9 result ingestion endpoint (step 4.7): the agent posts an
// ExecutionResult here after attempting a command. The handler validates the payload,
// confirms the incident exists, that the device matches the incident, and that the
// requestId matches the command the backend actually dispatched, then persists the
// outcome and advances the incident lifecycle to its post-result boundary:
//   - succeeded -> validating (Phase 10 decides whether health was restored; Phase 9
//     never resolves an incident on command success alone)
//   - anything else -> failed
//
// On success the queued command is acknowledged and the updated incident is handed to
// onIncidentUpdated for broadcast. store and queue are required; onIncidentUpdated may
// be nil.
func RemediationResultHandler(store *incidents.Store, queue *remediation.Queue, onIncidentUpdated func(incidents.Incident)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var result remediation.ExecutionResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Shape validation.
		result.IncidentID = strings.TrimSpace(result.IncidentID)
		result.DeviceID = strings.TrimSpace(result.DeviceID)
		result.RequestID = strings.TrimSpace(result.RequestID)
		if result.IncidentID == "" || result.DeviceID == "" || result.RequestID == "" {
			writeJSONError(w, http.StatusBadRequest, "incidentId, deviceId, and requestId are required")
			return
		}
		if !result.Status.IsValid() {
			writeJSONError(w, http.StatusBadRequest, "invalid execution status")
			return
		}

		// Incident must exist.
		inc, found := store.GetByID(result.IncidentID)
		if !found {
			writeJSONError(w, http.StatusNotFound, "incident not found")
			return
		}

		// Device must match the incident.
		if result.DeviceID != inc.DeviceID {
			writeJSONError(w, http.StatusConflict, "device does not match incident")
			return
		}

		// requestId must match the command the backend dispatched for this incident.
		cmd, ok := queue.GetByIncidentID(result.IncidentID)
		if !ok || cmd.RequestID == "" || cmd.RequestID != result.RequestID {
			writeJSONError(w, http.StatusConflict, "requestId does not match a dispatched command")
			return
		}

		// Duplicate-result safeguard (Phase 9 task 4.10): once a result for this command
		// has been ingested, the command is acknowledged. A second POST for the same
		// requestId is a duplicate delivery — log it and respond idempotently with the
		// already-stored incident rather than re-persisting and re-appending timeline.
		if cmd.Status == remediation.StatusAcknowledged {
			log.Printf(
				"remediation duplicate result ignored incident_id=%s device_id=%s request_id=%s reason=already_acknowledged",
				inc.IncidentID, inc.DeviceID, result.RequestID,
			)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(inc)
			return
		}

		// Bound logs and normalize timestamps defensively before persisting.
		normalized := result.Normalize()

		updated, err := store.SaveRemediationResult(
			result.IncidentID,
			executionOutcomeFrom(normalized),
			time.Now().UTC(),
			postResultState(normalized.Status),
		)
		if err != nil {
			if err == incidents.ErrIncidentNotFound {
				writeJSONError(w, http.StatusNotFound, "incident not found")
				return
			}
			writeJSONError(w, http.StatusConflict, err.Error())
			return
		}

		// Acknowledge the command so its lifecycle reflects completion (best effort).
		queue.MarkAcknowledged(result.IncidentID)

		log.Printf(
			"remediation result ingested incident_id=%s device_id=%s request_id=%s status=%s next_state=%s actions=%d",
			updated.IncidentID, updated.DeviceID, normalized.RequestID, normalized.Status, updated.State, len(normalized.Results),
		)

		if onIncidentUpdated != nil {
			onIncidentUpdated(updated)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(updated)
	}
}

// postResultState maps an overall execution status to the incident lifecycle state the
// incident should move to after the result is ingested.
func postResultState(status remediation.ExecutionStatus) incidents.IncidentState {
	if status == remediation.ExecStatusSucceeded {
		return incidents.StateValidating
	}
	return incidents.StateFailed
}

// executionOutcomeFrom maps the wire ExecutionResult into the incident-local outcome
// the store persists.
func executionOutcomeFrom(res remediation.ExecutionResult) incidents.ExecutionOutcome {
	actions := make([]incidents.RemediationActionResult, 0, len(res.Results))
	for _, a := range res.Results {
		actions = append(actions, incidents.RemediationActionResult{
			ActionID:   a.ActionID,
			Target:     a.Target,
			Status:     string(a.Status),
			Stdout:     a.Stdout,
			Stderr:     a.Stderr,
			ExitCode:   a.ExitCode,
			DurationMs: a.DurationMs,
		})
	}
	return incidents.ExecutionOutcome{
		RequestID:  res.RequestID,
		Status:     string(res.Status),
		StartedAt:  res.StartedAt,
		FinishedAt: res.FinishedAt,
		Actions:    actions,
	}
}

// newDispatchRequestID returns a short random correlation id for one dispatch attempt.
// On the vanishingly unlikely RNG failure it falls back to a static marker rather than
// failing the dispatch outright.
func newDispatchRequestID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "rem-unknown"
	}
	return "rem-" + hex.EncodeToString(buf)
}
