package incidents

import (
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrIncidentNotFound = errors.New("incident not found")

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
		seed.IncidentID = now.Format("20060102T150405.000000000")
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

	p.State = nextState
	if reason != "" {
		p.Reason = reason
	}
	p.UpdatedAt = time.Now().UTC()

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
