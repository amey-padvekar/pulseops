package incidents

// FinalSummary is the incident-local record of the Phase 11 closing report. It mirrors
// the agentbuilder.IncidentSummary shape but lives in the incidents package so the store
// never imports the agentbuilder package (which already imports incidents — importing back
// would be a cycle). Callers map the validated agentbuilder result into this type before
// persisting, the same pattern used for RemediationActionResult and SaveInvestigationResult.
//
// It is kept deliberately separate from the AI investigation output (ProbableCause,
// Summary) and the validation evidence (LastValidationSnapshot) so the closing report
// reads as its own first-class artifact.
type FinalSummary struct {
	RootCause       string   `json:"rootCause"`
	Evidence        []string `json:"evidence"`
	ActionsTaken    []string `json:"actionsTaken"`
	Result          string   `json:"result"`
	OperatorSummary string   `json:"operatorSummary,omitempty"`
}

// Summary generation status values persisted on the incident (Phase 11 step 4.6). They
// mirror the canonical agentbuilder.SummaryStatus* constants; the string values must stay
// in sync across the two packages (incidents cannot import agentbuilder).
const (
	SummaryStatusPending   = "pending"
	SummaryStatusGenerated = "generated"
	SummaryStatusFallback  = "fallback"
	SummaryStatusFailed    = "failed"
)
