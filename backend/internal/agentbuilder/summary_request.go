package agentbuilder

import (
	"fmt"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// Explicit terminal outcomes for a completed incident. These are derived from the
// incident lifecycle state so a failed incident is summarized as a failure rather than
// dressed up as a recovery (Phase 11 step 4.2 task 4).
const (
	OutcomeResolved   = "resolved"
	OutcomeFailed     = "failed"
	OutcomeIncomplete = "incomplete"
)

const (
	defaultMaxEvidence  = 12
	defaultMaxSnippetLen = 280
)

// FinalSummaryRequest is the bounded context package backend sends into the Agent
// Builder summary workflow once an incident lifecycle is complete (Phase 11 step 4.2).
//
// It is assembled entirely from the stored incident record — the state accumulated
// across Phases 4-10 (detection, AI diagnosis, approval, execution, validation) — so the
// summary stays grounded in real data. Large free-text (logs, stdout/stderr) is summarized
// before inclusion to keep the payload compact, and Outcome states the terminal verdict
// explicitly so failed incidents are summarized accurately.
type FinalSummaryRequest struct {
	SchemaVersion string    `json:"schemaVersion,omitempty"`
	RequestID     string    `json:"requestId"`
	RequestedAt   time.Time `json:"requestedAt"`

	// Incident metadata (Phase 4).
	IncidentID  string    `json:"incidentId"`
	DeviceID    string    `json:"deviceId"`
	ServiceName string    `json:"serviceName"`
	Severity    string    `json:"severity"`
	State       string    `json:"state"`
	Outcome     string    `json:"outcome"`
	Reason      string    `json:"reason,omitempty"`
	DetectedAt  time.Time `json:"detectedAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty"`

	// AI diagnosis (Phase 7).
	ProbableCause    string  `json:"probableCause,omitempty"`
	Confidence       float64 `json:"confidence,omitempty"`
	DiagnosisSummary string  `json:"diagnosisSummary,omitempty"`

	// Recommendation and approval (Phases 7-8).
	RecommendedActions []SummaryAction `json:"recommendedActions,omitempty"`
	ApprovedActions    []string        `json:"approvedActions,omitempty"`
	ApprovedBy         string          `json:"approvedBy,omitempty"`
	ApprovalNote       string          `json:"approvalNote,omitempty"`

	// Execution results (Phase 9).
	RemediationStatus string                    `json:"remediationStatus,omitempty"`
	ExecutionResults  []SummaryExecutionResult  `json:"executionResults,omitempty"`

	// Validation outcome (Phase 10).
	ValidationStatus        string `json:"validationStatus,omitempty"`
	ValidationReason        string `json:"validationReason,omitempty"`
	ValidationFailureReason string `json:"validationFailureReason,omitempty"`
	HealthyCycleCount       int    `json:"healthyCycleCount,omitempty"`
	RequiredHealthyCycles   int    `json:"requiredHealthyCycles,omitempty"`

	// Selected evidence snippets from telemetry, logs, and the incident timeline,
	// bounded and summarized for a compact payload.
	Evidence []string `json:"evidence,omitempty"`
}

// SummaryAction is a compact view of a recommended action for the summary request.
type SummaryAction struct {
	ActionID    string `json:"actionId"`
	Target      string `json:"target,omitempty"`
	Description string `json:"description,omitempty"`
}

// SummaryExecutionResult is a bounded view of one executed remediation action's outcome.
// The agent's raw stdout/stderr are summarized into a single short Detail line rather than
// included verbatim, keeping large logs out of the payload.
type SummaryExecutionResult struct {
	ActionID   string `json:"actionId"`
	Target     string `json:"target,omitempty"`
	Status     string `json:"status"`
	ExitCode   *int   `json:"exitCode,omitempty"`
	DurationMs int64  `json:"durationMs,omitempty"`
	Detail     string `json:"detail,omitempty"`
}

// FinalSummaryOptions controls assembly of the summary request.
type FinalSummaryOptions struct {
	RequestID     string
	RequestedAt   time.Time
	SchemaVersion string
	// RecentLogs are device log lines to consider as evidence. Optional.
	RecentLogs []string
	// MaxEvidence bounds the evidence array length (default 12).
	MaxEvidence int
	// MaxSnippetLen bounds the length of any single evidence/detail string (default 280).
	MaxSnippetLen int
}

// BuildFinalSummaryRequest assembles a bounded summary context package from a completed
// incident record (Phase 11 step 4.2). It requires an incident id and an explicit
// terminal outcome — callers should only invoke it for resolved or failed incidents.
func BuildFinalSummaryRequest(incident incidents.Incident, opts FinalSummaryOptions) (FinalSummaryRequest, error) {
	if strings.TrimSpace(incident.IncidentID) == "" {
		return FinalSummaryRequest{}, fmt.Errorf("incident missing incidentId")
	}

	requestedAt := opts.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now().UTC()
	} else {
		requestedAt = requestedAt.UTC()
	}

	requestID := strings.TrimSpace(opts.RequestID)
	if requestID == "" {
		requestID = requestedAt.Format(requestIDTimeFormat)
	}

	schemaVersion := strings.TrimSpace(opts.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = defaultSchemaVersion
	}

	maxEvidence := opts.MaxEvidence
	if maxEvidence <= 0 {
		maxEvidence = defaultMaxEvidence
	}
	maxSnippet := opts.MaxSnippetLen
	if maxSnippet <= 0 {
		maxSnippet = defaultMaxSnippetLen
	}

	req := FinalSummaryRequest{
		SchemaVersion: schemaVersion,
		RequestID:     requestID,
		RequestedAt:   requestedAt,

		IncidentID:  incident.IncidentID,
		DeviceID:    strings.TrimSpace(incident.DeviceID),
		ServiceName: strings.TrimSpace(incident.ServiceName),
		Severity:    string(incident.Severity),
		State:       string(incident.State),
		Outcome:     deriveOutcome(incident.State),
		Reason:      strings.TrimSpace(incident.Reason),
		DetectedAt:  incident.DetectedAt.UTC(),
		ResolvedAt:  utcPtr(incident.ValidatedAt),

		ProbableCause:    strings.TrimSpace(incident.ProbableCause),
		Confidence:       incident.Confidence,
		DiagnosisSummary: strings.TrimSpace(incident.Summary),

		RecommendedActions: summaryActions(incident.RecommendedActions),
		ApprovedActions:    trimmedNonEmpty(incident.ApprovedActions),
		ApprovedBy:         strings.TrimSpace(incident.ApprovedBy),
		ApprovalNote:       strings.TrimSpace(incident.ApprovalNote),

		RemediationStatus: strings.TrimSpace(incident.RemediationStatus),
		ExecutionResults:  summaryExecutionResults(incident.RemediationResults, maxSnippet),

		ValidationStatus:        strings.TrimSpace(incident.ValidationStatus),
		ValidationReason:        strings.TrimSpace(incident.LastValidationReason),
		ValidationFailureReason: strings.TrimSpace(incident.ValidationFailureReason),
		HealthyCycleCount:       incident.HealthyCycleCount,
		RequiredHealthyCycles:   incident.RequiredHealthyCycles,

		Evidence: selectEvidence(incident, opts.RecentLogs, maxEvidence, maxSnippet),
	}

	return req, nil
}

