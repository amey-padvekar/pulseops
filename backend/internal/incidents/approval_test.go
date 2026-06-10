package incidents

import (
	"errors"
	"testing"
	"time"
)

// toAwaitingApproval drives a freshly created incident through the lifecycle to
// awaiting_approval with a recommendation snapshot attached, mirroring the
// Phase 7 -> Phase 8 handoff the backend performs after a completed investigation.
func toAwaitingApproval(t *testing.T, s *Store, recs []RecommendedAction) Incident {
	t.Helper()

	inc, _ := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-1*time.Minute)))

	if _, err := s.UpdateState(inc.IncidentID, StateInvestigating, "triage started"); err != nil {
		t.Fatalf("transition to investigating failed: %v", err)
	}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "service crashed", 0.82, recs, []string{"verify service running"}, "summary", time.Now().UTC(), "trace-1", "completed"); err != nil {
		t.Fatalf("save investigation result failed: %v", err)
	}
	if _, err := s.UpdateState(inc.IncidentID, StateAwaitingApproval, "recommendation ready"); err != nil {
		t.Fatalf("transition to awaiting_approval failed: %v", err)
	}

	awaiting, ok := s.GetByID(inc.IncidentID)
	if !ok {
		t.Fatal("expected incident to exist after reaching awaiting_approval")
	}
	return awaiting
}

func TestApprove_PersistsApprovalMetadata(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{
		{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart the crashed VPN service"},
		{ActionID: "flush_dns", Target: "endpoint", Description: "clear stale resolver cache"},
	}
	awaiting := toAwaitingApproval(t, s, recs)

	// Ensure a clock tick elapses so the UpdatedAt advance is observable on
	// platforms with a coarse timer resolution (e.g. Windows).
	time.Sleep(2 * time.Millisecond)

	approvedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	approved, err := s.Approve(awaiting.IncidentID, "demo.operator", []string{"restart_service"}, "looks correct", approvedAt)
	if err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	if approved.State != StateApproved {
		t.Fatalf("state: got %q want %q", approved.State, StateApproved)
	}
	if approved.ApprovedBy != "demo.operator" {
		t.Fatalf("approvedBy: got %q want %q", approved.ApprovedBy, "demo.operator")
	}
	if approved.ApprovedAt == nil || !approved.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("approvedAt: got %v want %v", approved.ApprovedAt, approvedAt)
	}
	if approved.ApprovalNote != "looks correct" {
		t.Fatalf("approvalNote: got %q want %q", approved.ApprovalNote, "looks correct")
	}
	if len(approved.ApprovedActions) != 1 || approved.ApprovedActions[0] != "restart_service" {
		t.Fatalf("approvedActions: got %v want [restart_service]", approved.ApprovedActions)
	}
	if !approved.UpdatedAt.After(awaiting.UpdatedAt) {
		t.Fatalf("UpdatedAt not advanced: before=%v after=%v", awaiting.UpdatedAt, approved.UpdatedAt)
	}

	// The recommendation snapshot must survive approval untouched: approval data is
	// recorded alongside the recommendation, not by overwriting it (4.1 task 4).
	if len(approved.RecommendedActions) != len(recs) {
		t.Fatalf("recommendation snapshot mutated: got %d actions want %d", len(approved.RecommendedActions), len(recs))
	}
	if approved.RecommendedActions[0].ActionID != "restart_service" || approved.RecommendedActions[0].Description != "restart the crashed VPN service" {
		t.Fatalf("recommendation snapshot corrupted: %+v", approved.RecommendedActions[0])
	}
}

func TestApprove_PersistsAcrossReads(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{{ActionID: "restart_service", Target: "svc", Description: "restart"}}
	awaiting := toAwaitingApproval(t, s, recs)

	approvedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	if _, err := s.Approve(awaiting.IncidentID, "demo.operator", []string{"restart_service"}, "", approvedAt); err != nil {
		t.Fatalf("approve failed: %v", err)
	}

	// A fresh read (e.g. REST refetch) must reproduce the approved state durably.
	reread, ok := s.GetByID(awaiting.IncidentID)
	if !ok {
		t.Fatal("expected incident to exist after approval")
	}
	if reread.State != StateApproved || reread.ApprovedBy != "demo.operator" {
		t.Fatalf("approval not durable on reread: state=%q approvedBy=%q", reread.State, reread.ApprovedBy)
	}
	if reread.ApprovedAt == nil || !reread.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("approvedAt not durable on reread: got %v", reread.ApprovedAt)
	}
}

func TestApprove_RejectsBeforeAwaitingApproval(t *testing.T) {
	s := NewStore()
	inc, _ := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC()))
	if _, err := s.UpdateState(inc.IncidentID, StateInvestigating, "triage"); err != nil {
		t.Fatalf("transition to investigating failed: %v", err)
	}

	// Approval must require awaiting_approval; an incident still investigating is not approvable.
	_, err := s.Approve(inc.IncidentID, "demo.operator", nil, "", time.Now().UTC())
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestApprove_RejectsDuplicate(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{{ActionID: "restart_service", Target: "svc", Description: "restart"}}
	awaiting := toAwaitingApproval(t, s, recs)

	if _, err := s.Approve(awaiting.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC()); err != nil {
		t.Fatalf("first approve failed: %v", err)
	}
	_, err := s.Approve(awaiting.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC())
	if !errors.Is(err, ErrAlreadyApproved) {
		t.Fatalf("expected ErrAlreadyApproved on duplicate, got %v", err)
	}
}

