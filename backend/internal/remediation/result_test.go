package remediation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestExecutionStatus_IsValid(t *testing.T) {
	valid := []ExecutionStatus{
		ExecStatusQueued, ExecStatusDispatched, ExecStatusRunning,
		ExecStatusSucceeded, ExecStatusFailed, ExecStatusRejected,
	}
	for _, s := range valid {
		if !s.IsValid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	for _, s := range []ExecutionStatus{"", "done", "error", "Succeeded"} {
		if ExecutionStatus(s).IsValid() {
			t.Errorf("status %q should be invalid", s)
		}
	}
}

func TestBoundLog(t *testing.T) {
	short := "all good"
	if got := BoundLog(short); got != short {
		t.Fatalf("short log changed: %q", got)
	}

	long := strings.Repeat("x", MaxLogBytes+500)
	got := BoundLog(long)
	if len(got) > MaxLogBytes+len(logTruncationMarker) {
		t.Fatalf("bounded log too long: %d bytes", len(got))
	}
	if !strings.HasSuffix(got, logTruncationMarker) {
		t.Fatalf("bounded log missing truncation marker: %q", got[len(got)-20:])
	}
}

func TestBoundLog_RespectsRuneBoundary(t *testing.T) {
	// Multibyte runes packed past the limit must not be cut mid-rune.
	long := strings.Repeat("é", MaxLogBytes) // 2 bytes each, well over the cap
	got := BoundLog(long)
	body := strings.TrimSuffix(got, logTruncationMarker)
	if !utf8ValidString(body) {
		t.Fatalf("bounded log is not valid UTF-8")
	}
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestExecutionResult_Normalize(t *testing.T) {
	loc := time.FixedZone("PT", -7*3600)
	r := ExecutionResult{
		IncidentID: "INC-1001",
		StartedAt:  time.Date(2026, 5, 23, 15, 10, 6, 0, loc),
		FinishedAt: time.Date(2026, 5, 23, 15, 10, 8, 0, loc),
		Results: []ActionResult{
			{ActionID: "restart_service", Status: ExecStatusSucceeded, Stdout: strings.Repeat("y", MaxLogBytes+10)},
		},
	}

	n := r.Normalize()
	if n.StartedAt.Location() != time.UTC || n.FinishedAt.Location() != time.UTC {
		t.Fatalf("timestamps not normalized to UTC: %v / %v", n.StartedAt.Location(), n.FinishedAt.Location())
	}
	if !strings.HasSuffix(n.Results[0].Stdout, logTruncationMarker) {
		t.Fatalf("stdout not bounded by normalize")
	}
	// Original must be untouched.
	if strings.HasSuffix(r.Results[0].Stdout, logTruncationMarker) {
		t.Fatalf("normalize mutated the original result")
	}
}

func TestExecutionResult_JSONShapeMatchesContract(t *testing.T) {
	r := ExecutionResult{
		IncidentID: "INC-1001",
		DeviceID:   "DEV-AGENT-01",
		RequestID:  "rem-12345",
		Status:     ExecStatusSucceeded,
		StartedAt:  time.Date(2026, 5, 23, 22, 10, 6, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 23, 22, 10, 8, 0, time.UTC),
		Results: []ActionResult{
			{ActionID: "restart_service", Target: "OpenVPNService", Status: ExecStatusSucceeded, Stdout: "Service restarted successfully", Stderr: "", ExitCode: intPtr(0), DurationMs: 1500},
		},
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"incidentId", "deviceId", "requestId", "status", "startedAt", "finishedAt", "results"} {
		if _, ok := top[k]; !ok {
			t.Errorf("missing top-level key %q in %s", k, raw)
		}
	}

	var perAction []map[string]json.RawMessage
	if err := json.Unmarshal(top["results"], &perAction); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	for _, k := range []string{"actionId", "status", "stdout", "stderr", "exitCode", "durationMs"} {
		if _, ok := perAction[0][k]; !ok {
			t.Errorf("missing per-action key %q in %s", k, top["results"])
		}
	}
}

func intPtr(v int) *int { return &v }
