package agentbuilder

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParseFinalSummary_Valid(t *testing.T) {
	raw, _ := json.Marshal(validSummary())
	got, err := ParseFinalSummary(raw, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RootCause == "" || got.Result == "" || len(got.Evidence) == 0 || len(got.ActionsTaken) == 0 {
		t.Fatalf("expected fully populated summary: %+v", got)
	}
}

func TestParseFinalSummary_Sanitizes(t *testing.T) {
	in := validSummary()
	in.RootCause = "  spaced cause  "
	in.Evidence = []string{"  e1  ", "", "   "}
	in.ActionsTaken = []string{"", "a1"}
	raw, _ := json.Marshal(in)

	got, err := ParseFinalSummary(raw, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RootCause != "spaced cause" {
		t.Errorf("rootCause not trimmed: %q", got.RootCause)
	}
	if len(got.Evidence) != 1 || got.Evidence[0] != "e1" {
		t.Errorf("evidence not sanitized: %+v", got.Evidence)
	}
	if len(got.ActionsTaken) != 1 || got.ActionsTaken[0] != "a1" {
		t.Errorf("actions not sanitized: %+v", got.ActionsTaken)
	}
}

func TestParseFinalSummary_Errors(t *testing.T) {
	cases := []struct {
		name                string
		mutate              func(*IncidentSummary)
		remediationOccurred bool
		wantErr             error
	}{
		{"empty root cause", func(s *IncidentSummary) { s.RootCause = "" }, true, ErrEmptyRootCause},
		{"empty evidence", func(s *IncidentSummary) { s.Evidence = nil }, true, ErrEmptyEvidence},
		{"empty result", func(s *IncidentSummary) { s.Result = "" }, true, ErrEmptyResult},
		{"missing actions with remediation", func(s *IncidentSummary) { s.ActionsTaken = nil }, true, ErrEmptyActionsTaken},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSummary()
			tc.mutate(&s)
			raw, _ := json.Marshal(s)
			_, err := ParseFinalSummary(raw, tc.remediationOccurred)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
			// raw payload preserved for debugging
			var pe ParseError
			if !errors.As(err, &pe) || len(pe.RawPayload) == 0 {
				t.Fatalf("expected ParseError preserving raw payload, got %T", err)
			}
		})
	}
}

func TestParseFinalSummary_ActionsOptionalWithoutRemediation(t *testing.T) {
	s := validSummary()
	s.ActionsTaken = nil
	raw, _ := json.Marshal(s)

	if _, err := ParseFinalSummary(raw, false); err != nil {
		t.Fatalf("actionsTaken should be optional when no remediation occurred: %v", err)
	}
	if _, err := ParseFinalSummary(raw, true); !errors.Is(err, ErrEmptyActionsTaken) {
		t.Fatalf("expected ErrEmptyActionsTaken when remediation occurred, got %v", err)
	}
}

func TestParseFinalSummary_EmptyPayload(t *testing.T) {
	_, err := ParseFinalSummary(nil, true)
	var pe ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError for empty payload, got %v", err)
	}
}

func TestParseFinalSummary_InvalidJSONPreservesRaw(t *testing.T) {
	raw := []byte(`{not valid json`)
	_, err := ParseFinalSummary(raw, true)
	var pe ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %v", err)
	}
	if string(pe.RawPayload) != string(raw) {
		t.Fatalf("raw payload not preserved: %q", pe.RawPayload)
	}
}

func TestParseFinalSummary_UnwrapsEnvelope(t *testing.T) {
	inner := validSummary()
	for _, key := range []string{"summary", "finalSummary", "result", "output", "response"} {
		env := map[string]any{key: inner, "trace_id": "t-1"}
		raw, _ := json.Marshal(env)
		got, err := ParseFinalSummary(raw, true)
		if err != nil {
			t.Fatalf("envelope key %q: unexpected error: %v", key, err)
		}
		if got.RootCause != inner.RootCause {
			t.Fatalf("envelope key %q: not unwrapped: %+v", key, got)
		}
	}
}

func TestRemediationOccurred(t *testing.T) {
	if RemediationOccurred(FinalSummaryRequest{}) {
		t.Errorf("empty request should report no remediation")
	}
	if !RemediationOccurred(FinalSummaryRequest{RemediationStatus: "succeeded"}) {
		t.Errorf("remediation status should count")
	}
	if !RemediationOccurred(FinalSummaryRequest{ApprovedActions: []string{"restart_service"}}) {
		t.Errorf("approved actions should count")
	}
	if !RemediationOccurred(FinalSummaryRequest{ExecutionResults: []SummaryExecutionResult{{ActionID: "x"}}}) {
		t.Errorf("execution results should count")
	}
}

func TestFallbackSummary_Resolved(t *testing.T) {
	req := sampleSummaryRequest()
	fb := FallbackSummary(req)
	if err := validateFinalSummary(fb, RemediationOccurred(req)); err != nil {
		t.Fatalf("fallback should be valid: %v", err)
	}
	if !strings.Contains(strings.ToLower(fb.Result), "resolved") {
		t.Errorf("resolved fallback result should mention resolution: %q", fb.Result)
	}
	if fb.RootCause != req.ProbableCause {
		t.Errorf("fallback should reuse probable cause: %q", fb.RootCause)
	}
}

func TestFallbackSummary_Failed(t *testing.T) {
	req := sampleSummaryRequest()
	req.Outcome = OutcomeFailed
	req.ValidationFailureReason = "validation timed out: endpoint did not return to healthy"
	fb := FallbackSummary(req)
	if err := validateFinalSummary(fb, RemediationOccurred(req)); err != nil {
		t.Fatalf("failed fallback should be valid: %v", err)
	}
	if strings.Contains(strings.ToLower(fb.Result), "recovered and the incident was resolved") {
		t.Errorf("failed fallback must not claim recovery: %q", fb.Result)
	}
	if !strings.Contains(fb.Result, "timed out") {
		t.Errorf("failed fallback should surface failure reason: %q", fb.Result)
	}
}

func TestFallbackSummary_NoRemediationNoEvidence(t *testing.T) {
	req := FinalSummaryRequest{
		IncidentID: "inc-9",
		Outcome:    OutcomeFailed,
		Reason:     "service status is stopped",
	}
	fb := FallbackSummary(req)
	if err := validateFinalSummary(fb, RemediationOccurred(req)); err != nil {
		t.Fatalf("fallback must always be valid: %v", err)
	}
	if len(fb.ActionsTaken) != 1 || !strings.Contains(fb.ActionsTaken[0], "No remediation") {
		t.Errorf("expected explicit no-remediation action entry: %+v", fb.ActionsTaken)
	}
	if len(fb.Evidence) == 0 {
		t.Errorf("fallback must synthesize at least one evidence line")
	}
}
