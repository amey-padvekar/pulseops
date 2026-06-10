package remediation

import "sync"

// requestDeduper tracks the remediation request IDs the agent has already executed so a
// command delivered more than once for the same request ID is never run twice
// (Phase 9 task 4.10.2). A request ID is stable per dispatch attempt, so "already
// completed" is keyed on it. It is safe for concurrent use.
//
// Note: a deliberate backend retry re-dispatches with a *new* request ID, so it is not
// treated as a duplicate here — that is controlled recovery, not a naive repeat.
type requestDeduper struct {
	mu        sync.Mutex
	completed map[string]struct{}
}

func newRequestDeduper() *requestDeduper {
	return &requestDeduper{completed: make(map[string]struct{})}
}

// done reports whether the request ID has already been executed to completion. An empty
// request ID is never considered done (such a command predates dispatch stamping).
func (d *requestDeduper) done(requestID string) bool {
	if requestID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.completed[requestID]
	return ok
}

// markDone records that the request ID has been executed (success or failure) so it is
// never run again under the same ID.
func (d *requestDeduper) markDone(requestID string) {
	if requestID == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.completed[requestID] = struct{}{}
}
