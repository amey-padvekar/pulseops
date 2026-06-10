package agentbuilder

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseError wraps parsing/validation errors and preserves the raw payload
// returned by the Agent Builder workflow to aid debugging.
type ParseError struct {
	Err        error
	RawPayload []byte
}

func (e ParseError) Error() string {
	return fmt.Sprintf("%v: %s", e.Err, truncate(e.RawPayload, 200))
}

// Unwrap exposes the wrapped cause so callers can match it with errors.Is/errors.As
// while still preserving the raw payload on the ParseError.
func (e ParseError) Unwrap() error {
	return e.Err
}

// truncate returns a shortened string representation of the payload for errors.
func truncate(b []byte, n int) string {
	if len(b) == 0 {
		return "<empty>"
	}
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ErrUnsafeContent is returned when the parsed result contains disallowed shell-like content.
var ErrUnsafeContent = fmt.Errorf("parsed result contains disallowed shell-like content")

// ParseInvestigationResult parses raw JSON into an InvestigationResult and performs
// strict validation per Phase 7 requirements. If parsing or validation fails, a
// ParseError is returned that preserves the raw payload.
func ParseInvestigationResult(raw []byte) (InvestigationResult, error) {
	return ParseInvestigationResultWithAllowedActions(raw, nil)
}

// ParseInvestigationResultWithAllowedActions parses and validates investigation
// payloads, additionally enforcing that actions are in the supplied catalog when
// `allowedActions` is non-empty.
func ParseInvestigationResultWithAllowedActions(raw []byte, allowedActions []ActionOption) (InvestigationResult, error) {
	if len(raw) == 0 {
		return InvestigationResult{}, ParseError{Err: fmt.Errorf("empty payload"), RawPayload: raw}
	}

	var res InvestigationResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return InvestigationResult{}, ParseError{Err: fmt.Errorf("invalid JSON: %w", err), RawPayload: raw}
	}

	// Basic structural validation using model.Validate()
	if err := res.Validate(); err != nil {
		return InvestigationResult{}, ParseError{Err: err, RawPayload: raw}
	}

	// Safety boundary: only RecommendedActions[].Target reaches execution — it is passed
	// as the -Name argument to Restart-Service on the endpoint, and actionId is validated
	// against the allow-list separately. So Target is shell-checked strictly here.
	//
	// probableCause / summary / validationSteps / reason are display-only prose rendered as
	// escaped text in the dashboard; they are intentionally NOT shell-checked. Checking them
	// falsely rejected legitimate Gemini diagnoses that mention Windows paths (C:\...), '$',
	// or backticks, even though that text is never executed.
	for _, a := range res.RecommendedActions {
		if containsUnsafeText(a.Target) {
			return InvestigationResult{}, ParseError{Err: ErrUnsafeContent, RawPayload: raw}
		}
	}

	// Validate recommended actions against allowed catalog (if provided)
	if err := ValidateRecommendedActions(res.RecommendedActions, allowedActions); err != nil {
		return InvestigationResult{}, ParseError{Err: err, RawPayload: raw}
	}

	return res, nil
}

// containsUnsafeText performs a lightweight check for shell-like tokens.
func containsUnsafeText(s string) bool {
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	// simple heuristics: shell metacharacters or common command words
	if strings.ContainsAny(s, ";|$`\\") {
		return true
	}
	if strings.Contains(low, "&&") || strings.Contains(low, "sudo ") || strings.Contains(low, "rm ") || strings.Contains(low, "curl ") || strings.Contains(low, "wget ") || strings.Contains(low, "bash") || strings.Contains(low, "sh ") {
		return true
	}
	return false
}