// deriveOutcome maps the terminal incident state to an explicit outcome string so the
// summary can distinguish recovery from failure (task 4). Any non-terminal state is
// reported as incomplete rather than guessed.
func deriveOutcome(state incidents.IncidentState) string {
	switch state {
	case incidents.StateResolved:
		return OutcomeResolved
	case incidents.StateFailed:
		return OutcomeFailed
	default:
		return OutcomeIncomplete
	}
}

func summaryActions(actions []incidents.RecommendedAction) []SummaryAction {
	if len(actions) == 0 {
		return nil
	}
	out := make([]SummaryAction, 0, len(actions))
	for _, a := range actions {
		out = append(out, SummaryAction{
			ActionID:    strings.TrimSpace(a.ActionID),
			Target:      strings.TrimSpace(a.Target),
			Description: strings.TrimSpace(a.Description),
		})
	}
	return out
}

func summaryExecutionResults(results []incidents.RemediationActionResult, maxSnippet int) []SummaryExecutionResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]SummaryExecutionResult, 0, len(results))
	for _, r := range results {
		out = append(out, SummaryExecutionResult{
			ActionID:   strings.TrimSpace(r.ActionID),
			Target:     strings.TrimSpace(r.Target),
			Status:     strings.TrimSpace(r.Status),
			ExitCode:   r.ExitCode,
			DurationMs: r.DurationMs,
			Detail:     summarizeExecutionDetail(r, maxSnippet),
		})
	}
	return out
}

// summarizeExecutionDetail collapses an action's stdout/stderr into one short, bounded
// line so large command output never bloats the payload. stderr is preferred when present
// since it most often explains a failure.
func summarizeExecutionDetail(r incidents.RemediationActionResult, maxSnippet int) string {
	stderr := strings.TrimSpace(r.Stderr)
	if stderr != "" {
		return truncateSnippet("stderr: "+collapseWhitespace(stderr), maxSnippet)
	}
	stdout := strings.TrimSpace(r.Stdout)
	if stdout != "" {
		return truncateSnippet("stdout: "+collapseWhitespace(stdout), maxSnippet)
	}
	return ""
}

// truncateSnippet bounds s to at most maxLen bytes, appending an ellipsis when it has to
// cut. The ellipsis is counted against the budget so the result never exceeds maxLen, and
// any partial trailing UTF-8 rune left by the cut is trimmed.
func truncateSnippet(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	// ASCII ellipsis keeps the summary text encoding-safe for plain-text artifacts and
	// clients (e.g. PowerShell) that mis-decode multibyte UTF-8 without a charset hint.
	const ellipsis = "..."
	if maxLen <= len(ellipsis) {
		return s[:maxLen]
	}
	cut := s[:maxLen-len(ellipsis)]
	cut = strings.ToValidUTF8(cut, "")
	return cut + ellipsis
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func trimmedNonEmpty(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
