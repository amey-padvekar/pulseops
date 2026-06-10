package safety

import (
	"fmt"
	"strings"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

func ValidateInvestigationResult(res domain.InvestigationResult, allowed []domain.ActionOption) error {
	if strings.TrimSpace(res.ProbableCause) == "" {
		return fmt.Errorf("probableCause must be non-empty")
	}
	if res.Confidence < 0.0 || res.Confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0")
	}
	if strings.TrimSpace(res.Summary) == "" {
		return fmt.Errorf("summary must be non-empty")
	}

	if containsUnsafeText(res.ProbableCause) || containsUnsafeText(res.Summary) {
		return fmt.Errorf("unsafe text detected")
	}

	if len(res.ValidationSteps) == 0 {
		return fmt.Errorf("validationSteps must be non-empty")
	}
	for _, step := range res.ValidationSteps {
		if strings.TrimSpace(step) == "" || containsUnsafeText(step) {
			return fmt.Errorf("validationSteps contains invalid content")
		}
	}

	allowedSet := map[string]struct{}{}
	for _, item := range allowed {
		if strings.TrimSpace(item.ActionID) == "" {
			continue
		}
		allowedSet[item.ActionID] = struct{}{}
	}

	for _, action := range res.RecommendedActions {
		if strings.TrimSpace(action.ActionID) == "" {
			return fmt.Errorf("recommendedActions.actionId must be non-empty")
		}
		if len(allowedSet) > 0 {
			if _, ok := allowedSet[action.ActionID]; !ok {
				return fmt.Errorf("recommendedActions contains unapproved actionId")
			}
		}
		if containsUnsafeText(action.Target) || containsUnsafeText(action.Reason) {
			return fmt.Errorf("recommendedActions contains unsafe content")
		}
	}

	return nil
}

func containsUnsafeText(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.ContainsAny(s, ";|$`\\") {
		return true
	}
	if strings.Contains(low, "&&") || strings.Contains(low, "sudo ") || strings.Contains(low, "rm ") || strings.Contains(low, "curl ") || strings.Contains(low, "wget ") || strings.Contains(low, "bash") || strings.Contains(low, "sh ") || strings.Contains(low, "powershell") || strings.Contains(low, "cmd.exe") {
		return true
	}
	return false
}
