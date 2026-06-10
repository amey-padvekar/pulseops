package incidents

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrIncidentNotFound = errors.New("incident not found")
var ErrAlreadyApproved = errors.New("incident already approved")

// IncidentFilter narrows list results by common incident dimensions.
type IncidentFilter struct {
	Active   *bool
	DeviceID string
	State    IncidentState
}

// Store keeps incidents in memory and enforces one active incident per dedupe key.
type Store struct {
	mu          sync.RWMutex
	byID        map[string]*Incident
	activeByKey map[string]string
	keyByID     map[string]string
}

// NewStore returns an initialized incident store.
func NewStore() *Store {
	return &Store{
		byID:        make(map[string]*Incident),
		activeByKey: make(map[string]string),
		keyByID:     make(map[string]string),
	}
}

// CreateOrGetActive returns the existing active incident for key or creates one from seed.
// The bool return is true when a new incident was created.
func (s *Store) CreateOrGetActive(key string, seed Incident) (Incident, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if id, ok := s.activeByKey[key]; ok {
		if existing, found := s.byID[id]; found && existing.Active {
			return *existing, false
		}
		delete(s.activeByKey, key)
	}

	now := time.Now().UTC()
	if seed.IncidentID == "" {
		base := now.Format("20060102T150405.000000000")
		seed.IncidentID = base
		// On coarse clocks (notably Windows) two incidents can be created within the same
		// formatted instant; disambiguate with a suffix so a same-tick collision never
		// overwrites an existing incident in byID.
		for suffix := 1; ; suffix++ {
			if _, taken := s.byID[seed.IncidentID]; !taken {
				break
			}
			seed.IncidentID = base + "-" + strconv.Itoa(suffix)
		}
	}
	if seed.State == "" {
		seed.State = StateDetected
	}
	if seed.Severity == "" {
		seed.Severity = SeverityMedium
	}
	if seed.CreatedAt.IsZero() {
		seed.CreatedAt = now
	}
	if seed.DetectedAt.IsZero() {
		seed.DetectedAt = seed.CreatedAt
	}
	if seed.LastSeenAt.IsZero() {
		seed.LastSeenAt = seed.CreatedAt
	}
	seed.UpdatedAt = now
	seed.Active = true

	incidentCopy := seed
	s.byID[seed.IncidentID] = &incidentCopy
	s.activeByKey[key] = seed.IncidentID
	s.keyByID[seed.IncidentID] = key

	return incidentCopy, true
}

// GetByID returns a copy of an incident by ID.
func (s *Store) GetByID(incidentID string) (Incident, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, false
	}
	return *p, true
}

// List returns copies of incidents matching filter, sorted by UpdatedAt descending.
func (s *Store) List(filter IncidentFilter) []Incident {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Incident, 0, len(s.byID))
	for _, p := range s.byID {
		if filter.Active != nil && p.Active != *filter.Active {
			continue
		}
		if filter.DeviceID != "" && p.DeviceID != filter.DeviceID {
			continue
		}
		if filter.State != "" && p.State != filter.State {
			continue
		}
		out = append(out, *p)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	return out
}

// UpdateState updates state metadata for an incident and returns the updated copy.
func (s *Store) UpdateState(incidentID string, nextState IncidentState, reason string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}
	if err := validateTransition(p.State, nextState); err != nil {
		return Incident{}, err
	}

	now := time.Now().UTC()
	p.State = nextState
	if reason != "" {
		p.Reason = reason
	}
	p.UpdatedAt = now
	setValidationBoundaryLocked(p, nextState, now)

	if nextState == StateResolved || nextState == StateFailed {
		p.Active = false
		if key, hasKey := s.keyByID[incidentID]; hasKey {
			delete(s.activeByKey, key)
			delete(s.keyByID, incidentID)
		}
	}

	return *p, nil
}

