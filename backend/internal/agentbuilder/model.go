package agentbuilder

import (
	"encoding/json"
	"time"
)

// AgentBuilderRequest is the structured context payload sent to Agent Builder.
type AgentBuilderRequest struct {
	SchemaVersion       string              `json:"schemaVersion,omitempty"`
	RequestID           string              `json:"requestId"`
	IncidentID          string              `json:"incidentId"`
	DeviceID            string              `json:"deviceId"`
	ServiceName         string              `json:"serviceName"`
	IncidentState       string              `json:"incidentState"`
	Severity            string              `json:"severity"`
	RequestedAt         time.Time           `json:"requestedAt"`
	TimeWindow          TimeWindow          `json:"timeWindow"`
	TelemetrySnapshot   TelemetrySnapshot   `json:"telemetrySnapshot"`
	RecentLogs          []string            `json:"recentLogs"`
	IncidentSummary     string              `json:"incidentSummary"`
	AvailableActions    []ActionOption      `json:"availableActions"`
	ElasticContextHints ElasticContextHints `json:"elasticContextHints"`
}

// TimeWindow describes the incident investigation window.
type TimeWindow struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// TelemetrySnapshot captures the latest device telemetry values.
type TelemetrySnapshot struct {
	Timestamp        time.Time `json:"timestamp"`
	ServiceStatus    string    `json:"serviceStatus"`
	NetworkReachable bool      `json:"networkReachable"`
	CPUUsage         float64   `json:"cpuUsage"`
	MemoryUsage      float64   `json:"memoryUsage"`
	Heartbeat        bool      `json:"heartbeat"`
}

// ElasticContextHints provides query hints for Elastic-backed retrieval.
type ElasticContextHints struct {
	DeviceID           string    `json:"deviceId"`
	IncidentID         string    `json:"incidentId"`
	ServiceName        string    `json:"serviceName"`
	TimeRangeStart     time.Time `json:"timeRangeStart"`
	TimeRangeEnd       time.Time `json:"timeRangeEnd"`
	IndexPatterns      []string  `json:"indexPatterns"`
	RecommendedQueries []string  `json:"recommendedQueries"`
}

// ActionOption describes an allowed remediation action.
type ActionOption struct {
	ActionID         string   `json:"actionId"`
	Target           string   `json:"target,omitempty"`
	Description      string   `json:"description,omitempty"`
	AllowedPlatforms []string `json:"allowedPlatforms,omitempty"`
}

// AgentBuilderResponse captures the baseline response envelope from Agent Builder.
type AgentBuilderResponse struct {
	RequestID  string          `json:"requestId"`
	TraceID    string          `json:"traceId,omitempty"`
	Status     ResponseStatus  `json:"status"`
	ReceivedAt time.Time       `json:"receivedAt"`
	RawPayload json.RawMessage `json:"rawPayload,omitempty"`
}

// ResponseStatus distinguishes transport success from workflow state.
type ResponseStatus struct {
	Transport string `json:"transport"`
	Workflow  string `json:"workflow"`
}
