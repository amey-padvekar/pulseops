package remediation

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewDispatchCommand_CarriesContractFields(t *testing.T) {
	cmd, err := NewCommand(approvedIncident())
	if err != nil {
		t.Fatalf("unexpected error building command: %v", err)
	}

	dispatchedAt := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)
	dc := NewDispatchCommand(cmd, "rem-12345", dispatchedAt)

	if dc.IncidentID != cmd.IncidentID || dc.DeviceID != cmd.DeviceID {
		t.Fatalf("identity not carried: %+v", dc)
	}
	if dc.ApprovedBy != cmd.ApprovedBy || !dc.ApprovedAt.Equal(cmd.ApprovedAt) {
		t.Fatalf("approval metadata not carried: %+v", dc)
	}
	if dc.RequestID != "rem-12345" {
		t.Fatalf("requestId: got %q want %q", dc.RequestID, "rem-12345")
	}
	if !dc.DispatchedAt.Equal(dispatchedAt) {
		t.Fatalf("dispatchedAt: got %v want %v", dc.DispatchedAt, dispatchedAt)
	}
	if len(dc.Actions) != 1 || dc.Actions[0].ActionID != "restart_service" || dc.Actions[0].Target != "OpenVPNService" {
		t.Fatalf("actions not carried from command: %+v", dc.Actions)
	}
}

func TestNewDispatchCommand_CopiesActions(t *testing.T) {
	cmd, err := NewCommand(approvedIncident())
	if err != nil {
		t.Fatalf("unexpected error building command: %v", err)
	}

	dc := NewDispatchCommand(cmd, "rem-1", time.Unix(0, 0).UTC())
	dc.Actions[0].Target = "mutated"

	if cmd.Actions[0].Target == "mutated" {
		t.Fatal("mutating dispatch actions leaked back into the queued command")
	}
}

func TestDispatchCommand_JSONShapeMatchesContract(t *testing.T) {
	dc := DispatchCommand{
		IncidentID:   "INC-1001",
		DeviceID:     "DEV-AGENT-01",
		ApprovedBy:   "demo.operator",
		ApprovedAt:   time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC),
		Actions:      []Action{{ActionID: "restart_service", Target: "OpenVPNService"}},
		DispatchedAt: time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC),
		RequestID:    "rem-12345",
	}

	raw, err := json.Marshal(dc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// The wire contract is keyed by camelCase and must never include backend-only
	// queue state. Compare against the field set the agent relies on.
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	want := []string{"incidentId", "deviceId", "approvedBy", "approvedAt", "actions", "dispatchedAt", "requestId"}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing contract key %q in %s", k, raw)
		}
	}
	if _, leaked := got["status"]; leaked {
		t.Errorf("dispatch payload leaked backend-only queue state: %s", raw)
	}
	if len(got) != len(want) {
		t.Errorf("unexpected dispatch keys: got %d want %d (%s)", len(got), len(want), raw)
	}
}
