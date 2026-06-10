package incidents

import (
	"testing"

	"github.com/certainelf/pulseops/backend/internal/store"
)

func healthyState() store.DeviceState {
	return store.DeviceState{
		DeviceID:         "dev-1",
		ServiceName:      "vpn",
		ServiceStatus:    "running",
		Heartbeat:        true,
		NetworkReachable: true,
	}
}

func findCheck(t *testing.T, eval HealthEvaluation, name string) HealthCheck {
	t.Helper()
	for _, c := range eval.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("expected a %q check in evaluation, got %+v", name, eval.Checks)
	return HealthCheck{}
}

func TestEvaluateHealth_AllCriteriaPass(t *testing.T) {
	eval := EvaluateHealth(healthyState(), DefaultHealthCriteria())
	if !eval.Healthy {
		t.Fatalf("expected healthy evaluation, got %+v", eval)
	}
	if eval.Reason == "" {
		t.Fatal("expected a reason for narration")
	}
	for _, name := range []string{"heartbeat", "serviceStatus", "networkReachable"} {
		if c := findCheck(t, eval, name); !c.Passed {
			t.Fatalf("expected %q check to pass, got %+v", name, c)
		}
	}
}

func TestEvaluateHealth_ServiceStillStoppedFails(t *testing.T) {
	state := healthyState()
	state.ServiceStatus = "stopped"

	eval := EvaluateHealth(state, DefaultHealthCriteria())
	if eval.Healthy {
		t.Fatalf("expected unhealthy evaluation when service is stopped, got %+v", eval)
	}
	c := findCheck(t, eval, "serviceStatus")
	if c.Passed {
		t.Fatal("expected serviceStatus check to fail")
	}
	if c.Detail == "" {
		t.Fatal("expected a failure detail for the UI")
	}
}

func TestEvaluateHealth_MissingHeartbeatFails(t *testing.T) {
	state := healthyState()
	state.Heartbeat = false

	eval := EvaluateHealth(state, DefaultHealthCriteria())
	if eval.Healthy {
		t.Fatalf("expected unhealthy evaluation when heartbeat missing, got %+v", eval)
	}
	if c := findCheck(t, eval, "heartbeat"); c.Passed {
		t.Fatal("expected heartbeat check to fail")
	}
}

func TestEvaluateHealth_ServiceStatusCaseInsensitive(t *testing.T) {
	state := healthyState()
	state.ServiceStatus = "  RUNNING  "

	eval := EvaluateHealth(state, DefaultHealthCriteria())
	if !eval.Healthy {
		t.Fatalf("expected healthy evaluation with padded/upper status, got %+v", eval)
	}
}

func TestEvaluateHealth_ConnectivityOptionalByDefault(t *testing.T) {
	// With default criteria, an unreachable network is recorded as evidence but does
	// not block recovery (step 4.2 task 3).
	state := healthyState()
	state.NetworkReachable = false

	eval := EvaluateHealth(state, DefaultHealthCriteria())
	if !eval.Healthy {
		t.Fatalf("expected healthy evaluation when connectivity is optional, got %+v", eval)
	}
	c := findCheck(t, eval, "networkReachable")
	if c.Required {
		t.Fatal("expected networkReachable to be optional by default")
	}
	if c.Passed {
		t.Fatal("expected networkReachable check to record the failed signal")
	}
}

func TestEvaluateHealth_ConnectivityMandatoryWhenRequired(t *testing.T) {
	state := healthyState()
	state.NetworkReachable = false

	eval := EvaluateHealth(state, HealthCriteria{RequireNetworkReachable: true})
	if eval.Healthy {
		t.Fatalf("expected unhealthy evaluation when connectivity is required, got %+v", eval)
	}
	if c := findCheck(t, eval, "networkReachable"); !c.Required {
		t.Fatal("expected networkReachable to be required")
	}
}
