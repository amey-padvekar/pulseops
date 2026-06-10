package agentbuilder

import (
	"fmt"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// Summary generation lifecycle status. These describe where final-summary generation
// stands for an incident, independent of the incident's own lifecycle State. They are
// defined here (with the trigger rules that read them) and will be persisted on the
// incident record by the storage model in step 4.6.
const (
	// SummaryStatusNone means no summary has been generated or attempted.
	SummaryStatusNone = ""
	// SummaryStatusPending means generation is in flight; used to stop a refresh or a
	// concurrent automatic trigger from firing a second time.
	SummaryStatusPending = "pending"
	// SummaryStatusGenerated means a structured summary was produced and stored.
	SummaryStatusGenerated = "generated"
	// SummaryStatusFallback means a deterministic summary was synthesized from the stored
	// incident record because live generation timed out or returned unusable output. The
	// incident still has a usable closing report; it is just not the AI narrative.
	SummaryStatusFallback = "fallback"
	// SummaryStatusFailed means a generation attempt failed without even a stored fallback
	// (e.g. the request could not be built); a retry is permitted.
	SummaryStatusFailed = "failed"
)

// SummaryTriggerMode records how summary generation was initiated.
//
// Phase 11 supports both paths: generation fires automatically the moment an incident
// reaches a terminal state, and an operator can explicitly (re)trigger it as a
// debugging/demo backup. The mode changes only the idempotency rules — an automatic
// trigger never regenerates an existing summary, while an explicit operator trigger may.
type SummaryTriggerMode string

const (
	// SummaryTriggerAuto fires automatically at incident closure (resolved/failed).
	SummaryTriggerAuto SummaryTriggerMode = "automatic"
	// SummaryTriggerOperator is an explicit operator/demo request and may regenerate.
	SummaryTriggerOperator SummaryTriggerMode = "operator"
)

// SummaryTriggerDecision is the result of evaluating whether summary generation should
// proceed for an incident. Reason is a deterministic, operator-readable explanation,
// populated whether or not generation is allowed.
type SummaryTriggerDecision struct {
	Allowed bool
	Reason  string
}

// IsSummaryTriggerState reports whether an incident lifecycle state is a terminal state
// from which a final summary may be generated. Only resolved and failed qualify; every
// in-progress state (investigating, awaiting_approval, approved, executing, validating,
// detected, healthy) is excluded so a summary is never built from an incomplete incident.
func IsSummaryTriggerState(state incidents.IncidentState) bool {
	switch state {
	case incidents.StateResolved, incidents.StateFailed:
		return true
	default:
		return false
	}
}

// EvaluateSummaryTrigger decides whether final-summary generation should proceed for an
// incident given its lifecycle state, the current summary-generation status, and how the
// trigger was initiated.
//
// The rules, in order:
//  1. The incident must be in a terminal state (resolved or failed); otherwise generation
//     is refused. This is what keeps generation at a predictable lifecycle boundary.
//  2. Idempotency: when a summary is already generated or currently pending, an automatic
//     trigger is refused so a page refresh or a duplicate closure event cannot regenerate
//     it. An explicit operator trigger is allowed to override and regenerate.
//  3. A prior failed (or absent) summary may always be (re)attempted.
func EvaluateSummaryTrigger(state incidents.IncidentState, currentStatus string, mode SummaryTriggerMode) SummaryTriggerDecision {
	if !IsSummaryTriggerState(state) {
		return SummaryTriggerDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"incident state %q is not terminal; summary requires resolved or failed",
				state,
			),
		}
	}

	switch currentStatus {
	case SummaryStatusGenerated, SummaryStatusPending, SummaryStatusFallback:
		if mode == SummaryTriggerOperator {
			return SummaryTriggerDecision{
				Allowed: true,
				Reason:  fmt.Sprintf("operator-triggered regeneration over existing %q summary", statusLabel(currentStatus)),
			}
		}
		return SummaryTriggerDecision{
			Allowed: false,
			Reason:  fmt.Sprintf("summary already %s; skipping automatic regeneration", statusLabel(currentStatus)),
		}
	default:
		// SummaryStatusNone or SummaryStatusFailed — safe to generate / retry.
		return SummaryTriggerDecision{
			Allowed: true,
			Reason:  fmt.Sprintf("incident is %s and summary is %s", state, statusLabel(currentStatus)),
		}
	}
}

func statusLabel(status string) string {
	if status == SummaryStatusNone {
		return "not generated"
	}
	return status
}
