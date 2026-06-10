package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/remediation"
)

// IncidentApprovalHandler returns an http.HandlerFunc for
// POST /incidents/{incidentId}/approve.
//
// It is the single deterministic write path for human approval (Phase 8 task 4.4):
// it validates the request and incident state, records approval via the store, and
// returns the approval response DTO. When provided, onApproved is invoked with the
// approved incident so callers can broadcast the update (e.g. over websocket).
func IncidentApprovalHandler(s *incidents.Store, onApproved func(incidents.Incident)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		incidentID := strings.TrimSpace(r.PathValue("incidentId"))
		if incidentID == "" {
			writeJSONError(w, http.StatusNotFound, "incident not found")
			return
		}

		var req ApprovalRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		req.Normalize()

		// Structural contract validation (approver present, well-formed actions, bounded note).
		if err := req.Validate(); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		inc, found := s.GetByID(incidentID)
		if !found {
			writeJSONError(w, http.StatusNotFound, "incident not found")
			return
		}

		// An investigation result with a concrete recommendation must exist before
		// an operator can approve anything (task 4.4.2). Promotion to awaiting_approval
		// already guarantees this, but checking here yields a clear error for incidents
		// approached out of order.
		if len(inc.RecommendedActions) == 0 {
			writeJSONError(w, http.StatusConflict, "incident has no recommendation to approve")
			return
		}

		// Cross-check selected ids against the recommendation snapshot for a clean 4xx
		// before mutating state. The store re-validates this as defense in depth.
		if _, err := QueuedActionsFor(inc.RecommendedActions, req.SelectedActionIDs); err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		// Independent safety gate: every selected action must be in the backend
		// remediation catalog, not merely present in the recommendation. This rejects
		// a poisoned recommendation before any state mutation (action-ID-only model).
		for _, id := range req.SelectedActionIDs {
			if !remediation.IsApprovedAction(id) {
				writeJSONError(w, http.StatusBadRequest, "selected action is not in the remediation catalog: "+id)
				return
			}
		}

		approved, err := s.Approve(incidentID, req.ApprovedBy, req.SelectedActionIDs, req.Note, time.Now().UTC())
		if err != nil {
			switch {
			case errors.Is(err, incidents.ErrIncidentNotFound):
				writeJSONError(w, http.StatusNotFound, "incident not found")
			case errors.Is(err, incidents.ErrAlreadyApproved):
				writeJSONError(w, http.StatusConflict, "incident already approved")
			case errors.Is(err, incidents.ErrInvalidTransition):
				writeJSONError(w, http.StatusConflict, "incident is not awaiting approval")
			default:
				writeJSONError(w, http.StatusBadRequest, err.Error())
			}
			return
		}

		if onApproved != nil {
			onApproved(approved)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(NewApprovalResponse(approved))
	}
}

// writeJSONError writes a JSON {"error": message} body with the given status code.
func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
