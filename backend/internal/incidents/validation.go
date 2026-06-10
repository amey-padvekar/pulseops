package incidents

import (
	"fmt"
	"time"

	"github.com/certainelf/pulseops/backend/internal/store"
)

// ValidationOutcome reports the effect of recording one post-remediation telemetry
// observation against an incident's recovery validation (Phase 10 step 4.4).
type ValidationOutcome struct {
	// Incident is the incident after the observation was applied.
	Incident Incident
	// Admitted is true when the observation was a fresh, in-validation event that
	// actually advanced (or reset) the counter. A stale or out-of-phase event is not
	// admitted and leaves the incident untouched.
	Admitted bool
	// Healthy is the health verdict for this observation (meaningful only when Admitted).
	Healthy bool
	// Evaluation is the per-criterion breakdown for this observation, so callers can log
	// or broadcast why the cycle counted or reset (meaningful only when Admitted).
	Evaluation HealthEvaluation
	// Resolved is true when this observation completed the required healthy run and the
	// incident transitioned to resolved.
	Resolved bool
}

// RecordValidationObservation applies one telemetry snapshot to an incident's recovery
// validation and advances the healthy-cycle counter (Phase 10 step 4.4).
//
// The observation is admitted only when the incident is validating and the snapshot is
// fresh post-remediation evidence (step 4.3). For an admitted observation:
//   - a healthy snapshot increments HealthyCycleCount;
//   - an unhealthy snapshot resets HealthyCycleCount to 0 so only a consecutive run of
//     healthy cycles can resolve the incident;
//   - reaching RequiredHealthyCycles transitions the incident to resolved and clears its
//     active dedupe mapping.
//
// A stale or out-of-phase snapshot is ignored (Admitted=false) and never mutates state.
// Returns ErrIncidentNotFound if the incident does not exist.
func (s *Store) RecordValidationObservation(incidentID string, state store.DeviceState, criteria HealthCriteria) (ValidationOutcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return ValidationOutcome{}, ErrIncidentNotFound
	}

	// Only validating incidents accept recovery evidence, and only telemetry observed
	// strictly after the post-remediation boundary is admissible.
	if p.State != StateValidating {
		return ValidationOutcome{Incident: *p}, nil
	}
	seenAt := state.LastSeenAt.UTC()
	if !p.AcceptsTelemetryAt(seenAt) {
		return ValidationOutcome{Incident: *p}, nil
	}

	eval := EvaluateHealth(state, criteria)

	now := time.Now().UTC()
	p.LastValidationTelemetryAt = &seenAt
	p.LastValidationReason = eval.Reason
	// Preserve a compact evidence snapshot of the telemetry that produced this verdict
	// (step 4.7), kept separate from execution logs and AI diagnosis.
	snapshot := newValidationSnapshot(state, eval, seenAt)
	p.LastValidationSnapshot = &snapshot
	if eval.Healthy {
		p.HealthyCycleCount++
	} else {
		// Hold-the-line: a single unhealthy cycle resets progress so resolution requires
		// a fresh consecutive run. Failure-path handling lives in a later step.
		p.HealthyCycleCount = 0
	}

	required := p.RequiredHealthyCycles
	if required <= 0 {
		required = DefaultRequiredHealthyCycles
	}

	resolved := false
	if eval.Healthy && p.HealthyCycleCount >= required {
		p.State = StateResolved
		p.Reason = eval.Reason
		p.ValidationStatus = ValidationStatusSucceeded
		p.ValidatedAt = &now
		p.Active = false
		if key, hasKey := s.keyByID[incidentID]; hasKey {
			delete(s.activeByKey, key)
			delete(s.keyByID, incidentID)
		}
		resolved = true
	}

	p.UpdatedAt = now

	return ValidationOutcome{
		Incident:   *p,
		Admitted:   true,
		Healthy:    eval.Healthy,
		Evaluation: eval,
		Resolved:   resolved,
	}, nil
}

// ExpireValidationIfTimedOut fails a validating incident whose validation window has
// elapsed without confirming recovery (Phase 10 step 4.5), so an incident never sits in
// validating forever. It is a no-op (changed=false) when the incident is not validating,
// has no validation boundary, or the window has not yet elapsed. On expiry it transitions
// the incident to failed, records a derived failure reason, and clears the active dedupe
// mapping. Returns ErrIncidentNotFound if the incident does not exist.
func (s *Store) ExpireValidationIfTimedOut(incidentID string, now time.Time, timeout time.Duration) (Incident, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, false, ErrIncidentNotFound
	}

	if !validationTimedOutLocked(p, now, timeout) {
		return *p, false, nil
	}

	s.failValidationLocked(p, incidentID, validationTimeoutReason(p), now)
	return *p, true, nil
}

// ExpireTimedOutValidations sweeps every validating incident and fails those whose
// validation window has elapsed (Phase 10 step 4.5). It returns the incidents that were
// transitioned to failed, so a periodic caller can broadcast them. A zero or negative
// timeout falls back to DefaultValidationTimeout.
func (s *Store) ExpireTimedOutValidations(now time.Time, timeout time.Duration) []Incident {
	if timeout <= 0 {
		timeout = DefaultValidationTimeout
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var failed []Incident
	for id, p := range s.byID {
		if !validationTimedOutLocked(p, now, timeout) {
			continue
		}
		s.failValidationLocked(p, id, validationTimeoutReason(p), now)
		failed = append(failed, *p)
	}
	return failed
}

// validationTimedOutLocked reports whether a validating incident has passed its
// validation deadline (boundary + timeout). Callers must hold s.mu.
func validationTimedOutLocked(p *Incident, now time.Time, timeout time.Duration) bool {
	if p.State != StateValidating || p.ValidationBoundaryAt == nil {
		return false
	}
	deadline := p.ValidationBoundaryAt.Add(timeout)
	return now.UTC().After(deadline)
}

// failValidationLocked transitions an incident to failed with the supplied reason and
// clears its active dedupe mapping so a new incident can be opened for the same key.
// Callers must hold s.mu.
func (s *Store) failValidationLocked(p *Incident, incidentID, reason string, now time.Time) {
	t := now.UTC()
	p.State = StateFailed
	p.ValidationFailureReason = reason
	p.ValidationStatus = ValidationStatusFailed
	p.ValidatedAt = &t
	p.Reason = reason
	p.Active = false
	if key, hasKey := s.keyByID[incidentID]; hasKey {
		delete(s.activeByKey, key)
		delete(s.keyByID, incidentID)
	}
	p.UpdatedAt = t
}

// validationTimeoutReason derives a deterministic, operator-readable explanation for why
// validation timed out, citing the most specific available evidence (step 4.5 task 3):
//   - no fresh telemetry was ever processed during validation;
//   - some but not enough consecutive healthy cycles were observed;
//   - the most recent fresh observation was unhealthy (service stopped, heartbeat
//     missing, connectivity down), in which case its reason is surfaced verbatim.
func validationTimeoutReason(p *Incident) string {
	if p.LastValidationTelemetryAt == nil {
		return "validation timed out: no fresh telemetry received after remediation"
	}

	required := p.RequiredHealthyCycles
	if required <= 0 {
		required = DefaultRequiredHealthyCycles
	}

	if p.HealthyCycleCount > 0 {
		return fmt.Sprintf(
			"validation timed out: only %d of %d consecutive healthy cycles observed",
			p.HealthyCycleCount, required,
		)
	}

	if p.LastValidationReason != "" {
		return "validation timed out: " + p.LastValidationReason
	}
	return "validation timed out: endpoint did not return to healthy"
}
