package agentbuilder

import "fmt"

// IncidentSummary is the strict, deterministic shape of the final incident report
// produced by Agent Builder / Gemini once an incident lifecycle is complete.
//
// It is grounded in the stored incident record and distinguishes between a
// successful recovery and a failed remediation outcome. Evidence and actions are
// arrays so the dashboard can render them as clean, scannable lists.
type IncidentSummary struct {
	RootCause    string   `json:"rootCause"`
	Evidence     []string `json:"evidence"`
	ActionsTaken []string `json:"actionsTaken"`
	Result       string   `json:"result"`
	// OperatorSummary is an optional one-line narrative for at-a-glance reading.
	OperatorSummary string `json:"operatorSummary,omitempty"`
}

// Validate checks the required invariants for an IncidentSummary.
//
// Required fields are rootCause, evidence, actionsTaken, and result. operatorSummary
// is optional. Keeping this strict ensures generation and rendering share one stable
// contract and that an empty or malformed summary never reaches the dashboard.
func (s IncidentSummary) Validate() error {
	if s.RootCause == "" {
		return ErrEmptyRootCause
	}
	if len(s.Evidence) == 0 {
		return ErrEmptyEvidence
	}
	for _, e := range s.Evidence {
		if e == "" {
			return ErrBlankEvidenceEntry
		}
	}
	if len(s.ActionsTaken) == 0 {
		return ErrEmptyActionsTaken
	}
	for _, a := range s.ActionsTaken {
		if a == "" {
			return ErrBlankActionEntry
		}
	}
	if s.Result == "" {
		return ErrEmptyResult
	}
	return nil
}

// IncidentSummary validation errors.
var (
	ErrEmptyRootCause     = fmt.Errorf("rootCause must be non-empty")
	ErrEmptyEvidence      = fmt.Errorf("evidence must be non-empty")
	ErrBlankEvidenceEntry = fmt.Errorf("evidence entries must be non-empty")
	ErrEmptyActionsTaken  = fmt.Errorf("actionsTaken must be non-empty")
	ErrBlankActionEntry   = fmt.Errorf("actionsTaken entries must be non-empty")
	ErrEmptyResult        = fmt.Errorf("result must be non-empty")
)
