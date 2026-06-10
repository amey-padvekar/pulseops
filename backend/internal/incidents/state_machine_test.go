package incidents

import (
	"errors"
	"testing"
)

func TestCanTransition_MinimumPhase4Path(t *testing.T) {
	if !CanTransition(StateDetected, StateInvestigating) {
		t.Fatal("expected detected -> investigating to be allowed")
	}
}

func TestCanTransition_InvalidPath(t *testing.T) {
	if CanTransition(StateResolved, StateInvestigating) {
		t.Fatal("expected resolved -> investigating to be rejected")
	}
}

func TestCanTransition_SameStateAllowed(t *testing.T) {
	if !CanTransition(StateInvestigating, StateInvestigating) {
		t.Fatal("expected same-state transition to be allowed")
	}
}

func TestCanTransition_Phase10ValidationPath(t *testing.T) {
	// Phase 10 step 4.1: execution must flow through validation before closure.
	if !CanTransition(StateExecuting, StateValidating) {
		t.Fatal("expected executing -> validating to be allowed")
	}
	if !CanTransition(StateValidating, StateResolved) {
		t.Fatal("expected validating -> resolved to be allowed")
	}
	if !CanTransition(StateValidating, StateFailed) {
		t.Fatal("expected validating -> failed to be allowed")
	}
}

func TestCanTransition_RejectsExecutingDirectlyToResolved(t *testing.T) {
	// Phase 10 step 4.1 task 3: resolution requires post-remediation health evidence,
	// so an executing incident cannot close without first entering validating.
	if CanTransition(StateExecuting, StateResolved) {
		t.Fatal("expected executing -> resolved to be rejected (must validate first)")
	}
}

func TestCanTransition_ExecutingToFailedShortcut(t *testing.T) {
	// Failure shortcut: a remediation that did not complete can fail directly.
	if !CanTransition(StateExecuting, StateFailed) {
		t.Fatal("expected executing -> failed to be allowed")
	}
}

func TestValidateTransition_ReturnsTypedError(t *testing.T) {
	err := validateTransition(StateResolved, StateInvestigating)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}
