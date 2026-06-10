package incidents

import "time"

// RemediationActionResult is the incident-local record of one executed remediation
// action's outcome (Phase 9). It mirrors the agent/backend execution-result contract
// but lives in the incidents package so the store never imports the remediation
// package (which already imports incidents — importing back would be a cycle).
type RemediationActionResult struct {
	ActionID   string `json:"actionId"`
	Target     string `json:"target,omitempty"`
	Status     string `json:"status"`
	Stdout     string `json:"stdout,omitempty"`
	Stderr     string `json:"stderr,omitempty"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
}

// ExecutionOutcome is the incident-local remediation execution result the store
// persists. It bundles the overall status, correlation id, timing, and per-action
// detail reported by the agent.
type ExecutionOutcome struct {
	RequestID  string
	Status     string
	StartedAt  time.Time
	FinishedAt time.Time
	Actions    []RemediationActionResult
}
