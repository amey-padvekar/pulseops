package safety

import (
	"testing"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

func TestValidateInvestigationResult_RejectsUnknownAction(t *testing.T) {
	res := domain.InvestigationResult{
		ProbableCause:      "service stopped",
		Confidence:         0.8,
		RecommendedActions: []domain.RecommendedAction{{ActionID: "dangerous", Target: "svc", Reason: "x"}},
		ValidationSteps:    []string{"check service"},
		Summary:            "summary",
	}

	err := ValidateInvestigationResult(res, []domain.ActionOption{{ActionID: "restart_service"}})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateInvestigationResult_RejectsUnsafeText(t *testing.T) {
	res := domain.InvestigationResult{
		ProbableCause:      "run rm -rf /",
		Confidence:         0.8,
		RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service", Target: "svc", Reason: "safe"}},
		ValidationSteps:    []string{"check service"},
		Summary:            "summary",
	}

	err := ValidateInvestigationResult(res, []domain.ActionOption{{ActionID: "restart_service"}})
	if err == nil {
		t.Fatalf("expected unsafe-text validation error")
	}
}
