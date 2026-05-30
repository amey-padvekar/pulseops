package elastic

import (
	"testing"
	"time"
)

func TestValidateTelemetryDocument_Valid(t *testing.T) {

	doc := TelemetryEventDocument{
		EventType:     "telemetry_received",
		Timestamp:     time.Now().UTC(),
		DeviceID:      "DEV-01",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
	}

	err := ValidateTelemetryDocument(doc)
	if err != nil {
		t.Fatalf("expected valid telemetry doc, got error: %v", err)
	}
}

func TestValidateTelemetryDocument_MissingDeviceID(t *testing.T) {

	doc := TelemetryEventDocument{
		EventType:     "telemetry_received",
		Timestamp:     time.Now().UTC(),
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
	}

	err := ValidateTelemetryDocument(doc)

	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateIncidentDocument_Valid(t *testing.T) {

	doc := IncidentEventDocument{
		EventType:     "incident_updated",
		Timestamp:     time.Now().UTC(),
		IncidentID:    "INC-001",
		DeviceID:      "DEV-01",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "stopped",
		Severity:      "high",
	}

	err := ValidateIncidentDocument(doc)
	if err != nil {
		t.Fatalf("expected valid incident doc, got: %v", err)
	}
}

func TestValidateLogDocument_Valid(t *testing.T) {

	doc := LogEventDocument{
		EventType:   "endpoint_log",
		Timestamp:   time.Now().UTC(),
		DeviceID:    "DEV-01",
		ServiceName: "OpenVPNService",
		Message:     "service stopped",
		Source:      "agent_recent_logs",
	}

	err := ValidateLogDocument(doc)
	if err != nil {
		t.Fatalf("expected valid log doc, got: %v", err)
	}
}
