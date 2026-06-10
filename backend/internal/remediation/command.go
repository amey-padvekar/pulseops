package remediation

import (
	"errors"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// CommandStatus marks where a queued remediation command sits on its way to Phase 9
// execution. Phase 8 only ever produces queued commands; the later statuses are
// defined here so Phase 9 can advance them without redefining the contract.
type CommandStatus string

const (
	// StatusQueued means the command has been approved and is waiting to be picked
	// up. This is the only status Phase 8 assigns.
	StatusQueued CommandStatus = "queued"
	// StatusPendingDispatch means Phase 9 has claimed the command and is about to
	// deliver it to the endpoint.
	StatusPendingDispatch CommandStatus = "pending_dispatch"
	// StatusDispatched means the command has been handed to the endpoint.
	StatusDispatched CommandStatus = "dispatched"
	// StatusAcknowledged means the agent confirmed receipt of the dispatched command.
	// Acknowledgement is optional in the MVP and is used for observability only; it does
	// not affect repeat-dispatch prevention.
	StatusAcknowledged CommandStatus = "acknowledged"
)

// Action is a single safe remediation step prepared for execution. Both fields are
// derived from the trusted recommendation snapshot, never from freeform client input.
type Action struct {
	ActionID string `json:"actionId"`
	Target   string `json:"target,omitempty"`
}

// Command is the queued remediation payload produced once an incident is approved.
// It carries everything Phase 9 needs to act without reconstructing intent: which
// incident/device, who approved it and when, and the exact safe actions to run.
type Command struct {
	IncidentID string        `json:"incidentId"`
	DeviceID   string        `json:"deviceId"`
	ApprovedBy string        `json:"approvedBy"`
	ApprovedAt time.Time     `json:"approvedAt"`
	Actions    []Action      `json:"actions"`
	Status     CommandStatus `json:"status"`

	// Dispatch lifecycle metadata (Phase 9 step 4.3). These are zero until the command
	// is claimed for dispatch and are owned by the Queue, not by NewCommand.
	RequestID     string    `json:"requestId,omitempty"`
	DispatchedAt  time.Time `json:"dispatchedAt,omitempty"`
	DispatchCount int       `json:"dispatchCount,omitempty"`
}

// Command construction errors.
var (
	ErrNotApproved               = errors.New("incident is not approved")
	ErrNoApprovedActions         = errors.New("incident has no approved actions")
	ErrActionNotInRecommendation = errors.New("approved action is not present in the recommendation snapshot")
	ErrActionNotInCatalog        = errors.New("approved action is not in the backend remediation catalog")
)

// NewCommand builds a queued remediation command from an approved incident.
//
// It is intentionally strict: the incident must be in the approved state with at
// least one approved action, and every approved action must resolve against the
// incident's recommendation snapshot. Targets are pulled from that snapshot, so the
// command can never contain action data the AI did not originally recommend
// (task 4.5.3). The resulting command is always StatusQueued (task 4.5.5).
func NewCommand(inc incidents.Incident) (Command, error) {
	if inc.State != incidents.StateApproved {
		return Command{}, ErrNotApproved
	}
	if len(inc.ApprovedActions) == 0 {
		return Command{}, ErrNoApprovedActions
	}

	byID := make(map[string]incidents.RecommendedAction, len(inc.RecommendedActions))
	for _, rec := range inc.RecommendedActions {
		byID[rec.ActionID] = rec
	}

	actions := make([]Action, 0, len(inc.ApprovedActions))
	for _, id := range inc.ApprovedActions {
		rec, ok := byID[id]
		if !ok {
			return Command{}, ErrActionNotInRecommendation
		}
		// Final safety gate: the executable payload may only ever contain catalog
		// actions, even if an upstream snapshot were somehow poisoned.
		if !IsApprovedAction(rec.ActionID) {
			return Command{}, ErrActionNotInCatalog
		}
		actions = append(actions, Action{ActionID: rec.ActionID, Target: rec.Target})
	}

	var approvedAt time.Time
	if inc.ApprovedAt != nil {
		approvedAt = *inc.ApprovedAt
	}

	return Command{
		IncidentID: inc.IncidentID,
		DeviceID:   inc.DeviceID,
		ApprovedBy: inc.ApprovedBy,
		ApprovedAt: approvedAt,
		Actions:    actions,
		Status:     StatusQueued,
	}, nil
}
