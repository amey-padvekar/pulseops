// backend/internal/elastic/telemetry.go

package elastic

import "time"

type TelemetryEventDocument struct {
	SchemaVersion string    `json:"schemaVersion,omitempty"`
	EventType     string    `json:"eventType"`
	Timestamp     time.Time `json:"timestamp"`

	DeviceID      string `json:"deviceId"`
	ServiceName   string `json:"serviceName"`
	ServiceStatus string `json:"serviceStatus"`

	Heartbeat        bool `json:"heartbeat"`
	NetworkReachable bool `json:"networkReachable"`

	CPUUsage    float64 `json:"cpuUsage"`
	MemoryUsage float64 `json:"memoryUsage"`

	IncidentID string   `json:"incidentId,omitempty"`
	RecentLogs []string `json:"recentLogs,omitempty"`
}
