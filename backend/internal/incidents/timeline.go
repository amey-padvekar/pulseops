package incidents

import "time"

// TimelineEventType enumerates the remediation lifecycle milestones recorded on an
// incident's execution timeline (Phase 9 task 4.8.2). These are deliberately scoped to
// the execution phase and kept distinct from approval metadata.
type TimelineEventType string

const (
	EventCommandQueued     TimelineEventType = "command_queued"
	EventCommandDispatched TimelineEventType = "command_dispatched"
	EventCommandStarted    TimelineEventType = "command_started"
	EventCommandFinished   TimelineEventType = "command_finished"
)

// TimelineEvent is a single timestamped entry on the incident execution timeline.
// Detail is an optional short human-readable note (e.g. the request id or status).
type TimelineEvent struct {
	Type   TimelineEventType `json:"type"`
	At     time.Time         `json:"at"`
	Detail string            `json:"detail,omitempty"`
}

// appendTimelineLocked appends an event to the incident's timeline. Callers must hold
// the store write lock. The timestamp is normalized to UTC, defaulting to now when zero.
func appendTimelineLocked(p *Incident, eventType TimelineEventType, at time.Time, detail string) {
	t := at.UTC()
	if t.IsZero() {
		t = time.Now().UTC()
	}
	p.Timeline = append(p.Timeline, TimelineEvent{Type: eventType, At: t, Detail: detail})
}
