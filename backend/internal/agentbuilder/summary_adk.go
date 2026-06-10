package agentbuilder

import (
	"strings"
)

// summaryTaskName identifies the Phase 11 final-summary workflow to the ADK boundary,
// matching the Phase 7 convention (phase7_investigation).
const summaryTaskName = "phase11_summary"

// ADKSummaryRequestPayload is the ADK-facing envelope for the final-summary workflow.
// It carries the rendered prompt, trace metadata, and the structured summary request so
// the workflow can reason from compact facts and the backend can archive what was sent.
type ADKSummaryRequestPayload struct {
	Task           string              `json:"task"`
	Prompt         string              `json:"prompt"`
	Metadata       ADKRequestMetadata  `json:"metadata"`
	SummaryRequest FinalSummaryRequest `json:"summary_request"`
}

// BuildSummaryADKRequestPayload creates an ADK request payload for the final-summary
// workflow from a FinalSummaryRequest, reusing the Phase 7 ADK request pattern
// (BuildADKRequestPayload). The idempotency token lets retries dedupe a regenerated
// summary.
func BuildSummaryADKRequestPayload(req FinalSummaryRequest, idempotencyToken string) (ADKSummaryRequestPayload, error) {
	prompt, err := BuildSummaryPrompt(req)
	if err != nil {
		return ADKSummaryRequestPayload{}, err
	}

	return ADKSummaryRequestPayload{
		Task:   summaryTaskName,
		Prompt: prompt,
		Metadata: ADKRequestMetadata{
			IncidentID:       req.IncidentID,
			DeviceID:         req.DeviceID,
			RequestID:        req.RequestID,
			IdempotencyToken: strings.TrimSpace(idempotencyToken),
		},
		SummaryRequest: req,
	}, nil
}
