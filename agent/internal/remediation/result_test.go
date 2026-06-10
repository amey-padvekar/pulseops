package remediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestExecutionResult_DeserializesCanonicalFixture pins the agent-side result DTO to
// the frozen wire contract shared with the backend, so the producing side never drifts
// from what the backend stores.
func TestExecutionResult_DeserializesCanonicalFixture(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "contracts", "remediation_result_fixture.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var res ExecutionResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	if res.IncidentID != "INC-1001" || res.DeviceID != "DEV-AGENT-01" || res.RequestID != "rem-12345" {
		t.Fatalf("identity/correlation not parsed: %+v", res)
	}
	if res.Status != ExecStatusSucceeded {
		t.Fatalf("status: got %q", res.Status)
	}
	wantStart := time.Date(2026, 5, 23, 22, 10, 6, 0, time.UTC)
	wantFinish := time.Date(2026, 5, 23, 22, 10, 8, 0, time.UTC)
	if !res.StartedAt.Equal(wantStart) || !res.FinishedAt.Equal(wantFinish) {
		t.Fatalf("timestamps not parsed: %v / %v", res.StartedAt, res.FinishedAt)
	}
	if len(res.Results) != 1 {
		t.Fatalf("expected 1 action result, got %d", len(res.Results))
	}
	ar := res.Results[0]
	if ar.ActionID != "restart_service" || ar.Target != "OpenVPNService" || ar.Status != ExecStatusSucceeded {
		t.Fatalf("action result not parsed: %+v", ar)
	}
	if ar.Stdout != "Service restarted successfully" {
		t.Fatalf("stdout not parsed: %q", ar.Stdout)
	}
	if ar.ExitCode == nil || *ar.ExitCode != 0 {
		t.Fatalf("exitCode not parsed: %+v", ar.ExitCode)
	}
	if ar.DurationMs != 1500 {
		t.Fatalf("durationMs not parsed: got %d want 1500", ar.DurationMs)
	}
}
