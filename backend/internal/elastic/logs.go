// backend/internal/elastic/logs.go

package elastic

import "time"

type LogEventDocument struct {
	SchemaVersion string `json:"schemaVersion,omitempty"`

	EventType string    `json:"eventType"`
	Timestamp time.Time `json:"timestamp"`

	DeviceID    string `json:"deviceId"`
	ServiceName string `json:"serviceName"`

	IncidentID string `json:"incidentId,omitempty"`

	Message string `json:"message"`
	Source  string `json:"source"`
}
