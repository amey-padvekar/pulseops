package remediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// These integration-style tests bind the backend wire DTOs to the exact canonical
// fixtures the agent module consumes (docs/contracts/*.json), so a drift on either side
// is caught here. backend/internal/remediation -> repo root is three levels up.

func readContractFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "contracts", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func TestDispatchCommand_RoundTripsCanonicalFixture(t *testing.T) {
	raw := readContractFixture(t, "remediation_command_fixture.json")

	var dc DispatchCommand
	if err := json.Unmarshal(raw, &dc); err != nil {
		t.Fatalf("unmarshal command fixture: %v", err)
	}
	if dc.IncidentID != "INC-1001" || dc.DeviceID != "DEV-AGENT-01" || dc.RequestID != "rem-12345" {
		t.Fatalf("fixture not parsed into DispatchCommand: %+v", dc)
	}
	if len(dc.Actions) != 1 || dc.Actions[0].ActionID != "restart_service" || dc.Actions[0].Target != "OpenVPNService" {
		t.Fatalf("actions not parsed: %+v", dc.Actions)
	}
	if dc.DispatchedAt.IsZero() || dc.ApprovedAt.IsZero() {
		t.Fatalf("timestamps not parsed: %+v", dc)
	}

	// Re-marshal and parse again: the type round-trips its own wire form losslessly.
	out, err := json.Marshal(dc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back DispatchCommand
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if !reflect.DeepEqual(dc, back) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", back, dc)
	}
}

func TestExecutionResult_RoundTripsCanonicalFixture(t *testing.T) {
	raw := readContractFixture(t, "remediation_result_fixture.json")

	var res ExecutionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal result fixture: %v", err)
	}
	if res.IncidentID != "INC-1001" || res.RequestID != "rem-12345" || res.Status != ExecStatusSucceeded {
		t.Fatalf("fixture not parsed into ExecutionResult: %+v", res)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 action result, got %d", len(res.Results))
	}
	ar := res.Results[0]
	if ar.ActionID != "restart_service" || ar.Status != ExecStatusSucceeded {
		t.Fatalf("action result not parsed: %+v", ar)
	}
	if ar.ExitCode == nil || *ar.ExitCode != 0 || ar.DurationMs != 1500 {
		t.Fatalf("exitCode/durationMs not parsed: exit=%v dur=%d", ar.ExitCode, ar.DurationMs)
	}

	out, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back ExecutionResult
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	if !reflect.DeepEqual(res, back) {
		t.Fatalf("round-trip mismatch:\n got %+v\nwant %+v", back, res)
	}
}
