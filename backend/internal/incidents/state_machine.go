package incidents

import (
	"errors"
	"fmt"
)

// ErrInvalidTransition indicates an attempted illegal incident state transition.
var ErrInvalidTransition = errors.New("invalid incident state transition")

// allowedTransitions is the single source of truth for incident lifecycle moves.
// All state changes (store mutators, API handlers) funnel through validateTransition,
// so transition policy lives here rather than being re-decided per handler.
//
// Phase 10 recovery-proof boundary: an executing incident may only reach resolved by
// passing through validating. executing -> resolved is deliberately omitted so a
// command reporting success can never close an incident without post-remediation
// health evidence. executing -> failed remains as a failure shortcut for remediation
// that did not even complete.
var allowedTransitions = map[IncidentState]map[IncidentState]struct{}{
	StateHealthy: {
		StateDetected: {},
	},
	StateDetected: {
		StateInvestigating: {},
		StateResolved:      {},
		StateFailed:        {},
	},
	StateInvestigating: {
		StateAwaitingApproval: {},
		StateApproved:         {},
		StateExecuting:        {},
		StateValidating:       {},
		StateResolved:         {},
		StateFailed:           {},
	},
	StateAwaitingApproval: {
		StateApproved: {},
		StateResolved: {},
		StateFailed:   {},
	},
	StateApproved: {
		StateExecuting: {},
		StateResolved:  {},
		StateFailed:    {},
	},
	StateExecuting: {
		StateValidating: {},
		StateFailed:     {},
	},
	StateValidating: {
		StateInvestigating: {},
		StateResolved:      {},
		StateFailed:        {},
	},
}

// CanTransition reports whether moving from one incident state to another is allowed.
func CanTransition(from, to IncidentState) bool {
	if from == to {
		return true
	}
	nextStates, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = nextStates[to]
	return ok
}

func validateTransition(from, to IncidentState) error {
	if CanTransition(from, to) {
		return nil
	}
	return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
}
