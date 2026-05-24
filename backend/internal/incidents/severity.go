package incidents

import (
	"strings"

	"github.com/certainelf/pulseops/backend/internal/store"
)

// AssignSeverity maps telemetry state to a deterministic incident severity.
//
// Extension points for later phases:
// - network reachability loss can increase severity.
// - repeated failures within a short window can increase severity.
func AssignSeverity(state store.DeviceState) Severity {
	status := strings.TrimSpace(strings.ToLower(state.ServiceStatus))
	if status == "stopped" && state.Heartbeat {
		return SeverityHigh
	}

	return SeverityMedium
}
