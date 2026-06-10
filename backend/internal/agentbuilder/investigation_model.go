package agentbuilder

import "fmt"

// InvestigationResult is the strict shape returned by Agent Builder / Gemini reasoning.
type InvestigationResult struct {
    ProbableCause      string              `json:"probableCause"`
    Confidence         float64             `json:"confidence"` // 0.0 - 1.0
    RecommendedActions []RecommendedAction `json:"recommendedActions"`
    ValidationSteps    []string            `json:"validationSteps"`
    Summary            string              `json:"summary"`
}

// RecommendedAction represents a single safe remediation suggestion.
type RecommendedAction struct {
    ActionID string `json:"actionId"`
    Target   string `json:"target,omitempty"`
    Reason   string `json:"reason,omitempty"`
}

// Approved action IDs
const (
    ActionRestartService = "restart_service"
    ActionFlushDNS       = "flush_dns"
    ActionReconnectVPN   = "reconnect_vpn"
)

var approvedActionIDs = map[string]struct{}{
    ActionRestartService: {},
    ActionFlushDNS:       {},
    ActionReconnectVPN:   {},
}

// IsApprovedActionID returns true if the provided action id is in the whitelist.
func IsApprovedActionID(id string) bool {
    _, ok := approvedActionIDs[id]
    return ok
}

// Validate checks basic invariants for the InvestigationResult.
// This is intentionally lightweight; parser.go will perform stricter validation and error reporting.
func (r InvestigationResult) Validate() error {
    if r.ProbableCause == "" {
        return ErrEmptyProbableCause
    }
    if r.Confidence < 0.0 || r.Confidence > 1.0 {
        return ErrConfidenceOutOfRange
    }
    if len(r.ValidationSteps) == 0 {
        return ErrEmptyValidationSteps
    }
    for _, a := range r.RecommendedActions {
        if !IsApprovedActionID(a.ActionID) {
            return ErrUnapprovedActionID
        }
    }
    return nil
}

// Package-level errors
var (
    ErrEmptyProbableCause   = fmt.Errorf("probableCause must be non-empty")
    ErrConfidenceOutOfRange = fmt.Errorf("confidence must be between 0.0 and 1.0")
    ErrEmptyValidationSteps = fmt.Errorf("validationSteps must be non-empty")
    ErrUnapprovedActionID   = fmt.Errorf("recommendedActions contains unapproved actionId")
)
