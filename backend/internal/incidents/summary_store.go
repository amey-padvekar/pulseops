package incidents

import (
	"strings"
	"time"
)

// SaveFinalSummary persists a generated final summary on the incident (Phase 11 step 4.6)
// without changing lifecycle state. It stores the structured report, the correlation
// request id, the generation status (defaulting to "generated" when empty), and the
// completion timestamp, then refreshes UpdatedAt so the closing report's arrival is
// reflected in REST/websocket ordering. Slices are defensively copied so later mutation of
// the caller's data cannot alter stored evidence. Returns ErrIncidentNotFound if the
// incident does not exist.
//
// NOTE: like SaveInvestigationResult, this API intentionally avoids importing agentbuilder
// to prevent an import cycle. Callers map the validated agentbuilder.IncidentSummary into
// the incident-local FinalSummary before calling, and pass the agentbuilder.SummaryStatus*
// value as status.
func (s *Store) SaveFinalSummary(incidentID string, summary FinalSummary, requestID string, status string, generatedAt time.Time) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	stored := FinalSummary{
		RootCause:       summary.RootCause,
		Evidence:        append([]string(nil), summary.Evidence...),
		ActionsTaken:    append([]string(nil), summary.ActionsTaken...),
		Result:          summary.Result,
		OperatorSummary: summary.OperatorSummary,
	}
	p.FinalSummary = &stored

	if strings.TrimSpace(requestID) != "" {
		p.SummaryRequestID = requestID
	}
	if strings.TrimSpace(status) == "" {
		p.SummaryStatus = SummaryStatusGenerated
	} else {
		p.SummaryStatus = status
	}

	t := generatedAt.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	p.SummaryGeneratedAt = &t
	p.UpdatedAt = time.Now().UTC()

	return *p, nil
}

// SetSummaryStatus records a summary-generation status (e.g. "pending" while generation is
// in flight, or "failed" after a timeout/malformed response) without storing report
// content. This backs the step 4.3 idempotency rules and the step 4.5 fallback bookkeeping.
// It refreshes UpdatedAt and optionally records the correlation request id. Returns
// ErrIncidentNotFound if the incident does not exist.
func (s *Store) SetSummaryStatus(incidentID string, status string, requestID string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	p.SummaryStatus = status
	if strings.TrimSpace(requestID) != "" {
		p.SummaryRequestID = requestID
	}
	p.UpdatedAt = time.Now().UTC()

	return *p, nil
}