// Touch refreshes LastSeenAt and UpdatedAt for an existing incident.
func (s *Store) Touch(incidentID string, seenAt time.Time) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	t := seenAt.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	p.LastSeenAt = t
	p.UpdatedAt = t

	return *p, nil
}

// Resolve marks an incident as resolved and clears its active dedupe mapping.
func (s *Store) Resolve(incidentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return
	}

	p.State = StateResolved
	p.Active = false
	p.UpdatedAt = time.Now().UTC()

	if key, hasKey := s.keyByID[incidentID]; hasKey {
		delete(s.activeByKey, key)
		delete(s.keyByID, incidentID)
	}
}

// Delete removes an incident and its dedupe mappings from the store. It returns
// true when an incident existed and was removed. Used to clear/dismiss stale or
// failed incidents from the dashboard (the store is in-memory, so this is a hard
// delete — appropriate for an operator clearing noise during a demo).
func (s *Store) Delete(incidentID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.byID[incidentID]; !ok {
		return false
	}
	delete(s.byID, incidentID)
	if key, hasKey := s.keyByID[incidentID]; hasKey {
		delete(s.keyByID, incidentID)
		if s.activeByKey[key] == incidentID {
			delete(s.activeByKey, key)
		}
	}
	return true
}

// Approve records approval metadata and transitions an incident to the approved state.
// It validates selected action IDs against the incident's recommended actions.
func (s *Store) Approve(incidentID string, approvedBy string, selectedActionIDs []string, note string, approvedAt time.Time) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	if p.State == StateApproved {
		return Incident{}, ErrAlreadyApproved
	}

	if p.State != StateAwaitingApproval {
		return Incident{}, ErrInvalidTransition
	}

	// Validate selected actions exist in recommendation
	allowed := make(map[string]struct{})
	for _, a := range p.RecommendedActions {
		allowed[a.ActionID] = struct{}{}
	}
	for _, sel := range selectedActionIDs {
		if _, ok := allowed[sel]; !ok {
			return Incident{}, errors.New("invalid selected action id")
		}
	}

	// Record approval metadata
	t := approvedAt.UTC()
	p.ApprovedBy = approvedBy
	p.ApprovedAt = &t
	p.ApprovalNote = note
	p.ApprovedActions = append([]string(nil), selectedActionIDs...)
	p.State = StateApproved
	p.UpdatedAt = time.Now().UTC()

	return *p, nil
}

// PromoteToAwaitingApproval is the backend side of the Phase 7 -> Phase 8 handoff.
// The Google Agent Service deliberately leaves incident lifecycle unchanged, so the
// backend owns the gate that moves an investigated incident into an approvable state.
//
// It transitions an incident from investigating to awaiting_approval only when a
// concrete recommendation exists. The promoted return reports whether the transition
// actually happened:
//   - no recommended actions (e.g. a fallback/stub result) -> no-op, promoted=false.
//     A fallback investigation can therefore never become approvable.
//   - state is not investigating (already promoted, or terminal) -> no-op, promoted=false.
//     This keeps the handoff idempotent and race-tolerant for the async investigator.
//   - state is investigating with a recommendation -> transitions, promoted=true.
//
// It returns ErrIncidentNotFound when the incident does not exist.
func (s *Store) PromoteToAwaitingApproval(incidentID string) (Incident, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, false, ErrIncidentNotFound
	}

	// A fallback/stub result carries no actionable recommendation; nothing to approve.
	if len(p.RecommendedActions) == 0 {
		return *p, false, nil
	}

	// Only investigating incidents are eligible. Already-promoted or terminal
	// incidents are left untouched so this best-effort handoff stays idempotent.
	if p.State != StateInvestigating {
		return *p, false, nil
	}

	if err := validateTransition(p.State, StateAwaitingApproval); err != nil {
		return Incident{}, false, err
	}

	p.State = StateAwaitingApproval
	p.UpdatedAt = time.Now().UTC()

	return *p, true, nil
}

