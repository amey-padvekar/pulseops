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

func TestValidateTransition_ReturnsTypedError(t *testing.T) {
	err := validateTransition(StateResolved, StateInvestigating)
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}
