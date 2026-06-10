package remediation

import (
	"time"
	"unicode/utf8"
)

// ExecutionStatus is the normalized status vocabulary for the remediation lifecycle,
// used at both the top level of an execution result and per action. Defining one set
// of string values means the backend store and the dashboard render a single,
// predictable vocabulary regardless of which stage produced the status.
//
// The values intentionally share their string form with the command lifecycle
// (CommandStatus): "queued" and "dispatched" mean the same thing whether they appear
// on a queued Command or on an execution result.
type ExecutionStatus string

const (
	// ExecStatusQueued: approved and waiting to be dispatched.
	ExecStatusQueued ExecutionStatus = "queued"
	// ExecStatusDispatched: handed to the endpoint, execution not yet started.
	ExecStatusDispatched ExecutionStatus = "dispatched"
	// ExecStatusRunning: the agent is actively executing the action(s).
	ExecStatusRunning ExecutionStatus = "running"
	// ExecStatusSucceeded: execution completed successfully.
	ExecStatusSucceeded ExecutionStatus = "succeeded"
	// ExecStatusFailed: execution ran but did not succeed.
	ExecStatusFailed ExecutionStatus = "failed"
	// ExecStatusRejected: the agent refused the action (e.g. unknown/unmapped action ID).
	ExecStatusRejected ExecutionStatus = "rejected"
)

// knownExecutionStatuses is the closed set of valid status values.
var knownExecutionStatuses = map[ExecutionStatus]struct{}{
	ExecStatusQueued:     {},
	ExecStatusDispatched: {},
	ExecStatusRunning:    {},
	ExecStatusSucceeded:  {},
	ExecStatusFailed:     {},
	ExecStatusRejected:   {},
}

// IsValid reports whether s is part of the normalized status vocabulary.
func (s ExecutionStatus) IsValid() bool {
	_, ok := knownExecutionStatuses[s]
	return ok
}

// MaxLogBytes bounds how much stdout/stderr a single action result may carry. Logs
// are for explaining what happened during a demo, not for shipping full output, so we
// cap each stream and mark truncation rather than letting an unbounded blob reach the
// store or the dashboard.
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

// ExecutionResult is the payload an endpoint agent returns after attempting a
// remediation command. It is the Phase 9 result contract shared with the agent module
// (mirrored by agent/internal/remediation). RequestID ties it back to the originating
// DispatchCommand for end-to-end correlation.
type ExecutionResult struct {
	IncidentID string          `json:"incidentId"`
	DeviceID   string          `json:"deviceId"`
	RequestID  string          `json:"requestId"`
	Status     ExecutionStatus `json:"status"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt time.Time       `json:"finishedAt"`
	Results    []ActionResult  `json:"results"`
}

// Normalize returns a copy of the result in the canonical form the backend stores and
// renders: timestamps forced to UTC and every action's logs bounded to MaxLogBytes.
// It does not validate status values; callers that need that should check IsValid.
func (r ExecutionResult) Normalize() ExecutionResult {
	out := r
	out.StartedAt = r.StartedAt.UTC()
	out.FinishedAt = r.FinishedAt.UTC()

	out.Results = make([]ActionResult, len(r.Results))
	for i, ar := range r.Results {
		ar.Stdout = BoundLog(ar.Stdout)
		ar.Stderr = BoundLog(ar.Stderr)
		out.Results[i] = ar
	}
	return out
}