// AppendTimelineEvent records a remediation execution milestone on the incident
// timeline (Phase 9 task 4.8.2) and returns the updated incident. It does not change
// lifecycle state. Returns ErrIncidentNotFound if the incident does not exist.
func (s *Store) AppendTimelineEvent(incidentID string, eventType TimelineEventType, at time.Time, detail string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	appendTimelineLocked(p, eventType, at, detail)
	p.UpdatedAt = time.Now().UTC()
	return *p, nil
}

// MarkExecuting transitions an approved incident to executing when remediation
// dispatch begins (Phase 9 task 4.7.3). It is idempotent and best-effort: an incident
// that is not currently approved is left unchanged and changed=false is returned, so
// re-dispatch or a late call never regresses or errors on lifecycle. On the transition
// it records the dispatched command's request id and a command_dispatched timeline
// event. It returns ErrIncidentNotFound if the incident does not exist.
func (s *Store) MarkExecuting(incidentID, requestID string) (Incident, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, false, ErrIncidentNotFound
	}

	if p.State != StateApproved {
		return *p, false, nil
	}
	if err := validateTransition(p.State, StateExecuting); err != nil {
		return Incident{}, false, err
	}

	now := time.Now().UTC()
	p.State = StateExecuting
	if requestID != "" {
		p.RemediationRequestID = requestID
	}
	appendTimelineLocked(p, EventCommandDispatched, now, requestID)
	p.UpdatedAt = now
	return *p, true, nil
}

// SaveRemediationResult persists the agent's execution outcome on the incident and
// advances the lifecycle to nextState (Phase 9 tasks 4.7.4/4.7.5).
//
// If the incident is still in approved when the result arrives (dispatch did not mark
// it executing), it is first moved through executing so the lifecycle reflects that
// execution happened. Execution detail and timestamps are always persisted; nextState
// is only applied when it is a legal transition (otherwise ErrInvalidTransition is
// returned and nothing is changed). A terminal nextState (resolved/failed) deactivates
// the incident and clears its dedupe mapping, matching UpdateState.
func (s *Store) SaveRemediationResult(incidentID string, outcome ExecutionOutcome, receivedAt time.Time, nextState IncidentState) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	// Validate the lifecycle move up front so a bad transition leaves the incident
	// untouched (no half-applied result). The effective starting point accounts for the
	// implicit approved -> executing step below.
	effectiveFrom := p.State
	if effectiveFrom == StateApproved {
		effectiveFrom = StateExecuting
	}
	if nextState != "" && nextState != effectiveFrom {
		if err := validateTransition(effectiveFrom, nextState); err != nil {
			return Incident{}, err
		}
	}

	if p.State == StateApproved {
		p.State = StateExecuting
	}

	// Persist execution detail and timestamps.
	p.RemediationRequestID = outcome.RequestID
	p.RemediationStatus = outcome.Status
	if !outcome.StartedAt.IsZero() {
		t := outcome.StartedAt.UTC()
		p.RemediationStartedAt = &t
	}
	if !outcome.FinishedAt.IsZero() {
		t := outcome.FinishedAt.UTC()
		p.RemediationFinishedAt = &t
	}
	rt := receivedAt.UTC()
	if rt.IsZero() {
		rt = time.Now().UTC()
	}
	p.RemediationReceivedAt = &rt
	p.RemediationResults = append([]RemediationActionResult(nil), outcome.Actions...)

	// Record execution start/finish on the timeline (task 4.8.2), using the agent's
	// reported timestamps so the timeline reflects real execution timing.
	appendTimelineLocked(p, EventCommandStarted, outcome.StartedAt, outcome.RequestID)
	appendTimelineLocked(p, EventCommandFinished, outcome.FinishedAt, outcome.Status)

	// Advance the lifecycle to the post-result boundary.
	if nextState != "" && nextState != p.State {
		p.State = nextState
		// Stamp the freshness watermark using the server receive time so post-remediation
		// telemetry (server-stamped at upsert) can be compared against it (step 4.3).
		setValidationBoundaryLocked(p, nextState, rt)
		if nextState == StateResolved || nextState == StateFailed {
			p.Active = false
			if key, hasKey := s.keyByID[incidentID]; hasKey {
				delete(s.activeByKey, key)
				delete(s.keyByID, incidentID)
			}
		}
	}

	p.UpdatedAt = time.Now().UTC()
	return *p, nil
}

