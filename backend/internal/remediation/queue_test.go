package remediation

import (
	"testing"
	"time"
)

func cmd(incidentID string) Command {
	return Command{
		IncidentID: incidentID,
		DeviceID:   "dev-1",
		ApprovedBy: "demo.operator",
		ApprovedAt: time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC),
		Actions:    []Action{{ActionID: "restart_service", Target: "svc"}},
		Status:     StatusQueued,
	}
}

func TestQueue_EnqueueAndGet(t *testing.T) {
	q := NewQueue()
	q.Enqueue(cmd("INC-1"))

	got, ok := q.GetByIncidentID("INC-1")
	if !ok {
		t.Fatal("expected command to be queued")
	}
	if got.IncidentID != "INC-1" || got.Status != StatusQueued {
		t.Fatalf("unexpected queued command: %+v", got)
	}
	if _, ok := q.GetByIncidentID("missing"); ok {
		t.Fatal("did not expect a command for an unknown incident")
	}
}

func TestQueue_PreservesOrder(t *testing.T) {
	q := NewQueue()
	q.Enqueue(cmd("INC-1"))
	q.Enqueue(cmd("INC-2"))
	q.Enqueue(cmd("INC-3"))

	list := q.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(list))
	}
	want := []string{"INC-1", "INC-2", "INC-3"}
	for i, c := range list {
		if c.IncidentID != want[i] {
			t.Fatalf("order[%d]: got %q want %q", i, c.IncidentID, want[i])
		}
	}
}

func TestQueue_ReplaceKeepsPositionAndDoesNotDuplicate(t *testing.T) {
	q := NewQueue()
	q.Enqueue(cmd("INC-1"))
	q.Enqueue(cmd("INC-2"))

	// Re-enqueue INC-1 with an advanced status: replaces in place, no duplicate.
	replacement := cmd("INC-1")
	replacement.Status = StatusPendingDispatch
	q.Enqueue(replacement)

	if q.Len() != 2 {
		t.Fatalf("expected 2 commands after replacement, got %d", q.Len())
	}
	list := q.List()
	if list[0].IncidentID != "INC-1" || list[0].Status != StatusPendingDispatch {
		t.Fatalf("replacement not applied in place: %+v", list[0])
	}
	if list[1].IncidentID != "INC-2" {
		t.Fatalf("order disturbed by replacement: %+v", list)
	}
}
