package incidents

import (
	"testing"

	"github.com/certainelf/pulseops/backend/internal/store"
)

func TestAssignSeverity_StoppedWithHeartbeat_IsHigh(t *testing.T) {
	sev := AssignSeverity(store.DeviceState{
		ServiceStatus: "stopped",
		Heartbeat:     true,
	})

	if sev != SeverityHigh {
		t.Fatalf("severity: got %q, want %q", sev, SeverityHigh)
	}
}

func TestAssignSeverity_Default_IsMedium(t *testing.T) {
	tests := []store.DeviceState{
		{ServiceStatus: "running", Heartbeat: true},
		{ServiceStatus: "stopped", Heartbeat: false},
		{ServiceStatus: "", Heartbeat: false},
	}

	for i, tc := range tests {
		sev := AssignSeverity(tc)
		if sev != SeverityMedium {
			t.Fatalf("case %d severity: got %q, want %q", i, sev, SeverityMedium)
		}
	}
}