// DefaultRequiredHealthyCycles is the number of consecutive fresh, healthy telemetry
// observations required before an incident is resolved (Phase 10 step 4.4). Two cycles
// guard against a single momentary healthy snapshot closing an incident prematurely; a
// demo may lower an incident's RequiredHealthyCycles to 1 if timing demands it.
const DefaultRequiredHealthyCycles = 2

// DefaultValidationTimeout bounds how long an incident may sit in validating before the
// backend gives up and fails it (Phase 10 step 4.5). With a ~10s heartbeat interval, 60s
// allows several telemetry cycles (enough to observe the required healthy run plus slack)
// while still resolving to a deterministic outcome well within a live demo.
const DefaultValidationTimeout = 60 * time.Second

// setValidationBoundaryLocked stamps the post-remediation freshness watermark when an
// incident enters validating, capturing the instant from which telemetry is admissible
// as recovery evidence (step 4.3), and initializes the healthy-cycle target (step 4.4).
// It is a no-op for any other target state and is idempotent: an already-set boundary is
// never moved, so a re-entry into validating cannot reset the window. Callers must hold s.mu.
func setValidationBoundaryLocked(p *Incident, nextState IncidentState, at time.Time) {
	if nextState != StateValidating || p.ValidationBoundaryAt != nil {
		return
	}
	t := at.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	p.ValidationBoundaryAt = &t
	if p.RequiredHealthyCycles == 0 {
		p.RequiredHealthyCycles = DefaultRequiredHealthyCycles
	}
	if p.ValidationStatus == "" {
		p.ValidationStatus = ValidationStatusInProgress
	}
}

// SaveInvestigationFailure records investigation failure metadata without changing
// the incident lifecycle. It sets InvestigationStatus (e.g., "timeout"), records
// an error message, associates the Agent Builder trace id if available, and updates timestamps.
func (s *Store) SaveInvestigationFailure(incidentID string, status string, errMsg string, traceID string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	p.InvestigationStatus = status
	p.InvestigationError = errMsg
	p.AgentBuilderTraceID = traceID
	p.UpdatedAt = time.Now().UTC()

	return *p, nil
}

// SaveInvestigationResult stores a validated AI investigation result on the incident
// without altering incident lifecycle state. It updates UpdatedAt and sets
// InvestigatedAt to the provided time (or now UTC if zero).
//
// NOTE: This API intentionally avoids importing agentbuilder to prevent
// package import cycles. Callers should map agentbuilder types into the
// incident-local representation before calling.
func (s *Store) SaveInvestigationResult(incidentID string, probableCause string, confidence float64, recommended []RecommendedAction, validationSteps []string, summary string, investigatedAt time.Time, traceID string, status string) (Incident, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p, ok := s.byID[incidentID]
	if !ok {
		return Incident{}, ErrIncidentNotFound
	}

	// map recommended actions (already in incident-local type)
	p.RecommendedActions = append([]RecommendedAction(nil), recommended...)
	p.ProbableCause = probableCause
	p.Confidence = confidence
	p.ValidationSteps = append([]string(nil), validationSteps...)
	p.Summary = summary

	t := investigatedAt.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	p.InvestigatedAt = &t
	p.UpdatedAt = time.Now().UTC()

	if strings.TrimSpace(status) == "" {
		p.InvestigationStatus = "completed"
	} else {
		p.InvestigationStatus = status
	}
	p.InvestigationError = ""
	p.AgentBuilderTraceID = traceID

	return *p, nil
}
