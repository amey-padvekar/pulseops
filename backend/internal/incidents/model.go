package incidents

import "time"

// IncidentState describes where an incident is in the lifecycle.
type IncidentState string

const (
	StateHealthy          IncidentState = "healthy"
	StateDetected         IncidentState = "detected"
	StateInvestigating    IncidentState = "investigating"
	StateAwaitingApproval IncidentState = "awaiting_approval"
	StateApproved         IncidentState = "approved"
	StateExecuting        IncidentState = "executing"
	StateValidating       IncidentState = "validating"
	StateResolved         IncidentState = "resolved"
	StateFailed           IncidentState = "failed"
)

// Severity represents incident urgency.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Incident is the canonical incident record used by store, API, and websocket payloads.
type Incident struct {
	IncidentID    string        `json:"incidentId"`
	DeviceID      string        `json:"deviceId"`
	ServiceName   string        `json:"serviceName"`
	ServiceStatus string        `json:"serviceStatus"`
	State         IncidentState `json:"state"`
	Severity      Severity      `json:"severity"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     time.Time     `json:"updatedAt"`
	DetectedAt    time.Time     `json:"detectedAt"`
	LastSeenAt    time.Time     `json:"lastSeenAt"`
	Reason        string        `json:"reason"`
	Active        bool          `json:"active"`
}

// NewIncident creates a detected, active incident using the current UTC time.
func NewIncident(
	incidentID string,
	deviceID string,
	serviceName string,
	serviceStatus string,
	severity Severity,
	reason string,
) Incident {
	return NewIncidentAt(
		incidentID,
		deviceID,
		serviceName,
		serviceStatus,
		severity,
		reason,
		time.Now().UTC(),
	)
}

// NewIncidentAt creates a detected, active incident using the supplied timestamp.
// The timestamp is normalized to UTC for consistent persistence and API output.
func NewIncidentAt(
	incidentID string,
	deviceID string,
	serviceName string,
	serviceStatus string,
	severity Severity,
	reason string,
	detectedAt time.Time,
) Incident {
	t := detectedAt.UTC()
	if severity == "" {
		severity = SeverityMedium
	}

	return Incident{
		IncidentID:    incidentID,
		DeviceID:      deviceID,
		ServiceName:   serviceName,
		ServiceStatus: serviceStatus,
		State:         StateDetected,
		Severity:      severity,
		CreatedAt:     t,
		UpdatedAt:     t,
		DetectedAt:    t,
		LastSeenAt:    t,
		Reason:        reason,
		Active:        true,
	}
}
