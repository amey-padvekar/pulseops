package incidents

import (
	"testing"
	"time"
)

func seedResolvedIncident(t *testing.T, s *Store) Incident {
	t.Helper()
	inc, _ := s.CreateOrGetActive("dev-1|OpenVPNService", Incident{
		IncidentID:  "inc-1",
		DeviceID:    "dev-1",
		ServiceName: "OpenVPNService",
		State:       StateResolved,
	})
	return inc
}

func sampleFinalSummary() FinalSummary {
	return FinalSummary{
		RootCause:       "OpenVPN service unexpectedly stopped.",
		Evidence:        []string{"serviceStatus=stopped", "serviceStatus=running after remediation"},
		ActionsTaken:    []string{"Approved action: restart_service.", "Agent executed the restart."},
		Result:          "Service health recovered and the incident was resolved.",
		OperatorSummary: "Service stopped, remediation restarted it, recovery confirmed.",
	}
}

func TestSaveFinalSummary_PersistsAndDefaultsStatus(t *testing.T) {
	s := NewStore()
	seedResolvedIncident(t, s)
	before, _ := s.GetByID("inc-1")

	at := time.Date(2026, 6, 5, 10, 5, 0, 0, time.UTC)
	got, err := s.SaveFinalSummary("inc-1", sampleFinalSummary(), "sum-req-1", "", at)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.FinalSummary == nil {
		t.Fatal("expected final summary stored")
	}
	if got.FinalSummary.RootCause == "" || len(got.FinalSummary.Evidence) != 2 || len(got.FinalSummary.ActionsTaken) != 2 {
		t.Fatalf("summary content mismatch: %+v", got.FinalSummary)
	}
	if got.SummaryStatus != SummaryStatusGenerated {
		t.Fatalf("status = %q, want %q", got.SummaryStatus, SummaryStatusGenerated)
	}
	if got.SummaryRequestID != "sum-req-1" {
		t.Fatalf("request id = %q", got.SummaryRequestID)
	}
	if got.SummaryGeneratedAt == nil || !got.SummaryGeneratedAt.Equal(at) {
		t.Fatalf("generatedAt mismatch: %v", got.SummaryGeneratedAt)
	}
	if !got.UpdatedAt.After(before.UpdatedAt) && !got.UpdatedAt.Equal(before.UpdatedAt) {
		// UpdatedAt should be refreshed to now (>= before).
		t.Fatalf("UpdatedAt not refreshed: before=%v after=%v", before.UpdatedAt, got.UpdatedAt)
	}
}

func TestSaveFinalSummary_DoesNotChangeLifecycleOrInvestigation(t *testing.T) {
	s := NewStore()
	inc, _ := s.CreateOrGetActive("dev-1|svc", Incident{
		IncidentID:    "inc-2",
		DeviceID:      "dev-1",
		ServiceName:   "svc",
		State:         StateFailed,
		ProbableCause: "service stopped",
		Summary:       "investigation summary",
	})

	got, err := s.SaveFinalSummary("inc-2", sampleFinalSummary(), "", SummaryStatusGenerated, time.Time{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.State != inc.State {
		t.Fatalf("lifecycle changed: %q -> %q", inc.State, got.State)
	}
	// Investigation output must remain separate and untouched.
	if got.ProbableCause != "service stopped" || got.Summary != "investigation summary" {
		t.Fatalf("investigation output altered: cause=%q summary=%q", got.ProbableCause, got.Summary)
	}
	if got.SummaryGeneratedAt == nil {
		t.Fatal("expected generatedAt defaulted to now")
	}
}

func TestSaveFinalSummary_DefensiveCopy(t *testing.T) {
	s := NewStore()
	seedResolvedIncident(t, s)

	summary := sampleFinalSummary()
	if _, err := s.SaveFinalSummary("inc-1", summary, "r", SummaryStatusGenerated, time.Time{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Mutate caller's slices after saving; stored copy must be unaffected.
	summary.Evidence[0] = "TAMPERED"
	summary.ActionsTaken[0] = "TAMPERED"

	got, _ := s.GetByID("inc-1")
	if got.FinalSummary.Evidence[0] == "TAMPERED" || got.FinalSummary.ActionsTaken[0] == "TAMPERED" {
		t.Fatalf("stored summary not isolated from caller mutation: %+v", got.FinalSummary)
	}
}

func TestSaveFinalSummary_NotFound(t *testing.T) {
	s := NewStore()
	if _, err := s.SaveFinalSummary("missing", sampleFinalSummary(), "r", "", time.Time{}); err != ErrIncidentNotFound {
		t.Fatalf("got %v, want ErrIncidentNotFound", err)
	}
}

func TestSetSummaryStatus(t *testing.T) {
	s := NewStore()
	seedResolvedIncident(t, s)

	got, err := s.SetSummaryStatus("inc-1", SummaryStatusPending, "sum-req-9")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SummaryStatus != SummaryStatusPending {
		t.Fatalf("status = %q, want pending", got.SummaryStatus)
	}
	if got.SummaryRequestID != "sum-req-9" {
		t.Fatalf("request id = %q", got.SummaryRequestID)
	}
	if got.FinalSummary != nil {
		t.Fatalf("status-only update must not store content")
	}

	// Transition to failed without content.
	got, err = s.SetSummaryStatus("inc-1", SummaryStatusFailed, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SummaryStatus != SummaryStatusFailed {
		t.Fatalf("status = %q, want failed", got.SummaryStatus)
	}
	// Empty request id should not clear the prior id.
	if got.SummaryRequestID != "sum-req-9" {
		t.Fatalf("request id should be preserved: %q", got.SummaryRequestID)
	}
}

func TestSetSummaryStatus_NotFound(t *testing.T) {
	s := NewStore()
	if _, err := s.SetSummaryStatus("missing", SummaryStatusPending, ""); err != ErrIncidentNotFound {
		t.Fatalf("got %v, want ErrIncidentNotFound", err)
	}
}
