package incidents

import (
	"fmt"
	"strings"

	"github.com/certainelf/pulseops/backend/internal/store"
)

const (
	serviceStoppedFailureSignature = "service_stopped"
	serviceStoppedReason           = "service stopped while heartbeat is present"
)

// DetectionResult is the decision output of telemetry incident evaluation.
type DetectionResult struct {
	ShouldCreateOrUpdate bool
	DedupeKey            string
	FailureSignature     string
	Reason               string
	Severity             Severity
}

// EvaluateTelemetry deterministically evaluates telemetry against incident rules.
func EvaluateTelemetry(state store.DeviceState) DetectionResult {
	status := strings.TrimSpace(strings.ToLower(state.ServiceStatus))
	if status != "stopped" || !state.Heartbeat {
		return DetectionResult{}
	}

	deviceID := strings.TrimSpace(state.DeviceID)
	serviceName := strings.TrimSpace(state.ServiceName)

	return DetectionResult{
		ShouldCreateOrUpdate: true,
		DedupeKey:            fmt.Sprintf("%s|%s|%s", deviceID, serviceName, serviceStoppedFailureSignature),
		FailureSignature:     serviceStoppedFailureSignature,
		Reason:               serviceStoppedReason,
		Severity:             AssignSeverity(state),
	}
}
