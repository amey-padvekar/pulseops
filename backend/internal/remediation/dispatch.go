package remediation

import "time"

// DispatchCommand is the canonical on-the-wire remediation command the backend
// sends to an endpoint agent. It is the Phase 9 execution contract shared with the
// agent module (mirrored by agent/internal/remediation): the agent deserializes this
// exact shape.
//
// It differs from the internal queued Command in two ways:
//   - it omits backend-only queue state (Status), which the agent never needs; and
//   - it adds dispatch-time correlation fields (DispatchedAt, RequestID) so a single
//     dispatch attempt is traceable end to end across backend logs and agent logs.
//
// The payload is intentionally bounded and action-ID-based only. It never carries
// raw shell commands: every Action.ActionID is a backend catalog action (enforced
// upstream by NewCommand), and the agent maps those IDs to platform-specific
// implementations rather than executing anything literal from this payload.
type DispatchCommand struct {
	IncidentID   string    `json:"incidentId"`
	DeviceID     string    `json:"deviceId"`
	ApprovedBy   string    `json:"approvedBy"`
	ApprovedAt   time.Time `json:"approvedAt"`
	Actions      []Action  `json:"actions"`
	DispatchedAt time.Time `json:"dispatchedAt"`
	RequestID    string    `json:"requestId"`
}

// NewDispatchCommand builds the wire-format dispatch payload from a queued command.
//
// requestID is a fresh correlation id for this dispatch attempt and dispatchedAt
// stamps when the backend handed the command off; both are supplied by the caller so
// this stays deterministic and easy to test. The actions slice is copied so the
// dispatch payload is independent of the queued command's backing array. Approval
// metadata (ApprovedBy/ApprovedAt) is carried through verbatim for traceability.
func NewDispatchCommand(cmd Command, requestID string, dispatchedAt time.Time) DispatchCommand {
	actions := make([]Action, len(cmd.Actions))
	copy(actions, cmd.Actions)
	return DispatchCommand{
		IncidentID:   cmd.IncidentID,
		DeviceID:     cmd.DeviceID,
		ApprovedBy:   cmd.ApprovedBy,
		ApprovedAt:   cmd.ApprovedAt,
		Actions:      actions,
		DispatchedAt: dispatchedAt,
		RequestID:    requestID,
	}
}
