package elastic

import (
	"fmt"
	"strings"
)

func ValidateTelemetryDocument(
	doc TelemetryEventDocument,
) error {

	if strings.TrimSpace(doc.DeviceID) == "" {
		return fmt.Errorf("deviceId is required")
	}

	if strings.TrimSpace(doc.ServiceName) == "" {
		return fmt.Errorf("serviceName is required")
	}

	if strings.TrimSpace(doc.ServiceStatus) == "" {
		return fmt.Errorf("serviceStatus is required")
	}

	if doc.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	if strings.TrimSpace(doc.EventType) == "" {
		return fmt.Errorf("eventType is required")
	}

	return nil
}

func ValidateIncidentDocument(
	doc IncidentEventDocument,
) error {

	if strings.TrimSpace(doc.DeviceID) == "" {
		return fmt.Errorf("deviceId is required")
	}

	if strings.TrimSpace(doc.IncidentID) == "" {
		return fmt.Errorf("incidentId is required")
	}

	if strings.TrimSpace(doc.ServiceName) == "" {
		return fmt.Errorf("serviceName is required")
	}

	if strings.TrimSpace(doc.ServiceStatus) == "" {
		return fmt.Errorf("serviceStatus is required")
	}

	if strings.TrimSpace(doc.Severity) == "" {
		return fmt.Errorf("severity is required")
	}

	if doc.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	if strings.TrimSpace(doc.EventType) == "" {
		return fmt.Errorf("eventType is required")
	}

	return nil
}

func ValidateLogDocument(
	doc LogEventDocument,
) error {

	if strings.TrimSpace(doc.DeviceID) == "" {
		return fmt.Errorf("deviceId is required")
	}

	if strings.TrimSpace(doc.ServiceName) == "" {
		return fmt.Errorf("serviceName is required")
	}

	if strings.TrimSpace(doc.Message) == "" {
		return fmt.Errorf("message is required")
	}

	if doc.Timestamp.IsZero() {
		return fmt.Errorf("timestamp is required")
	}

	if strings.TrimSpace(doc.EventType) == "" {
		return fmt.Errorf("eventType is required")
	}

	return nil
}
