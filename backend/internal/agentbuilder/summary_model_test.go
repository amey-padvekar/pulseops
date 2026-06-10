package agentbuilder

import (
	"encoding/json"
	"errors"
	"testing"
)

func validSummary() IncidentSummary {
	return IncidentSummary{
		RootCause: "OpenVPN service unexpectedly stopped.",
		Evidence: []string{
			"Telemetry showed serviceStatus=stopped while heartbeat remained true.",
			"Validation telemetry later confirmed serviceStatus=running after remediation.",
		},
		ActionsTaken: []string{
			"Approved action: restart_service for OpenVPNService.",
			"Agent executed the restart successfully.",
		},
		Result:          "Service health recovered and the incident was resolved.",
		OperatorSummary: "The monitored service stopped, the approved remediation restarted it, and recovery was confirmed by fresh telemetry.",
	}
}

func TestIncidentSummary_Validate_Valid(t *testing.T) {
	if err := validSummary().Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestIncidentSummary_Validate_OptionalOperatorSummary(t *testing.T) {
	s := validSummary()
	s.OperatorSummary = ""
	if err := s.Validate(); err != nil {
		t.Fatalf("operatorSummary should be optional, got: %v", err)
	}
}

func TestIncidentSummary_Validate_Errors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*IncidentSummary)
		wantErr error
	}{
		{"empty root cause", func(s *IncidentSummary) { s.RootCause = "" }, ErrEmptyRootCause},
		{"nil evidence", func(s *IncidentSummary) { s.Evidence = nil }, ErrEmptyEvidence},
		{"blank evidence entry", func(s *IncidentSummary) { s.Evidence = []string{"ok", ""} }, ErrBlankEvidenceEntry},
		{"nil actions", func(s *IncidentSummary) { s.ActionsTaken = nil }, ErrEmptyActionsTaken},
		{"blank action entry", func(s *IncidentSummary) { s.ActionsTaken = []string{""} }, ErrBlankActionEntry},
		{"empty result", func(s *IncidentSummary) { s.Result = "" }, ErrEmptyResult},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSummary()
			tc.mutate(&s)
			err := s.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestIncidentSummary_JSONRoundTrip(t *testing.T) {
	in := validSummary()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out IncidentSummary
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := out.Validate(); err != nil {
		t.Fatalf("round-tripped summary failed validation: %v", err)
	}
	if out.RootCause != in.RootCause || out.Result != in.Result {
		t.Fatalf("round-trip mismatch: %+v", out)
	}
	if len(out.Evidence) != len(in.Evidence) || len(out.ActionsTaken) != len(in.ActionsTaken) {
		t.Fatalf("array length mismatch after round-trip: %+v", out)
	}
}

func TestIncidentSummary_OmitsEmptyOperatorSummary(t *testing.T) {
	s := validSummary()
	s.OperatorSummary = ""
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if contains(string(raw), "operatorSummary") {
		t.Fatalf("expected operatorSummary to be omitted when empty, got: %s", raw)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