func TestApprove_RejectsActionOutsideRecommendation(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{{ActionID: "restart_service", Target: "svc", Description: "restart"}}
	awaiting := toAwaitingApproval(t, s, recs)

	// reconnect_vpn is a valid catalog action but was not recommended for this incident.
	_, err := s.Approve(awaiting.IncidentID, "demo.operator", []string{"reconnect_vpn"}, "", time.Now().UTC())
	if err == nil {
		t.Fatal("expected error for action id outside the recommendation snapshot")
	}

	// State must remain awaiting_approval after a rejected approval attempt.
	after, ok := s.GetByID(awaiting.IncidentID)
	if !ok {
		t.Fatal("expected incident to exist after rejected approval")
	}
	if after.State != StateAwaitingApproval {
		t.Fatalf("state mutated on rejected approval: got %q want %q", after.State, StateAwaitingApproval)
	}
	if after.ApprovedBy != "" || after.ApprovedAt != nil {
		t.Fatalf("approval metadata leaked on rejected approval: approvedBy=%q approvedAt=%v", after.ApprovedBy, after.ApprovedAt)
	}
}

func TestApprove_NotFound(t *testing.T) {
	s := NewStore()
	_, err := s.Approve("missing", "demo.operator", nil, "", time.Now().UTC())
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}

// investigatedIncident drives an incident to investigating with the supplied
// recommendation snapshot persisted, but leaves the lifecycle in investigating,
// exactly as the agent service does on a completed investigation.
func investigatedIncident(t *testing.T, s *Store, recs []RecommendedAction, status string) Incident {
	t.Helper()

	inc, _ := s.CreateOrGetActive("dev-1|vpn|service_stopped", seedIncident("inc-1", "dev-1", "vpn", time.Now().UTC().Add(-1*time.Minute)))
	if _, err := s.UpdateState(inc.IncidentID, StateInvestigating, "triage started"); err != nil {
		t.Fatalf("transition to investigating failed: %v", err)
	}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "service crashed", 0.82, recs, []string{"verify service running"}, "summary", time.Now().UTC(), "trace-1", status); err != nil {
		t.Fatalf("save investigation result failed: %v", err)
	}
	got, ok := s.GetByID(inc.IncidentID)
	if !ok {
		t.Fatal("expected investigated incident to exist")
	}
	return got
}

func TestPromoteToAwaitingApproval_PromotesOnRecommendation(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{{ActionID: "restart_service", Target: "svc", Description: "restart"}}
	inc := investigatedIncident(t, s, recs, "completed")

	promoted, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID)
	if err != nil {
		t.Fatalf("promote failed: %v", err)
	}
	if !ok {
		t.Fatal("expected promoted=true for a completed, non-empty investigation")
	}
	if promoted.State != StateAwaitingApproval {
		t.Fatalf("state: got %q want %q", promoted.State, StateAwaitingApproval)
	}

	// The transition must be durable on a fresh read.
	reread, _ := s.GetByID(inc.IncidentID)
	if reread.State != StateAwaitingApproval {
		t.Fatalf("promotion not durable: got %q", reread.State)
	}
}

func TestPromoteToAwaitingApproval_NoOpForFallback(t *testing.T) {
	s := NewStore()
	// Fallback/stub result: empty recommendations, status "fallback".
	inc := investigatedIncident(t, s, []RecommendedAction{}, "fallback")

	promoted, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID)
	if err != nil {
		t.Fatalf("promote returned error: %v", err)
	}
	if ok {
		t.Fatal("expected promoted=false for an empty (fallback) recommendation")
	}
	if promoted.State != StateInvestigating {
		t.Fatalf("fallback incident must stay investigating: got %q", promoted.State)
	}
}

func TestPromoteToAwaitingApproval_Idempotent(t *testing.T) {
	s := NewStore()
	recs := []RecommendedAction{{ActionID: "restart_service", Target: "svc", Description: "restart"}}
	inc := investigatedIncident(t, s, recs, "completed")

	if _, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil || !ok {
		t.Fatalf("first promote: ok=%v err=%v", ok, err)
	}
	// A second call is a benign no-op (already awaiting_approval), not an error.
	promoted, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID)
	if err != nil {
		t.Fatalf("second promote returned error: %v", err)
	}
	if ok {
		t.Fatal("expected promoted=false on the idempotent second call")
	}
	if promoted.State != StateAwaitingApproval {
		t.Fatalf("state changed unexpectedly: got %q", promoted.State)
	}
}

func TestPromoteToAwaitingApproval_NotFound(t *testing.T) {
	s := NewStore()
	_, ok, err := s.PromoteToAwaitingApproval("missing")
	if ok {
		t.Fatal("expected promoted=false for missing incident")
	}
	if !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}
