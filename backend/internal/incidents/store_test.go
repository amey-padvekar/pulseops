package incidents

import (
	"errors"
	"testing"
	"time"
)

func seedIncident(id, deviceID, serviceName string, t time.Time) Incident {
	return NewIncidentAt(
		id,
		deviceID,
		serviceName,
		"stopped",
		SeverityHigh,
		"service stopped while heartbeat is present",
		t,
	)
}

func TestStore_CreateOrGetActive_DedupesByKey(t *testing.T) {
	s := NewStore()
	t0 := time.Now().UTC().Add(-1 * time.Minute)

	first, created := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-1", "dev-1", "vpn", t0))
	if !created {
		t.Fatal("expected first call to create incident")
	}

	second, created := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-2", "dev-1", "vpn", time.Now().UTC()))
	if created {
		t.Fatal("expected second call to reuse active incident")
	}
	if second.IncidentID != first.IncidentID {
		t.Fatalf("expected dedupe incidentID %q, got %q", first.IncidentID, second.IncidentID)
	}

	all := s.List(IncidentFilter{})
	if len(all) != 1 {
		t.Fatalf("expected one incident in store, got %d", len(all))
	}
}

func TestStore_Resolve_AllowsNewIncidentForSameKey(t *testing.T) {
	s := NewStore()
	key := "dev-1|vpn|service_stopped"

	first, created := s.CreateOrGetActive(key, seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-2*time.Minute)))
	if !created {
		t.Fatal("expected first incident to be created")
	}

	s.Resolve(first.IncidentID)

	second, created := s.CreateOrGetActive(key, seedIncident("inc-2", "dev-1", "vpn", time.Now().UTC()))
	if !created {
		t.Fatal("expected a new incident after resolve")
	}
	if second.IncidentID == first.IncidentID {
		t.Fatalf("expected new incident ID after resolve, both are %q", second.IncidentID)
	}
}

func TestStore_GetByID_ReturnsCopy(t *testing.T) {
	s := NewStore()
	created, _ := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC()))

	got, ok := s.GetByID(created.IncidentID)
	if !ok {
		t.Fatal("expected incident to be found")
	}
	got.State = StateFailed

	fresh, ok := s.GetByID(created.IncidentID)
	if !ok {
		t.Fatal("expected incident to be found on second read")
	}
	if fresh.State != StateDetected {
		t.Fatalf("store mutated via returned copy: got %q", fresh.State)
	}
}

func TestStore_List_FilterAndSort(t *testing.T) {
	s := NewStore()

	oldInc, _ := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-old", "dev-1", "vpn", time.Now().UTC().Add(-2*time.Minute)))
	time.Sleep(5 * time.Millisecond)
	newInc, _ := s.CreateOrGetActive("dev-2|agent|service_stopped", seedIncident("inc-new", "dev-2", "agent", time.Now().UTC().Add(-1*time.Minute)))

	all := s.List(IncidentFilter{})
	if len(all) != 2 {
		t.Fatalf("expected 2 incidents, got %d", len(all))
	}
	if all[0].IncidentID != newInc.IncidentID {
		t.Fatalf("expected newest UpdatedAt first, got %q then %q", all[0].IncidentID, all[1].IncidentID)
	}

	active := true
	filtered := s.List(IncidentFilter{Active: &active, DeviceID: "dev-1", State: StateDetected})
	if len(filtered) != 1 || filtered[0].IncidentID != oldInc.IncidentID {
		t.Fatalf("unexpected filtered result: %+v", filtered)
	}
}

func TestStore_UpdateState_ResolvePathClearsActiveIndex(t *testing.T) {
	s := NewStore()
	key := "dev-1|vpn|service_stopped"
	created, _ := s.CreateOrGetActive(key, seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-1*time.Minute)))

	updated, err := s.UpdateState(created.IncidentID, StateInvestigating, "triage started")
	if err != nil {
		t.Fatalf("UpdateState investigating failed: %v", err)
	}
	if updated.State != StateInvestigating {
		t.Fatalf("state: got %q, want %q", updated.State, StateInvestigating)
	}
	if updated.Reason != "triage started" {
		t.Fatalf("reason: got %q", updated.Reason)
	}

	resolved, err := s.UpdateState(created.IncidentID, StateResolved, "fixed")
	if err != nil {
		t.Fatalf("UpdateState resolved failed: %v", err)
	}
	if resolved.Active {
		t.Fatal("expected resolved incident to be inactive")
	}

	next, createdNext := s.CreateOrGetActive(key, seedIncident("inc-2", "dev-1", "vpn", time.Now().UTC()))
	if !createdNext {
		t.Fatal("expected key to be clear after resolve")
	}
	if next.IncidentID == created.IncidentID {
		t.Fatal("expected a new incident ID after resolving prior incident")
	}
}

func TestStore_UpdateState_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.UpdateState("missing", StateInvestigating, "")
	if err == nil {
		t.Fatal("expected error for missing incident")
	}
}

func TestStore_UpdateState_InvalidTransitionDoesNotMutate(t *testing.T) {
	s := NewStore()
	created, _ := s.CreateOrGetActive(
		"dev-1|vpn|service_stopped",
		seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-2*time.Minute)),
	)

	resolved, err := s.UpdateState(created.IncidentID, StateResolved, "fixed")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	before, ok := s.GetByID(created.IncidentID)
	if !ok {
		t.Fatal("expected incident to exist")
	}
	if before.State != StateResolved {
		t.Fatalf("precondition failed, got state %q", before.State)
	}

	_, err = s.UpdateState(created.IncidentID, StateInvestigating, "reopen")
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}

	after, ok := s.GetByID(created.IncidentID)
	if !ok {
		t.Fatal("expected incident to exist after invalid transition")
	}
	if after.State != StateResolved {
		t.Fatalf("state mutated on invalid transition: got %q", after.State)
	}
	if after.Reason != resolved.Reason {
		t.Fatalf("reason mutated on invalid transition: got %q want %q", after.Reason, resolved.Reason)
	}
	if !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("updatedAt mutated on invalid transition: before=%v after=%v", before.UpdatedAt, after.UpdatedAt)
	}
}

func TestStore_Touch_UpdatesLastSeenAndUpdatedAt(t *testing.T) {
	s := NewStore()
	created, _ := s.CreateOrGetActive(
		"dev-1|vpn|service_stopped",
		seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-2*time.Minute)),
	)

	seenAt := time.Now().UTC().Add(-10 * time.Second)
	touched, err := s.Touch(created.IncidentID, seenAt)
	if err != nil {
		t.Fatalf("Touch failed: %v", err)
	}
	if !touched.LastSeenAt.Equal(seenAt) {
		t.Fatalf("LastSeenAt: got %v want %v", touched.LastSeenAt, seenAt)
	}
	if !touched.UpdatedAt.Equal(seenAt) {
		t.Fatalf("UpdatedAt: got %v want %v", touched.UpdatedAt, seenAt)
	}
}

func TestStore_Touch_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.Touch("missing", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for missing incident")
	}
}
