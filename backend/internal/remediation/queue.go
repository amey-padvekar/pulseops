package remediation

import (
	"sync"
	"time"
)

// Queue is a thread-safe in-memory store of queued remediation commands that
// Phase 9 will consume. It keeps at most one command per incident: re-approving or
// re-enqueuing the same incident replaces the prior command rather than duplicating
// it, which matches the one-active-incident-per-key model upstream.
type Queue struct {
	mu         sync.RWMutex
	byIncident map[string]Command
	order      []string // incident IDs in first-enqueue order
}

// NewQueue returns an initialized, empty queue.
func NewQueue() *Queue {
	return &Queue{
		byIncident: make(map[string]Command),
	}
}

// Enqueue adds or replaces the command for its incident. Insertion order is
// preserved for first-time incidents; a replacement keeps the original position.
func (q *Queue) Enqueue(cmd Command) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.byIncident[cmd.IncidentID]; !exists {
		q.order = append(q.order, cmd.IncidentID)
	}
	q.byIncident[cmd.IncidentID] = cmd
}

// GetByIncidentID returns the queued command for an incident, if present.
func (q *Queue) GetByIncidentID(incidentID string) (Command, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	cmd, ok := q.byIncident[incidentID]
	return cmd, ok
}

// List returns the queued commands in enqueue order.
func (q *Queue) List() []Command {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make([]Command, 0, len(q.order))
	for _, id := range q.order {
		out = append(out, q.byIncident[id])
	}
	return out
}

// Len reports how many commands are queued.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.byIncident)
}

// ClaimPendingForDevice is the backend dispatch step (Phase 9 task 4.3). It transitions
// every StatusQueued command for deviceID to StatusDispatched and returns the
// wire-format payloads to deliver, in enqueue order.
//
// Each claimed command is stamped with a fresh requestID (from genRequestID) and
// dispatchedAt, and its DispatchCount is incremented for traceability. Commands that
// are not currently queued (already dispatched or acknowledged) are skipped, so a
// command is dispatched exactly once unless RequeueForRetry explicitly re-arms it
// (task 4.3.5). The result is empty when nothing is pending.
//
// Because commands only ever enter the queue via NewCommand — which requires an
// approved incident — this only dispatches approved work (task 4.3.3).
func (q *Queue) ClaimPendingForDevice(deviceID string, dispatchedAt time.Time, genRequestID func() string) []DispatchCommand {
	q.mu.Lock()
	defer q.mu.Unlock()

	var out []DispatchCommand
	for _, id := range q.order {
		cmd := q.byIncident[id]
		if cmd.DeviceID != deviceID || cmd.Status != StatusQueued {
			continue
		}
		cmd.Status = StatusDispatched
		cmd.RequestID = genRequestID()
		cmd.DispatchedAt = dispatchedAt
		cmd.DispatchCount++
		q.byIncident[id] = cmd
		out = append(out, NewDispatchCommand(cmd, cmd.RequestID, dispatchedAt))
	}
	return out
}

// MarkAcknowledged records that the agent acknowledged a dispatched command. Ack is
// optional in the MVP and is purely observational: it advances the lifecycle for
// visibility and does not gate repeat dispatch. Returns false if the incident is
// unknown or the command is not currently dispatched.
func (q *Queue) MarkAcknowledged(incidentID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	cmd, ok := q.byIncident[incidentID]
	if !ok || cmd.Status != StatusDispatched {
		return false
	}
	cmd.Status = StatusAcknowledged
	q.byIncident[incidentID] = cmd
	return true
}

// RequeueForRetry re-arms a previously dispatched command so the next poll dispatches
// it again. This is the explicit opt-in the default once-only dispatch path avoids
// (task 4.3.5), and the backend retry policy for Phase 9 (task 4.10.3):
//
//   - dispatched but not acknowledged (e.g. agent timeout during retrieval, or a result
//     that never arrived) -> retry IS allowed; the command is re-armed and re-dispatched
//     with a fresh requestId on the next poll.
//   - acknowledged (a result was ingested, whether it succeeded or failed) -> retry is
//     REFUSED. This prevents duplicate execution and means a confirmed failed action is
//     never automatically retried (task 4.10.4).
//   - still queued, or unknown incident -> nothing to retry.
//
// Retry always produces a new requestId (assigned at the next dispatch), so the agent's
// per-requestId dedup never mistakes a deliberate retry for a naive duplicate.
//
// The bool reports whether the command was re-armed, so callers can log the decision.
func (q *Queue) RequeueForRetry(incidentID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	cmd, ok := q.byIncident[incidentID]
	if !ok || cmd.Status != StatusDispatched {
		return false
	}
	cmd.Status = StatusQueued
	q.byIncident[incidentID] = cmd
	return true
}
