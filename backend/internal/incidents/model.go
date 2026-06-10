package incidents

import (
	"time"
)

// IncidentState describes where an incident is in the lifecycle.
type IncidentState string

const (
	StateHealthy          IncidentState = "healthy"
	StateDetected         IncidentState = "detected"
	StateInvestigating    IncidentState = "investigating"
	StateAwaitingApproval IncidentState = "awaiting_approval"
	StateApproved         IncidentState = "approved"
	StateExecuting        IncidentState = "executing"
	StateValidating       IncidentState = "validating"
	StateResolved         IncidentState = "resolved"
	StateFailed           IncidentState = "failed"
)

// Validation status values describe where post-remediation recovery validation stands,
// independent of the incident lifecycle State (Phase 10 step 4.7).
const (
	ValidationStatusInProgress = "in_progress"
	ValidationStatusSucceeded  = "succeeded"
	ValidationStatusFailed     = "failed"
)

// Severity represents incident urgency.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Incident is the canonical incident record used by store, API, and websocket payloads.
type Incident struct {
	IncidentID    string        `json:"incidentId"`
	DeviceID      string        `json:"deviceId"`
	ServiceName   string        `json:"serviceName"`
	ServiceStatus string        `json:"serviceStatus"`
	State         IncidentState `json:"state"`
	Severity      Severity      `json:"severity"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	DetectedAt    time.Time     `json:"detectedAt"`
	LastSeenAt    time.Time     `json:"lastSeenAt"`
	Reason        string        `json:"reason"`
	Active        bool          `json:"active"`
	// Recommendation produced by Phase 7 (AI). Keep as a snapshot
	// so approval can be validated against it later.
	RecommendedActions []RecommendedAction `json:"recommendedActions,omitempty"`

	// AI investigation result fields (Phase 7)
	ProbableCause  string    `json:"probableCause,omitempty"`
	Confidence     float64   `json:"confidence,omitempty"`
	ValidationSteps []string `json:"validationSteps,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	InvestigatedAt *time.Time `json:"investigatedAt,omitempty"`
	// Investigation status/metadata
	InvestigationStatus   string `json:"investigationStatus,omitempty"`
	InvestigationError    string `json:"investigationError,omitempty"`
	AgentBuilderTraceID   string `json:"agentBuilderTraceId,omitempty"`

	// Approval metadata (Phase 8)
	ApprovedBy      string     `json:"approvedBy,omitempty"`
	ApprovedAt      *time.Time `json:"approvedAt,omitempty"`
	ApprovalNote    string     `json:"approvalNote,omitempty"`
	ApprovedActions []string   `json:"approvedActions,omitempty"`

	// Remediation execution outcome (Phase 9). Populated once the agent reports back.
	RemediationStatus     string                    `json:"remediationStatus,omitempty"`
	RemediationRequestID  string                    `json:"remediationRequestId,omitempty"`
	RemediationStartedAt  *time.Time                `json:"remediationStartedAt,omitempty"`
	RemediationFinishedAt *time.Time                `json:"remediationFinishedAt,omitempty"`
	RemediationReceivedAt *time.Time                `json:"remediationReceivedAt,omitempty"`
	RemediationResults    []RemediationActionResult `json:"remediationResults,omitempty"`

	// Execution timeline (Phase 9 task 4.8): chronological remediation milestones
	// (queued -> dispatched -> started -> finished). Kept separate from approval and
	// execution-result metadata so each phase reads clearly.
	Timeline []TimelineEvent `json:"timeline,omitempty"`

	// ValidationBoundaryAt is the post-remediation freshness watermark (Phase 10 step
	// 4.3). It is the server-observed instant the incident entered validating; only
	// telemetry observed strictly after it counts as recovery evidence. Snapshots from
	// before or during remediation are ignored as stale. The telemetry transport carries
	// no sequence IDs, so freshness is decided purely by UTC-normalized timestamp.
	ValidationBoundaryAt *time.Time `json:"validationBoundaryAt,omitempty"`

	// Validation progress (Phase 10 step 4.4). Consecutive healthy observations are
	// required before closure so a single momentary healthy snapshot cannot resolve an
	// incident.
	//   - HealthyCycleCount: consecutive fresh, healthy observations so far. Reset to 0
	//     whenever a fresh observation is unhealthy, so only a run of healthy cycles counts.
	//   - RequiredHealthyCycles: target run length needed to resolve (default 2). Set when
	//     the incident enters validating; may be lowered to 1 for demo timing.
	//   - LastValidationTelemetryAt: server timestamp of the most recent fresh observation
	//     processed for validation.
	HealthyCycleCount         int        `json:"healthyCycleCount,omitempty"`
	RequiredHealthyCycles     int        `json:"requiredHealthyCycles,omitempty"`
	LastValidationTelemetryAt *time.Time `json:"lastValidationTelemetryAt,omitempty"`

	// Validation outcome detail (Phase 10 step 4.5).
	//   - LastValidationReason: human-readable verdict of the most recent fresh
	//     observation (e.g. "service status is stopped"). Preserved for operator
	//     debugging and later summary generation.
	//   - ValidationFailureReason: set when validation terminates unsuccessfully (timeout
	//     without recovery), explaining why recovery was never confirmed.
	LastValidationReason    string `json:"lastValidationReason,omitempty"`
	ValidationFailureReason string `json:"validationFailureReason,omitempty"`

	// Validation evidence model (Phase 10 step 4.7). These fields, together with the
	// counters and boundary above, let the incident record justify on its own why it
	// resolved or failed — kept distinct from execution logs (RemediationResults) and AI
	// diagnosis (ProbableCause/Summary).
	//   - ValidationStatus: in_progress | succeeded | failed. Set when validation begins
	//     and on its terminal outcome.
	//   - ValidatedAt: when validation concluded (resolution or failure). The matching
	//     "started" instant is ValidationBoundaryAt.
	//   - LastValidationSnapshot: compact evidence of the most recent telemetry evaluated
	//     during validation (the values checked and the per-criterion verdict).
	ValidationStatus       string              `json:"validationStatus,omitempty"`
	ValidatedAt            *time.Time          `json:"validatedAt,omitempty"`
	LastValidationSnapshot *ValidationSnapshot `json:"lastValidationSnapshot,omitempty"`

	// Final summary (Phase 11 step 4.6). The operator-facing closing report, generated
	// once the incident lifecycle is terminal (resolved/failed). Kept distinct from the AI
	// investigation output (ProbableCause/Summary) and validation evidence so the closing
	// report is its own first-class artifact and survives refresh as durable truth.
	//   - FinalSummary: structured closing report (nil until generated).
	//   - SummaryStatus: pending | generated | failed (empty = not generated).
	//   - SummaryGeneratedAt: when generation completed.
	//   - SummaryRequestID: correlation id of the generation request, for tracing.
	FinalSummary       *FinalSummary `json:"finalSummary,omitempty"`
	SummaryStatus      string        `json:"summaryStatus,omitempty"`
	SummaryGeneratedAt *time.Time    `json:"summaryGeneratedAt,omitempty"`
	SummaryRequestID   string        `json:"summaryRequestId,omitempty"`
}

