package incidents

import (
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/store"
)

// serviceStatusRunning is the telemetry serviceStatus value that signals a healthy,
// running service. Detection (detector.go) keys off "stopped"; recovery is the inverse.
const serviceStatusRunning = "running"

// HealthCheck is one evaluated recovery criterion. It is the unit the UI uses to
// explain, field by field, why an incident resolved or failed (step 4.2 task 4).
type HealthCheck struct {
	// Name identifies the telemetry field/criterion (e.g. "heartbeat").
	Name string `json:"name"`
	// Passed reports whether the observed telemetry satisfied this criterion.
	Passed bool `json:"passed"`
	// Required reports whether this criterion must pass for the snapshot to be healthy.
	// A non-required check that fails is recorded as evidence but does not block recovery.
	Required bool `json:"required"`
	// Detail is a short human-readable explanation of the observed value.
	Detail string `json:"detail"`
}

// HealthEvaluation is the deterministic result of checking a single post-remediation
// telemetry snapshot against the recovery criteria for the MVP service-failure incident.
type HealthEvaluation struct {
	// Healthy is true only when every required criterion passed.
	Healthy bool `json:"healthy"`
	// Checks is the per-criterion breakdown, in a stable order for the UI.
	Checks []HealthCheck `json:"checks"`
	// Reason is a single demo-narratable summary of the evaluation outcome.
	Reason string `json:"reason"`
}

// ValidationSnapshot is a compact, self-contained record of one telemetry observation
// evaluated during recovery validation (Phase 10 step 4.7). It captures the telemetry
// values that were checked plus the per-criterion verdict, so the incident can explain
// the evidence behind a resolution or failure without consulting raw telemetry logs.
type ValidationSnapshot struct {
	ObservedAt       time.Time     `json:"observedAt"`
	Healthy          bool          `json:"healthy"`
	Reason           string        `json:"reason"`
	Checks           []HealthCheck `json:"checks"`
	ServiceStatus    string        `json:"serviceStatus"`
	Heartbeat        bool          `json:"heartbeat"`
	NetworkReachable bool          `json:"networkReachable"`
}

// newValidationSnapshot builds a ValidationSnapshot from the evaluated telemetry and its
// health verdict, taking a defensive copy of the checks so later mutation cannot alter
// stored evidence.
func newValidationSnapshot(state store.DeviceState, eval HealthEvaluation, observedAt time.Time) ValidationSnapshot {
	return ValidationSnapshot{
		ObservedAt:       observedAt.UTC(),
		Healthy:          eval.Healthy,
		Reason:           eval.Reason,
		Checks:           append([]HealthCheck(nil), eval.Checks...),
		ServiceStatus:    state.ServiceStatus,
		Heartbeat:        state.Heartbeat,
		NetworkReachable: state.NetworkReachable,
	}
}

// HealthCriteria configures which recovery criteria are mandatory. The defaults are
// tuned for the deterministic MVP demo; connectivity is optional because
// networkReachable is the least reliable signal across demo environments.
type HealthCriteria struct {
	// RequireNetworkReachable promotes the connectivity check from optional evidence to
	// a mandatory recovery criterion. Defaults to false (step 4.2 task 3).
	RequireNetworkReachable bool
}

// DefaultHealthCriteria returns the recovery criteria used for the MVP service-failure
// incident type: heartbeat and a running service are mandatory; connectivity is optional.
func DefaultHealthCriteria() HealthCriteria {
	return HealthCriteria{RequireNetworkReachable: false}
}

// EvaluateHealth deterministically checks one telemetry snapshot against the recovery
// criteria and returns a per-criterion breakdown plus an overall healthy verdict.
//
// All inputs are fields the agent already produces in store.DeviceState (step 4.2
// task 2), so validation never depends on signals the endpoint does not report:
//   - heartbeat       must be true        (mandatory)
//   - serviceStatus   must be "running"   (mandatory)
//   - networkReachable should be true     (mandatory only when criteria require it)
//
// The snapshot is healthy only when every mandatory check passes. The connectivity
// check is always recorded so the UI can show it even when it is not gating recovery.
func EvaluateHealth(state store.DeviceState, criteria HealthCriteria) HealthEvaluation {
	checks := []HealthCheck{
		heartbeatCheck(state),
		serviceStatusCheck(state),
		networkReachableCheck(state, criteria.RequireNetworkReachable),
	}

	healthy := true
	failed := make([]string, 0, len(checks))
	for _, c := range checks {
		if c.Required && !c.Passed {
			healthy = false
			// Cite the observed condition (e.g. "service status is stopped"), not just the
			// field name, so the reason is self-explanatory for the UI, operator debugging,
			// and later summary generation.
			failed = append(failed, c.Detail)
		}
	}

	return HealthEvaluation{
		Healthy: healthy,
		Checks:  checks,
		Reason:  summarizeHealth(healthy, failed),
	}
}

func heartbeatCheck(state store.DeviceState) HealthCheck {
	detail := "heartbeat present"
	if !state.Heartbeat {
		detail = "heartbeat missing"
	}
	return HealthCheck{
		Name:     "heartbeat",
		Passed:   state.Heartbeat,
		Required: true,
		Detail:   detail,
	}
}

func serviceStatusCheck(state store.DeviceState) HealthCheck {
	status := strings.TrimSpace(strings.ToLower(state.ServiceStatus))
	passed := status == serviceStatusRunning
	detail := "service running"
	if !passed {
		observed := status
		if observed == "" {
			observed = "unknown"
		}
		detail = "service status is " + observed
	}
	return HealthCheck{
		Name:     "serviceStatus",
		Passed:   passed,
		Required: true,
		Detail:   detail,
	}
}

func networkReachableCheck(state store.DeviceState, required bool) HealthCheck {
	detail := "network reachable"
	if !state.NetworkReachable {
		detail = "network unreachable"
	}
	return HealthCheck{
		Name:     "networkReachable",
		Passed:   state.NetworkReachable,
		Required: required,
		Detail:   detail,
	}
}

// summarizeHealth produces a single demo-narratable reason string for the verdict.
func summarizeHealth(healthy bool, failed []string) string {
	if healthy {
		return "post-remediation telemetry healthy: service running with heartbeat present"
	}
	return "post-remediation telemetry unhealthy: " + strings.Join(failed, ", ") + " did not recover"
}
