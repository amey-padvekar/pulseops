package agentbuilder

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseFinalSummary parses a raw Agent Builder summary payload into a clean, validated
// IncidentSummary (Phase 11 step 4.5). It accepts either a bare IncidentSummary JSON
// object or one nested under a common envelope key, sanitizes the content (trims fields,
// drops blank array entries), and enforces the acceptance rules:
//   - rootCause is non-empty
//   - evidence contains at least one item
//   - result is non-empty
//   - actionsTaken contains at least one item when remediation occurred
//
// On any failure it returns a ParseError that preserves the raw payload for debugging
// (task 3). Use FallbackSummary when the model times out or returns unusable output.
func ParseFinalSummary(raw []byte, remediationOccurred bool) (IncidentSummary, error) {
	if len(raw) == 0 {
		return IncidentSummary{}, ParseError{Err: fmt.Errorf("empty payload"), RawPayload: raw}
	}

	payload := extractSummaryPayload(raw)

	var s IncidentSummary
	if err := json.Unmarshal(payload, &s); err != nil {
		return IncidentSummary{}, ParseError{Err: fmt.Errorf("invalid JSON: %w", err), RawPayload: raw}
	}

	s = sanitizeSummary(s)

	if err := validateFinalSummary(s, remediationOccurred); err != nil {
		return IncidentSummary{}, ParseError{Err: err, RawPayload: raw}
	}

	return s, nil
}

// sanitizeSummary trims all string fields and drops blank array entries so stored content
// is clean and renders predictably.
func sanitizeSummary(s IncidentSummary) IncidentSummary {
	return IncidentSummary{
		RootCause:       strings.TrimSpace(s.RootCause),
		Evidence:        trimmedNonEmpty(s.Evidence),
		ActionsTaken:    trimmedNonEmpty(s.ActionsTaken),
		Result:          strings.TrimSpace(s.Result),
		OperatorSummary: strings.TrimSpace(s.OperatorSummary),
	}
}

// validateFinalSummary enforces the step 4.5 acceptance rules. actionsTaken is required
// only when remediation actually occurred; an incident that failed before any action ran
// may legitimately have no actions taken.
func validateFinalSummary(s IncidentSummary, remediationOccurred bool) error {
	if s.RootCause == "" {
		return ErrEmptyRootCause
	}
	if len(s.Evidence) == 0 {
		return ErrEmptyEvidence
	}
	if s.Result == "" {
		return ErrEmptyResult
	}
	if remediationOccurred && len(s.ActionsTaken) == 0 {
		return ErrEmptyActionsTaken
	}
	return nil
}

// extractSummaryPayload returns the IncidentSummary-shaped JSON from raw, unwrapping a
// common envelope key when the payload is nested (mirrors extractInvestigationPayload).
// It falls back to the original payload when no nested summary is found.
func extractSummaryPayload(raw []byte) []byte {
	if isFinalSummaryJSON(raw) {
		return raw
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw
	}

	for _, key := range []string{"summary", "finalSummary", "result", "output", "response"} {
		nested, ok := decoded[key]
		if !ok || len(nested) == 0 {
			continue
		}
		if isFinalSummaryJSON(nested) {
			return nested
		}
	}

	return raw
}

// isFinalSummaryJSON reports whether raw looks like a usable IncidentSummary (the two
// always-required scalar fields are present and non-empty).
func isFinalSummaryJSON(raw []byte) bool {
	var probe struct {
		RootCause string `json:"rootCause"`
		Result    string `json:"result"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	return strings.TrimSpace(probe.RootCause) != "" && strings.TrimSpace(probe.Result) != ""
}

// RemediationOccurred reports whether any remediation was executed for an incident, based
// on the assembled summary request. Used to decide whether actionsTaken is required.
func RemediationOccurred(req FinalSummaryRequest) bool {
	return len(req.ExecutionResults) > 0 ||
		len(req.ApprovedActions) > 0 ||
		strings.TrimSpace(req.RemediationStatus) != ""
}

// FallbackSummary synthesizes a deterministic, record-grounded IncidentSummary directly
// from the assembled request (Phase 11 step 4.5 task 4). It is used when summary
// generation times out or returns malformed output, so the dashboard still shows a usable,
// non-speculative closing artifact built entirely from stored incident data. The returned
// summary always satisfies validateFinalSummary.
func FallbackSummary(req FinalSummaryRequest) IncidentSummary {
	rootCause := firstNonEmpty(req.ProbableCause, req.Reason, "Root cause could not be determined from the available record.")

	evidence := trimmedNonEmpty(req.Evidence)
	if len(evidence) == 0 {
		evidence = []string{fallbackEvidenceLine(req)}
	}

	actions := fallbackActions(req)

	return IncidentSummary{
		RootCause:       rootCause,
		Evidence:        evidence,
		ActionsTaken:    actions,
		Result:          fallbackResult(req),
		OperatorSummary: fallbackOperatorSummary(req, rootCause),
	}
}

func fallbackActions(req FinalSummaryRequest) []string {
	var actions []string
	for _, r := range req.ExecutionResults {
		id := strings.TrimSpace(r.ActionID)
		if id == "" {
			continue
		}
		actions = append(actions, fmt.Sprintf("Executed %s: %s.", id, orPlaceholder(r.Status, "status unknown")))
	}
	if len(actions) == 0 {
		for _, a := range req.ApprovedActions {
			if t := strings.TrimSpace(a); t != "" {
				actions = append(actions, "Approved action: "+t+".")
			}
		}
	}
	if len(actions) == 0 {
		actions = []string{"No remediation actions were executed."}
	}
	return actions
}

func fallbackResult(req FinalSummaryRequest) string {
	switch req.Outcome {
	case OutcomeResolved:
		return "Service health recovered and the incident was resolved."
	case OutcomeFailed:
		if r := strings.TrimSpace(req.ValidationFailureReason); r != "" {
			return "Remediation did not restore health: " + r
		}
		return "Remediation did not restore health and the incident was marked failed."
	default:
		return "The incident did not reach a confirmed resolved or failed outcome."
	}
}

func fallbackOperatorSummary(req FinalSummaryRequest, rootCause string) string {
	// Strip trailing punctuation from the cause so it joins the next clause cleanly,
	// regardless of how the stored probable cause was phrased.
	clause := strings.TrimRight(lowerFirst(rootCause), ". ")
	switch req.Outcome {
	case OutcomeResolved:
		return "Detected " + clause + "; the approved remediation ran and recovery was confirmed."
	case OutcomeFailed:
		return "Detected " + clause + "; remediation was attempted, but recovery was not confirmed."
	default:
		return "Incident summary generated from the stored record; outcome was not confirmed."
	}
}

func fallbackEvidenceLine(req FinalSummaryRequest) string {
	if r := strings.TrimSpace(req.ValidationReason); r != "" {
		return "Validation: " + r
	}
	if r := strings.TrimSpace(req.ValidationFailureReason); r != "" {
		return "Validation failure: " + r
	}
	if r := strings.TrimSpace(req.Reason); r != "" {
		return "Detection: " + r
	}
	return fmt.Sprintf("Incident %s reached outcome %s.", orPlaceholder(req.IncidentID, "unknown"), orPlaceholder(req.Outcome, OutcomeIncomplete))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
