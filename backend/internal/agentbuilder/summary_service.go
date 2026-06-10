package agentbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// DefaultSummaryTimeout bounds a single live summary generation attempt (Phase 11 step
// 4.10 task 1). It is a touch longer than the investigation budget because the narrative
// summary is a second-pass reasoning step, while still resolving well within a demo.
const DefaultSummaryTimeout = 12 * time.Second

// SummaryStore is the persistence surface the summary service needs. *incidents.Store
// satisfies it.
type SummaryStore interface {
	GetByID(incidentID string) (incidents.Incident, bool)
	SetSummaryStatus(incidentID, status, requestID string) (incidents.Incident, error)
	SaveFinalSummary(incidentID string, summary incidents.FinalSummary, requestID, status string, generatedAt time.Time) (incidents.Incident, error)
}

// SummarySubmitter submits a summary request to Agent Builder / ADK and returns the raw
// response payload. A nil submitter means no live backend is available, in which case the
// service uses the deterministic fallback only — so an incident always gets a usable
// closing report.
type SummarySubmitter interface {
	SubmitSummary(ctx context.Context, payload ADKSummaryRequestPayload) (json.RawMessage, error)
}

// SummaryGenerationConfig parameterizes one generation pass.
type SummaryGenerationConfig struct {
	// Timeout bounds the live attempt; DefaultSummaryTimeout is used when non-positive.
	Timeout time.Duration
	// Mode controls idempotency (auto never regenerates an existing summary; operator may).
	Mode SummaryTriggerMode
	// RecentLogs are optional device log lines to consider as evidence.
	RecentLogs []string
	// Now is an injectable clock for deterministic tests; defaults to time.Now().UTC().
	Now time.Time
}

// GenerateAndStoreSummary runs the end-to-end final-summary pass for one completed incident
// (Phase 11 step 4.10): it enforces the trigger/idempotency rules, marks generation
// in-flight, attempts a live summary within the timeout budget, and — on any timeout,
// transport error, or unusable response — synthesizes a deterministic fallback from the
// stored record so the incident stays usable without the AI narrative. The summary
// (live or fallback) is persisted with its status, and request/response identifiers and
// failure reasons are logged.
//
// It returns the updated incident, a changed flag (true when anything was persisted, so the
// caller can broadcast), and an error only for unexpected store failures — a fallback is a
// successful, non-error outcome.
func GenerateAndStoreSummary(
	ctx context.Context,
	store SummaryStore,
	submitter SummarySubmitter,
	incidentID string,
	cfg SummaryGenerationConfig,
) (incidents.Incident, bool, error) {
	if store == nil {
		return incidents.Incident{}, false, errors.New("summary store is required")
	}

	incident, ok := store.GetByID(incidentID)
	if !ok {
		return incidents.Incident{}, false, incidents.ErrIncidentNotFound
	}

	mode := cfg.Mode
	if mode == "" {
		mode = SummaryTriggerAuto
	}

	decision := EvaluateSummaryTrigger(incident.State, incident.SummaryStatus, mode)
	if !decision.Allowed {
		log.Printf("summary generation skipped incident_id=%s reason=%q", incidentID, decision.Reason)
		return incident, false, nil
	}

	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	req, err := BuildFinalSummaryRequest(incident, FinalSummaryOptions{
		RequestedAt: now,
		RecentLogs:  cfg.RecentLogs,
	})
	if err != nil {
		// Cannot even build a request: record a failed status (no stored content) so the UI
		// can show its fallback state, and surface the error.
		log.Printf("summary request build failed incident_id=%s error=%v", incidentID, err)
		updated, serr := store.SetSummaryStatus(incidentID, SummaryStatusFailed, "")
		if serr != nil {
			return incident, false, serr
		}
		return updated, true, err
	}

	// Mark in-flight so a refresh or duplicate trigger cannot double-fire (idempotency).
	if _, serr := store.SetSummaryStatus(incidentID, SummaryStatusPending, req.RequestID); serr != nil {
		return incident, false, serr
	}

	remediationOccurred := RemediationOccurred(req)
	summary, status, failureReason := generateSummaryContent(ctx, submitter, req, cfg.Timeout, remediationOccurred)

	if failureReason != "" {
		log.Printf(
			"summary generation fell back incident_id=%s request_id=%s status=%s reason=%q",
			incidentID, req.RequestID, status, failureReason,
		)
	} else {
		log.Printf(
			"summary generated incident_id=%s request_id=%s status=%s",
			incidentID, req.RequestID, status,
		)
	}

	updated, serr := store.SaveFinalSummary(incidentID, toIncidentFinalSummary(summary), req.RequestID, status, now)
	if serr != nil {
		return incident, false, serr
	}
	return updated, true, nil
}

// generateSummaryContent attempts a live summary and falls back to a deterministic one on
// any timeout, transport error, or unparseable/invalid response. It returns the summary to
// store, the status to record, and a non-empty failureReason when the fallback was used.
func generateSummaryContent(
	ctx context.Context,
	submitter SummarySubmitter,
	req FinalSummaryRequest,
	timeout time.Duration,
	remediationOccurred bool,
) (IncidentSummary, string, string) {
	if submitter == nil {
		return FallbackSummary(req), SummaryStatusFallback, "no summary backend configured"
	}

	if timeout <= 0 {
		timeout = DefaultSummaryTimeout
	}

	payload, err := BuildSummaryADKRequestPayload(req, "")
	if err != nil {
		return FallbackSummary(req), SummaryStatusFallback, "build payload: " + err.Error()
	}

	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	raw, err := submitter.SubmitSummary(attemptCtx, payload)
	if err != nil {
		return FallbackSummary(req), SummaryStatusFallback, "submit: " + err.Error()
	}

	summary, err := ParseFinalSummary(raw, remediationOccurred)
	if err != nil {
		return FallbackSummary(req), SummaryStatusFallback, "parse: " + err.Error()
	}

	return summary, SummaryStatusGenerated, ""
}

// toIncidentFinalSummary maps the agentbuilder result into the incident-local type the
// store persists (avoiding the import cycle).
func toIncidentFinalSummary(s IncidentSummary) incidents.FinalSummary {
	return incidents.FinalSummary{
		RootCause:       s.RootCause,
		Evidence:        s.Evidence,
		ActionsTaken:    s.ActionsTaken,
		Result:          s.Result,
		OperatorSummary: s.OperatorSummary,
	}
}
