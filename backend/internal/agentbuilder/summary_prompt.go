package agentbuilder

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// FinalSummaryPromptTemplate is the Gemini/Agent Builder prompt used to produce the final
// incident report (Phase 11). It mirrors the Phase 7 investigation prompt conventions:
// the backend supplies compact, pre-selected facts and the workflow must return strictly
// parseable JSON matching the IncidentSummary shape (see summary_model.go).
//
// Template placeholders (all pre-rendered, deterministic blocks):
//   - {{.IncidentID}}
//   - {{.DeviceID}}
//   - {{.Service}}
//   - {{.Outcome}}        (resolved | failed | incomplete)
//   - {{.Diagnosis}}      (prior AI investigation, may be "None recorded.")
//   - {{.Actions}}        (recommended / approved / executed actions)
//   - {{.OutcomeDetail}}  (validation status and reason)
//   - {{.Evidence}}       (bullet list of selected evidence snippets)
const FinalSummaryPromptTemplate = `You are an incident report writer. Produce a final, operator-facing incident summary using ONLY the facts provided below. Do not introduce any cause, action, or outcome that is not present in the record.

Incident:
- incidentId: {{.IncidentID}}
- deviceId: {{.DeviceID}}
- service: {{.Service}}
- outcome: {{.Outcome}}

Diagnosis (what the prior AI investigation believed):
{{.Diagnosis}}

Actions (what was recommended, approved, and executed):
{{.Actions}}

Outcome detail (what actually happened during validation):
{{.OutcomeDetail}}

Evidence (facts only):
{{.Evidence}}

Rules (follow exactly):
1) Reason ONLY from the facts above. Do not invent telemetry, logs, causes, or actions.
2) Preserve the factual distinction between the diagnosis (what was believed), the actions taken (what was done), and the outcome (what actually happened). Never blur them together.
3) 'result' and 'operatorSummary' MUST match the stated outcome. If outcome is "failed", describe a failed remediation and do NOT claim recovery. If outcome is "resolved", state that recovery was confirmed.
4) Keep wording concise and operator-facing: 'rootCause', 'result', and 'operatorSummary' are one short sentence each.
5) 'evidence' and 'actionsTaken' are arrays of short factual strings drawn from the record. If no remediation action was executed, 'actionsTaken' must contain a single entry stating that explicitly.
6) Avoid speculative language ("might", "possibly", "likely", "appears to") beyond what the record supports.
7) Output MUST be valid JSON strictly matching the 'IncidentSummary' schema below. Output JSON only, with no surrounding prose.

Output example (JSON only):
{
  "rootCause": "short factual cause",
  "evidence": ["short factual evidence sentence", "another"],
  "actionsTaken": ["Approved action: restart_service for <target>.", "Agent executed the restart successfully."],
  "result": "one-sentence final outcome matching '` + "{{.Outcome}}" + `'",
  "operatorSummary": "one-sentence at-a-glance narrative"
}
`

// summaryPromptData holds the pre-rendered blocks injected into FinalSummaryPromptTemplate.
type summaryPromptData struct {
	IncidentID    string
	DeviceID      string
	Service       string
	Outcome       string
	Diagnosis     string
	Actions       string
	OutcomeDetail string
	Evidence      string
}

const summaryEmptyBlock = "None recorded."

// BuildSummaryPrompt renders the final-summary prompt from a FinalSummaryRequest. The
// request is assumed already bounded and compacted (step 4.2 / 4.7); this only formats it
// into deterministic prompt blocks so the workflow produces a consistent closing artifact.
func BuildSummaryPrompt(req FinalSummaryRequest) (string, error) {
	tmpl, err := template.New("agentbuilder_summary_prompt").Parse(FinalSummaryPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("parse summary prompt template: %w", err)
	}

	data := summaryPromptData{
		IncidentID:    orPlaceholder(req.IncidentID, "unknown"),
		DeviceID:      orPlaceholder(req.DeviceID, "unknown"),
		Service:       orPlaceholder(req.ServiceName, "unknown"),
		Outcome:       orPlaceholder(req.Outcome, OutcomeIncomplete),
		Diagnosis:     renderDiagnosisBlock(req),
		Actions:       renderActionsBlock(req),
		OutcomeDetail: renderOutcomeDetailBlock(req),
		Evidence:      renderEvidenceBlock(req.Evidence),
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("render summary prompt template: %w", err)
	}
	return buf.String(), nil
}

func renderDiagnosisBlock(req FinalSummaryRequest) string {
	var lines []string
	if c := strings.TrimSpace(req.ProbableCause); c != "" {
		if req.Confidence > 0 {
			lines = append(lines, fmt.Sprintf("- probable cause: %s (confidence %.2f)", c, req.Confidence))
		} else {
			lines = append(lines, "- probable cause: "+c)
		}
	}
	if s := strings.TrimSpace(req.DiagnosisSummary); s != "" {
		lines = append(lines, "- investigation summary: "+s)
	}
	return joinOrEmpty(lines)
}

func renderActionsBlock(req FinalSummaryRequest) string {
	var lines []string
	for _, a := range req.RecommendedActions {
		id := strings.TrimSpace(a.ActionID)
		if id == "" {
			continue
		}
		if t := strings.TrimSpace(a.Target); t != "" {
			lines = append(lines, fmt.Sprintf("- recommended: %s for %s", id, t))
		} else {
			lines = append(lines, "- recommended: "+id)
		}
	}
	if len(req.ApprovedActions) > 0 {
		lines = append(lines, "- approved: "+strings.Join(req.ApprovedActions, ", "))
	}
	if s := strings.TrimSpace(req.RemediationStatus); s != "" {
		lines = append(lines, "- execution status: "+s)
	}
	for _, r := range req.ExecutionResults {
		id := strings.TrimSpace(r.ActionID)
		if id == "" {
			continue
		}
		line := fmt.Sprintf("- executed: %s -> %s", id, orPlaceholder(r.Status, "unknown"))
		if d := strings.TrimSpace(r.Detail); d != "" {
			line += " (" + d + ")"
		}
		lines = append(lines, line)
	}
	return joinOrEmpty(lines)
}

func renderOutcomeDetailBlock(req FinalSummaryRequest) string {
	var lines []string
	lines = append(lines, "- outcome: "+orPlaceholder(req.Outcome, OutcomeIncomplete))
	if s := strings.TrimSpace(req.ValidationStatus); s != "" {
		lines = append(lines, "- validation status: "+s)
	}
	if r := strings.TrimSpace(req.ValidationReason); r != "" {
		lines = append(lines, "- validation reason: "+r)
	}
	if r := strings.TrimSpace(req.ValidationFailureReason); r != "" {
		lines = append(lines, "- failure reason: "+r)
	}
	return joinOrEmpty(lines)
}

func renderEvidenceBlock(evidence []string) string {
	var lines []string
	for _, e := range evidence {
		if t := strings.TrimSpace(e); t != "" {
			lines = append(lines, "- "+t)
		}
	}
	return joinOrEmpty(lines)
}

func joinOrEmpty(lines []string) string {
	if len(lines) == 0 {
		return summaryEmptyBlock
	}
	return strings.Join(lines, "\n")
}

func orPlaceholder(s, placeholder string) string {
	if t := strings.TrimSpace(s); t != "" {
		return t
	}
	return placeholder
}
