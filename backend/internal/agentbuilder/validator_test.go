package agentbuilder

import "testing"

func TestValidateRecommendedActions_AllowedCatalog(t *testing.T) {
    actions := []RecommendedAction{{ActionID: ActionRestartService, Target: "svc"}}
    allowed := []ActionOption{{ActionID: ActionRestartService, Target: "svc"}}

    if err := ValidateRecommendedActions(actions, allowed); err != nil {
        t.Fatalf("unexpected validation error: %v", err)
    }
}

func TestValidateRecommendedActions_TargetMismatch(t *testing.T) {
    actions := []RecommendedAction{{ActionID: ActionRestartService, Target: "svc-a"}}
    allowed := []ActionOption{{ActionID: ActionRestartService, Target: "svc-b"}}

    if err := ValidateRecommendedActions(actions, allowed); err == nil {
        t.Fatalf("expected target mismatch error")
    }
}

func TestValidateRecommendedActions_GlobalWhitelist(t *testing.T) {
    actions := []RecommendedAction{{ActionID: ActionFlushDNS, Target: ""}}

    // no allowed catalog provided; should validate against global whitelist only
    if err := ValidateRecommendedActions(actions, nil); err != nil {
        t.Fatalf("unexpected validation error: %v", err)
    }
}
