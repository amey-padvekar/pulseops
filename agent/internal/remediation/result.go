package remediation

import (
	"time"
	"unicode/utf8"
)

// ExecutionStatus is the normalized status vocabulary the agent uses when reporting
// remediation outcomes. It mirrors the backend's vocabulary
// (backend/internal/remediation.ExecutionStatus) string for string so the backend can
// store and render results without translation.
type ExecutionStatus string

const (
	ExecStatusQueued     ExecutionStatus = "queued"
	ExecStatusDispatched ExecutionStatus = "dispatched"
	ExecStatusRunning    ExecutionStatus = "running"
	ExecStatusSucceeded  ExecutionStatus = "succeeded"
	ExecStatusFailed     ExecutionStatus = "failed"
	ExecStatusRejected   ExecutionStatus = "rejected"
)

// MaxLogBytes bounds how much stdout/stderr a single action result may carry. The
// agent caps each stream so it never ships an unbounded blob back to the backend.
const MaxLogBytes = 4096

const logTruncationMarker = "…[truncated]"

// BoundLog truncates s to at most MaxLogBytes, appending a marker when it had to cut.
// Truncation respects UTF-8 rune boundaries so the result is always valid text.
func BoundLog(s string) string {
	if len(s) <= MaxLogBytes {
		return s
	}
	cut := MaxLogBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + logTruncationMarker
}

// ActionResult is the outcome of a single remediation action the agent attempted.
// ExitCode and DurationMs are populated only when a command actually ran; for actions
// rejected before execution (unknown action, missing target) ExitCode is nil and
// DurationMs is 0.
type ActionResult struct {
	ActionID   string          `json:"actionId"`
	Target     string          `json:"target,omitempty"`
	Status     ExecutionStatus `json:"status"`
	Stdout     string          `json:"stdout"`
	Stderr     string          `json:"stderr"`
	ExitCode   *int            `json:"exitCode,omitempty"`
	DurationMs int64           `json:"durationMs"`
}

// ExecutionResult is the payload the agent returns to the backend after attempting a
// remediation Command. It mirrors backend/internal/remediation.ExecutionResult; the
// canonical shape is docs/contracts/remediation_result_fixture.json. RequestID echoes
// the dispatched command's RequestID for end-to-end correlation. Timestamps are UTC
// and logs are bounded via BoundLog before sending.
type ExecutionResult struct {
	IncidentID string          `json:"incidentId"`
	DeviceID   string          `json:"deviceId"`
	RequestID  string          `json:"requestId"`
	Status     ExecutionStatus `json:"status"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt time.Time       `json:"finishedAt"`
	Results    []ActionResult  `json:"results"`
}
