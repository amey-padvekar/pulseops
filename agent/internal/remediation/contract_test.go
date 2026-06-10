package remediation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCommand_DeserializesCanonicalFixture pins the agent-side DTO to the frozen
// wire contract shared with the backend. If the canonical fixture and this struct
// ever drift, this test fails loudly rather than the agent silently dropping fields.
func TestCommand_DeserializesCanonicalFixture(t *testing.T) {
	// agent/internal/remediation -> repo root is three levels up.
	path := filepath.Join("..", "..", "..", "docs", "contracts", "remediation_command_fixture.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var cmd Command
	if err := json.Unmarshal(raw, &cmd); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}

	if cmd.IncidentID != "INC-1001" || cmd.DeviceID != "DEV-AGENT-01" {
		t.Fatalf("identity not parsed: %+v", cmd)
	}
	if cmd.ApprovedBy != "demo.operator" {
		t.Fatalf("approvedBy not parsed: %q", cmd.ApprovedBy)
	}
	wantApprovedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	if !cmd.ApprovedAt.Equal(wantApprovedAt) {
		t.Fatalf("approvedAt: got %v want %v", cmd.ApprovedAt, wantApprovedAt)
	}
	wantDispatchedAt := time.Date(2026, 5, 23, 22, 10, 5, 0, time.UTC)
	if !cmd.DispatchedAt.Equal(wantDispatchedAt) {
		t.Fatalf("dispatchedAt: got %v want %v", cmd.DispatchedAt, wantDispatchedAt)
	}
	if cmd.RequestID != "rem-12345" {
		t.Fatalf("requestId: got %q", cmd.RequestID)
	}
	if len(cmd.Actions) != 1 || cmd.Actions[0].ActionID != "restart_service" || cmd.Actions[0].Target != "OpenVPNService" {
		t.Fatalf("actions not parsed: %+v", cmd.Actions)
	}
}
