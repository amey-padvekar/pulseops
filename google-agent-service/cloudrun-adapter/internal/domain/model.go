package domain

type Metadata struct {
	IncidentID       string `json:"incident_id"`
	DeviceID         string `json:"device_id"`
	RequestID        string `json:"request_id"`
	IdempotencyToken string `json:"idempotency_token,omitempty"`
}

type ActionOption struct {
	ActionID         string   `json:"actionId"`
	Target           string   `json:"target,omitempty"`
	Description      string   `json:"description,omitempty"`
	AllowedPlatforms []string `json:"allowedPlatforms,omitempty"`
}

type ElasticContextHints struct {
	DeviceID           string   `json:"deviceId,omitempty"`
	IncidentID         string   `json:"incidentId,omitempty"`
	ServiceName        string   `json:"serviceName,omitempty"`
	TimeRangeStart     string   `json:"timeRangeStart,omitempty"`
	TimeRangeEnd       string   `json:"timeRangeEnd,omitempty"`
	IndexPatterns      []string `json:"indexPatterns,omitempty"`
	RecommendedQueries []string `json:"recommendedQueries,omitempty"`
}

type InvestigateRequest struct {
	Task                string              `json:"task"`
	Prompt              string              `json:"prompt"`
	Metadata            Metadata            `json:"metadata"`
	ElasticContextHints ElasticContextHints `json:"elastic_context_hints,omitempty"`
	AvailableActions    []ActionOption      `json:"available_actions"`
	EvidenceSummary     string              `json:"evidence_summary,omitempty"`
}

type RecommendedAction struct {
	ActionID string `json:"actionId"`
	Target   string `json:"target,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type InvestigationResult struct {
	ProbableCause      string              `json:"probableCause"`
	Confidence         float64             `json:"confidence"`
	RecommendedActions []RecommendedAction `json:"recommendedActions"`
	ValidationSteps    []string            `json:"validationSteps"`
	Summary            string              `json:"summary"`
}

type Status struct {
	Transport string `json:"transport"`
	Workflow  string `json:"workflow"`
}

type InvestigateResponse struct {
	RequestID string               `json:"request_id"`
	TraceID   string               `json:"trace_id"`
	Status    Status               `json:"status"`
	Result    *InvestigationResult `json:"result,omitempty"`
	Error     *ErrorEnvelope       `json:"error,omitempty"`
}

type ErrorEnvelope struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
