package incidents

import (
	"errors"
	"fmt"
)

// ErrInvalidTransition indicates an attempted illegal incident state transition.
var ErrInvalidTransition = errors.New("invalid incident state transition")

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
		StateResolved:   {},
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
