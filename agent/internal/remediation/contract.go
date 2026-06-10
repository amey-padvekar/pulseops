package remediation

import "time"

// Command is the remediation command an endpoint agent receives from the backend.
//
// It mirrors the backend's wire contract (backend/internal/remediation.DispatchCommand)
// field for field, including JSON tags, so the agent can deserialize exactly what the
// backend dispatches. The two definitions are intentionally separate Go types because
// the backend and agent are independent modules; the canonical shape lives at
// docs/contracts/remediation_command_fixture.json, and both sides must move together.
//
// The payload is action-ID-based only. It never carries raw shell commands: each
// Action.ActionID is a backend catalog action that the agent maps to a platform
// specific implementation in later Phase 9 steps. RequestID and DispatchedAt are
// carried purely for correlation so a dispatch can be traced across backend and agent
// logs.
type Command struct {
	IncidentID   string    `json:"incidentId"`
	DeviceID     string    `json:"deviceId"`
	ApprovedBy   string    `json:"approvedBy"`
	ApprovedAt   time.Time `json:"approvedAt"`
	Actions      []Action  `json:"actions"`
	DispatchedAt time.Time `json:"dispatchedAt"`
	RequestID    string    `json:"requestId"`
}

// Action is a single approved remediation step. Both fields are derived upstream from
// the trusted recommendation snapshot; the agent treats ActionID as an opaque catalog
// key to look up, never as a command to run literally.
type Action struct {
	ActionID string `json:"actionId"`
	Target   string `json:"target,omitempty"`
}