// AcceptsTelemetryAt reports whether a telemetry snapshot observed at seenAt is
// admissible as post-remediation recovery evidence for this incident. It is false until
// a validation boundary has been set (i.e. the incident has entered validating), and
// false for telemetry that is not strictly newer than that boundary. This is the guard
// that keeps Phase 10 validation from acting on stale, pre-remediation telemetry.
func (i Incident) AcceptsTelemetryAt(seenAt time.Time) bool {
	if i.ValidationBoundaryAt == nil {
		return false
	}
	return IsTelemetryFresh(*i.ValidationBoundaryAt, seenAt)
}

// IsTelemetryFresh reports whether telemetry observed at seenAt is newer than the
// post-remediation boundary. Both instants are normalized to UTC before comparison
// (step 4.3 task 4); telemetry exactly at the boundary is treated as stale, so only
// strictly newer telemetry is admitted.
func IsTelemetryFresh(boundary, seenAt time.Time) bool {
	return seenAt.UTC().After(boundary.UTC())
}

// RecommendedAction is a lightweight snapshot of an action option produced by Agent Builder.
type RecommendedAction struct {
	ActionID    string `json:"actionId"`
	Target      string `json:"target,omitempty"`
	Description string `json:"description,omitempty"`
}

// NewIncident creates a detected, active incident using the current UTC time.
func NewIncident(
	incidentID string,
	deviceID string,
	serviceName string,
	serviceStatus string,
	severity Severity,
	reason string,
) Incident {
	return NewIncidentAt(
		incidentID,
		deviceID,
		serviceName,
		serviceStatus,
		severity,
		reason,
		time.Now().UTC(),
	)
}

// NewIncidentAt creates a detected, active incident using the supplied timestamp.
// The timestamp is normalized to UTC for consistent persistence and API output.
func NewIncidentAt(
	incidentID string,
	deviceID string,
	serviceName string,
	serviceStatus string,
	severity Severity,
	reason string,
	detectedAt time.Time,
) Incident {
	t := detectedAt.UTC()
	if severity == "" {
		severity = SeverityMedium
	}

	return Incident{
		IncidentID:    incidentID,
		DeviceID:      deviceID,
		ServiceName:   serviceName,
		ServiceStatus: serviceStatus,
		State:         StateDetected,
		Severity:      severity,
		CreatedAt:     t,
		UpdatedAt:     t,
		DetectedAt:    t,
		LastSeenAt:    t,
		Reason:        reason,
		Active:        true,
	}
}
