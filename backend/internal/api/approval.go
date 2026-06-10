package api

import (
	"errors"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// MaxApprovalNoteLength bounds the optional operator note (task 4.3.4). The note
// is for human context/auditability, not machine input, so a generous-but-finite
// cap keeps payloads sane without constraining the operator.
const MaxApprovalNoteLength = 500

// Approval contract errors. These are structural/contract violations surfaced to
// the client as 4xx by the approval endpoint (Phase 8 task 4.4).
var (
	ErrApprovalMissingApprover           = errors.New("approvedBy is required")
	ErrApprovalNoActions                 = errors.New("selectedActionIds must contain at least one action")
	ErrApprovalBlankActionID             = errors.New("selectedActionIds contains an empty action id")
	ErrApprovalDuplicateAction           = errors.New("selectedActionIds contains duplicate action ids")
	ErrApprovalNoteTooLong               = errors.New("note exceeds maximum length")
	ErrApprovalActionNotInRecommendation = errors.New("selected action id is not part of the incident recommendation")
)

// ApprovalRequest is the payload an operator submits to approve a recommendation.
//
//	{
//	  "approvedBy": "demo.operator",
//	  "selectedActionIds": ["restart_service"],
//	  "note": "Approved after reviewing recommendation"
//	}
type ApprovalRequest struct {
	ApprovedBy        string   `json:"approvedBy"`
	SelectedActionIDs []string `json:"selectedActionIds"`
	Note              string   `json:"note,omitempty"`
}

// QueuedAction is a single action prepared for Phase 9 execution. Its target is
// derived from the trusted backend recommendation snapshot, never from client input.
type QueuedAction struct {
	ActionID string `json:"actionId"`
	Target   string `json:"target,omitempty"`
}

// ApprovalResponse is returned once an incident has been approved.
//
//	{
//	  "incidentId": "INC-1001",
//	  "state": "approved",
//	  "approvedBy": "demo.operator",
//	  "approvedAt": "2026-05-23T22:10:00Z",
//	  "queuedActions": [{ "actionId": "restart_service", "target": "OpenVPNService" }]
//	}
type ApprovalResponse struct {
	IncidentID    string         `json:"incidentId"`
	State         string         `json:"state"`
	ApprovedBy    string         `json:"approvedBy"`
	ApprovedAt    time.Time      `json:"approvedAt"`
	QueuedActions []QueuedAction `json:"queuedActions"`
}

// Normalize trims surrounding whitespace on request string fields so validation
// and downstream comparisons operate on canonical values.
func (r *ApprovalRequest) Normalize() {
	r.ApprovedBy = strings.TrimSpace(r.ApprovedBy)
	r.Note = strings.TrimSpace(r.Note)
	for i := range r.SelectedActionIDs {
		r.SelectedActionIDs[i] = strings.TrimSpace(r.SelectedActionIDs[i])
	}
}

// Validate performs structural validation that does not depend on a specific
// incident: approver presence (task 4.3.2), at least one well-formed action id,
// no duplicates, and a bounded note (task 4.3.4). Cross-checking the selected
// ids against the incident's recommendation is done by QueuedActionsFor
// (task 4.3.3) once the incident is known.
//
// Call Normalize first.
func (r ApprovalRequest) Validate() error {
	if r.ApprovedBy == "" {
		return ErrApprovalMissingApprover
	}
	if len(r.SelectedActionIDs) == 0 {
		return ErrApprovalNoActions
	}

	seen := make(map[string]struct{}, len(r.SelectedActionIDs))
	for _, id := range r.SelectedActionIDs {
		if id == "" {
			return ErrApprovalBlankActionID
		}
		if _, dup := seen[id]; dup {
			return ErrApprovalDuplicateAction
		}
		seen[id] = struct{}{}
	}

	if len([]rune(r.Note)) > MaxApprovalNoteLength {
		return ErrApprovalNoteTooLong
	}

	return nil
}

// QueuedActionsFor validates the selected action ids against the incident's
// recommendation snapshot (task 4.3.3) and returns the queued actions with target
// data pulled from that trusted snapshot. It returns ErrApprovalActionNotInRecommendation
// if any selected id was not recommended for this incident.
//
// This mirrors the validation enforced inside the store's Approve method, giving
// the HTTP layer a clean way to reject mismatches with a 4xx before mutating state.
func QueuedActionsFor(recommended []incidents.RecommendedAction, selectedIDs []string) ([]QueuedAction, error) {
	byID := make(map[string]incidents.RecommendedAction, len(recommended))
	for _, rec := range recommended {
		byID[rec.ActionID] = rec
	}

	queued := make([]QueuedAction, 0, len(selectedIDs))
	for _, id := range selectedIDs {
		rec, ok := byID[id]
		if !ok {
			return nil, ErrApprovalActionNotInRecommendation
		}
		queued = append(queued, QueuedAction{ActionID: rec.ActionID, Target: rec.Target})
	}
	return queued, nil
}

// NewApprovalResponse builds the response DTO from an already-approved incident.
// Queued actions are derived from the incident's approved action set and the
// trusted recommendation snapshot, so targets always come from backend data.
func NewApprovalResponse(inc incidents.Incident) ApprovalResponse {
	// The store guarantees ApprovedActions is a subset of RecommendedActions, so
	// the lookup cannot fail here. The error is intentionally ignored.
	queued, _ := QueuedActionsFor(inc.RecommendedActions, inc.ApprovedActions)

	var approvedAt time.Time
	if inc.ApprovedAt != nil {
		approvedAt = *inc.ApprovedAt
	}

	return ApprovalResponse{
		IncidentID:    inc.IncidentID,
		State:         string(inc.State),
		ApprovedBy:    inc.ApprovedBy,
		ApprovedAt:    approvedAt,
		QueuedActions: queued,
	}
}
