// backend/internal/elastic/incidents.go

package elastic

import "time"

type IncidentEventDocument struct {
	SchemaVersion string `json:"schemaVersion,omitempty"`

	EventType string    `json:"eventType"`
	Timestamp time.Time `json:"timestamp"`

	IncidentID string `json:"incidentId"`

	DeviceID      string `json:"deviceId"`
	ServiceName   string `json:"serviceName"`
	ServiceStatus string `json:"serviceStatus"`

	Severity string `json:"severity"`
	State    string `json:"state"`

	Reason string `json:"reason"`
	Active bool   `json:"active"`
}
