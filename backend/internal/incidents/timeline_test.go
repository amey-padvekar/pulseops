package incidents

import (
	"errors"
	"testing"
	"time"
)

func timelineTypes(inc Incident) []TimelineEventType {
	out := make([]TimelineEventType, 0, len(inc.Timeline))
	for _, e := range inc.Timeline {
		out = append(out, e.Type)
	}
	return out
}

func TestAppendTimelineEvent(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)

	at := time.Date(2026, 5, 23, 22, 9, 0, 0, time.UTC)
	updated, err := s.AppendTimelineEvent(id, EventCommandQueued, at, "rem-1")
	if err != nil {
		t.Fatalf("AppendTimelineEvent: %v", err)
	}
	if len(updated.Timeline) != 1 {
		t.Fatalf("expected 1 timeline event, got %d", len(updated.Timeline))
	}
	ev := updated.Timeline[0]
	if ev.Type != EventCommandQueued || !ev.At.Equal(at) || ev.Detail != "rem-1" {
		t.Fatalf("unexpected event: %+v", ev)
	}

	if _, err := s.AppendTimelineEvent("missing", EventCommandQueued, at, ""); !errors.Is(err, ErrIncidentNotFound) {
		t.Fatalf("expected ErrIncidentNotFound, got %v", err)
	}
}

func TestTimeline_FullLifecycleOrder(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)

	// queued (approval-time orchestration), then dispatch, then result.
	if _, err := s.AppendTimelineEvent(id, EventCommandQueued, time.Now().UTC(), "rem-1"); err != nil {
		t.Fatalf("queued: %v", err)
	}
	if _, _, err := s.MarkExecuting(id, "rem-1"); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	outcome := ExecutionOutcome{
		RequestID:  "rem-1",
		Status:     "succeeded",
		StartedAt:  time.Date(2026, 5, 23, 22, 10, 6, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 23, 22, 10, 8, 0, time.UTC),
	}
	final, err := s.SaveRemediationResult(id, outcome, time.Now().UTC(), StateValidating)
	if err != nil {
		t.Fatalf("result: %v", err)
	}

	got := timelineTypes(final)
	want := []TimelineEventType{EventCommandQueued, EventCommandDispatched, EventCommandStarted, EventCommandFinished}
	if len(got) != len(want) {
		t.Fatalf("timeline: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("timeline[%d]: got %q want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	// started/finished must carry the agent's reported timestamps.
	if !final.Timeline[2].At.Equal(outcome.StartedAt) || !final.Timeline[3].At.Equal(outcome.FinishedAt) {
		t.Fatalf("started/finished timestamps not from result: %+v", final.Timeline)
	}
}

func TestMarkExecuting_RecordsDispatchedEventAndRequestID(t *testing.T) {
	s := NewStore()
	id := approvedIncidentInStore(t, s)

	updated, _, err := s.MarkExecuting(id, "rem-42")
	if err != nil {
		t.Fatalf("MarkExecuting: %v", err)
	}
	if updated.RemediationRequestID != "rem-42" {
		t.Fatalf("request id not set at dispatch: %q", updated.RemediationRequestID)
	}
	if got := timelineTypes(updated); len(got) != 1 || got[0] != EventCommandDispatched {
		t.Fatalf("expected a single dispatched event, got %v", got)
	}
}
