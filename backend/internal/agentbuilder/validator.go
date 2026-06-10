package agentbuilder

import (
    "fmt"
    "strings"
)

// ValidateRecommendedActions ensures each recommended action uses an approved
// actionId and, when a catalog of allowed actions is supplied, that the action
// appears in that catalog and its target (if specified) matches the allowed
// target rule.
func ValidateRecommendedActions(actions []RecommendedAction, allowed []ActionOption) error {
    // If no allowed catalog provided, only validate against the global whitelist.
    if len(allowed) == 0 {
        for _, a := range actions {
            if !IsApprovedActionID(a.ActionID) {
                return fmt.Errorf("unapproved actionId: %s", a.ActionID)
            }
        }
        return nil
    }

    // Build lookup for allowed actions by actionId
    allowedMap := make(map[string]ActionOption, len(allowed))
    for _, o := range allowed {
        allowedMap[o.ActionID] = o
    }

    for _, a := range actions {
        // Must be globally approved as well
        if !IsApprovedActionID(a.ActionID) {
            return fmt.Errorf("unapproved actionId: %s", a.ActionID)
        }

        opt, ok := allowedMap[a.ActionID]
        if !ok {
            return fmt.Errorf("actionId not permitted in this request: %s", a.ActionID)
        }

        // If the allowed option includes a target constraint, ensure it matches.
        // Compared trimmed + case-insensitively: service names are case-insensitive on
        // Windows, and the LLM may vary casing/whitespace when echoing the target.
        if opt.Target != "" && !strings.EqualFold(strings.TrimSpace(a.Target), strings.TrimSpace(opt.Target)) {
            return fmt.Errorf("actionId %s target mismatch: got %q want %q", a.ActionID, a.Target, opt.Target)
        }
    }

    return nil
}
