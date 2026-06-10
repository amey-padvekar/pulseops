package remediation

import (
	"errors"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func approvedIncident() incidents.Incident {
	approvedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	return incidents.Incident{
		IncidentID: "INC-1001",
		DeviceID:   "dev-1",
		State:      incidents.StateApproved,
		ApprovedBy: "demo.operator",
		ApprovedAt: &approvedAt,
		RecommendedActions: []incidents.RecommendedAction{
			{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart"},
			{ActionID: "flush_dns", Target: "endpoint", Description: "flush"},
		},
		ApprovedActions: []string{"restart_service"},
	}
}

func TestNewCommand_BuildsFromApprovedSnapshot(t *testing.T) {
	inc := approvedIncident()

	cmd, err := NewCommand(inc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cmd.IncidentID != "INC-1001" || cmd.DeviceID != "dev-1" || cmd.ApprovedBy != "demo.operator" {
		t.Fatalf("unexpected command header: %+v", cmd)
	}
	if !cmd.ApprovedAt.Equal(*inc.ApprovedAt) {
		t.Fatalf("approvedAt: got %v want %v", cmd.ApprovedAt, *inc.ApprovedAt)
	}
	if cmd.Status != StatusQueued {
		t.Fatalf("status: got %q want %q", cmd.Status, StatusQueued)
	}
	if len(cmd.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(cmd.Actions))
	}
	// Target must come from the recommendation snapshot, not the approved id alone.
	if cmd.Actions[0].ActionID != "restart_service" || cmd.Actions[0].Target != "OpenVPNService" {
		t.Fatalf("action not derived from snapshot: %+v", cmd.Actions[0])
	}
}

func TestNewCommand_RejectsUnapprovedIncident(t *testing.T) {
	inc := approvedIncident()
	inc.State = incidents.StateAwaitingApproval

	if _, err := NewCommand(inc); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("expected ErrNotApproved, got %v", err)
	}
}

func TestNewCommand_RejectsNoApprovedActions(t *testing.T) {
	inc := approvedIncident()
	inc.ApprovedActions = nil

	if _, err := NewCommand(inc); !errors.Is(err, ErrNoApprovedActions) {
		t.Fatalf("expected ErrNoApprovedActions, got %v", err)
	}
}

func TestNewCommand_RejectsActionNotInSnapshot(t *testing.T) {
	inc := approvedIncident()
	// An approved id with no matching recommendation should never happen via the
	// store, but the constructor must defend against it rather than emit a blank target.
	inc.ApprovedActions = []string{"reconnect_vpn"}

	if _, err := NewCommand(inc); !errors.Is(err, ErrActionNotInRecommendation) {
		t.Fatalf("expected ErrActionNotInRecommendation, got %v", err)
	}
}

func TestNewCommand_RejectsActionNotInCatalog(t *testing.T) {
	inc := approvedIncident()
	// A poisoned recommendation snapshot carrying a non-catalog action that was then
	// "approved" must never reach the executable payload.
	inc.RecommendedActions = []incidents.RecommendedAction{
		{ActionID: "danger_cmd", Target: "host", Description: "not a catalog action"},
	}
	inc.ApprovedActions = []string{"danger_cmd"}

	if _, err := NewCommand(inc); !errors.Is(err, ErrActionNotInCatalog) {
		t.Fatalf("expected ErrActionNotInCatalog, got %v", err)
	}
}
