package remediation

import (
	"fmt"
	"testing"
	"time"
)

// seqIDs returns a deterministic requestID generator for tests.
func seqIDs() func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("rem-%d", n)
	}
}

func deviceCmd(incidentID, deviceID string) Command {
	c := cmd(incidentID)
	c.DeviceID = deviceID
	return c
}

func TestQueue_ClaimPendingForDevice_DispatchesQueuedAndStamps(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))
	q.Enqueue(deviceCmd("INC-2", "dev-2"))
	q.Enqueue(deviceCmd("INC-3", "dev-1"))

	at := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)
	got := q.ClaimPendingForDevice("dev-1", at, seqIDs())

	if len(got) != 2 {
		t.Fatalf("expected 2 commands for dev-1, got %d", len(got))
	}
	if got[0].IncidentID != "INC-1" || got[1].IncidentID != "INC-3" {
		t.Fatalf("wrong/unordered commands: %+v", got)
	}
	for _, dc := range got {
		if dc.DeviceID != "dev-1" {
			t.Fatalf("claimed a command for the wrong device: %+v", dc)
		}
		if dc.RequestID == "" || !dc.DispatchedAt.Equal(at) {
			t.Fatalf("dispatch fields not stamped: %+v", dc)
		}
	}

	// The underlying queued commands must now be marked dispatched with metadata.
	inc1, _ := q.GetByIncidentID("INC-1")
	if inc1.Status != StatusDispatched || inc1.DispatchCount != 1 || inc1.RequestID == "" {
		t.Fatalf("queued command not transitioned: %+v", inc1)
	}

	// The other device's command is untouched.
	inc2, _ := q.GetByIncidentID("INC-2")
	if inc2.Status != StatusQueued {
		t.Fatalf("dev-2 command should remain queued, got %q", inc2.Status)
	}
}

func TestQueue_ClaimPendingForDevice_DispatchesOnce(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))

	at := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)
	first := q.ClaimPendingForDevice("dev-1", at, seqIDs())
	if len(first) != 1 {
		t.Fatalf("first claim should dispatch 1 command, got %d", len(first))
	}

	second := q.ClaimPendingForDevice("dev-1", at, seqIDs())
	if len(second) != 0 {
		t.Fatalf("a dispatched command must not be re-dispatched, got %d", len(second))
	}
}

func TestQueue_ClaimPendingForDevice_NoneReturnsEmpty(t *testing.T) {
	q := NewQueue()
	if got := q.ClaimPendingForDevice("nobody", time.Unix(0, 0).UTC(), seqIDs()); len(got) != 0 {
		t.Fatalf("expected no commands, got %d", len(got))
	}
}

func TestQueue_RequeueForRetry_AllowsRedispatch(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))
	at := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)

	q.ClaimPendingForDevice("dev-1", at, seqIDs())
	if q.ClaimPendingForDevice("dev-1", at, seqIDs()) != nil {
		t.Fatal("should not re-dispatch before retry is armed")
	}

	if !q.RequeueForRetry("INC-1") {
		t.Fatal("RequeueForRetry should succeed for a known incident")
	}
	again := q.ClaimPendingForDevice("dev-1", at, seqIDs())
	if len(again) != 1 {
		t.Fatalf("retry should re-dispatch the command, got %d", len(again))
	}
	inc1, _ := q.GetByIncidentID("INC-1")
	if inc1.DispatchCount != 2 {
		t.Fatalf("dispatch count should reflect the retry: got %d want 2", inc1.DispatchCount)
	}

	if q.RequeueForRetry("missing") {
		t.Fatal("RequeueForRetry should fail for an unknown incident")
	}
}

func TestQueue_RequeueForRetry_RefusesAcknowledged(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))
	at := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)

	q.ClaimPendingForDevice("dev-1", at, seqIDs())
	if !q.MarkAcknowledged("INC-1") {
		t.Fatal("precondition: command should acknowledge")
	}

	// A confirmed/acknowledged command must never be re-armed (no auto-retry after a
	// result, success or failure).
	if q.RequeueForRetry("INC-1") {
		t.Fatal("RequeueForRetry must refuse an acknowledged command")
	}
}

func TestQueue_RequeueForRetry_RefusesStillQueued(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))

	// Not yet dispatched: there is nothing to retry.
	if q.RequeueForRetry("INC-1") {
		t.Fatal("RequeueForRetry should be a no-op for a still-queued command")
	}
}

func TestQueue_MarkAcknowledged(t *testing.T) {
	q := NewQueue()
	q.Enqueue(deviceCmd("INC-1", "dev-1"))

	// Cannot acknowledge a command that has not been dispatched.
	if q.MarkAcknowledged("INC-1") {
		t.Fatal("should not acknowledge a queued (undispatched) command")
	}

	q.ClaimPendingForDevice("dev-1", time.Unix(0, 0).UTC(), seqIDs())
	if !q.MarkAcknowledged("INC-1") {
		t.Fatal("should acknowledge a dispatched command")
	}
	inc1, _ := q.GetByIncidentID("INC-1")
	if inc1.Status != StatusAcknowledged {
		t.Fatalf("status: got %q want %q", inc1.Status, StatusAcknowledged)
	}
}
