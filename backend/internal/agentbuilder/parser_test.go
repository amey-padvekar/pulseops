package agentbuilder

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseInvestigationResult_Valid(t *testing.T) {
	res := InvestigationResult{
		ProbableCause: "service crashed",
		Confidence:    0.85,
		RecommendedActions: []RecommendedAction{
			{ActionID: ActionRestartService, Target: "svc-a", Reason: "service exit detected"},
		},
		ValidationSteps: []string{"check service logs"},
		Summary:         "service crashed and needs restart",
	}

	raw, _ := json.Marshal(res)

	allowed := []ActionOption{{ActionID: ActionRestartService, Target: "svc-a"}}

	parsed, err := ParseInvestigationResultWithAllowedActions(raw, allowed)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if parsed.ProbableCause != res.ProbableCause {
		t.Fatalf("probable cause mismatch: got %q want %q", parsed.ProbableCause, res.ProbableCause)
	}
}

func TestParseInvestigationResult_InvalidActionID(t *testing.T) {
	// Action ID not in allowed catalog
	payload := `{"probableCause":"x","confidence":0.5,"recommendedActions":[{"actionId":"dangerous","target":"t","reason":"r"}],"validationSteps":["s"],"summary":"s"}`
	allowed := []ActionOption{{ActionID: ActionRestartService, Target: "t"}}

	_, err := ParseInvestigationResultWithAllowedActions([]byte(payload), allowed)
	if err == nil {
		t.Fatalf("expected parse error for invalid action id")
	}
}

func TestParseInvestigationResult_MalformedJSON(t *testing.T) {
	raw := []byte(`{not a valid json}`)
	_, err := ParseInvestigationResult(raw)
	if err == nil {
		t.Fatalf("expected parse error for malformed json")
	}
}

func TestParseInvestigationResult_RejectsUnsafeTarget(t *testing.T) {
	// Target reaches execution, so shell-like content there must be rejected.
	payload := `{"probableCause":"service issue","confidence":0.6,"recommendedActions":[{"actionId":"restart_service","target":"svc; rm -rf /","reason":"safe"}],"validationSteps":["check status"],"summary":"safe summary"}`

	_, err := ParseInvestigationResult([]byte(payload))
	if err == nil {
		t.Fatalf("expected parse error for unsafe target")
	}

	var pe ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if !errors.Is(pe.Err, ErrUnsafeContent) {
		t.Fatalf("expected ErrUnsafeContent, got %v", pe.Err)
	}
}

func TestParseInvestigationResult_AllowsProseWithWindowsPathsAndCommands(t *testing.T) {
	// Display-only prose may legitimately contain Windows paths ('\'), '$', backticks, or
	// command words. These are never executed, so they must NOT cause rejection.
	payload := `{"probableCause":"MySQL92 stopped; inspect C:\\ProgramData\\MySQL\\Data ($log) or run Restart-Service","confidence":0.6,"recommendedActions":[{"actionId":"restart_service","target":"MySQL92","reason":"service exited; see C:\\path"}],"validationSteps":["Verify service at C:\\Windows\\System32"],"summary":"restart MySQL92 | confirm running"}`
	allowed := []ActionOption{{ActionID: ActionRestartService, Target: "MySQL92"}}

	res, err := ParseInvestigationResultWithAllowedActions([]byte(payload), allowed)
	if err != nil {
		t.Fatalf("prose with paths/commands should be allowed, got: %v", err)
	}
	if res.ProbableCause == "" || len(res.RecommendedActions) != 1 {
		t.Fatalf("expected a parsed result, got %+v", res)
	}
}

func TestParseInvestigationResult_RejectsUnsupportedTarget(t *testing.T) {
	payload := `{"probableCause":"service issue","confidence":0.6,"recommendedActions":[{"actionId":"restart_service","target":"svc-a","reason":"safe"}],"validationSteps":["check status"],"summary":"safe summary"}`
	allowed := []ActionOption{{ActionID: ActionRestartService, Target: "svc-b"}}

	_, err := ParseInvestigationResultWithAllowedActions([]byte(payload), allowed)
	if err == nil {
		t.Fatalf("expected parse error for unsupported target")
	}
}

func TestParseInvestigationResult_PreservesRawPayloadOnFailure(t *testing.T) {
	raw := []byte(`{"probableCause":"","confidence":0.8,"recommendedActions":[],"validationSteps":["x"],"summary":"x"}`)
	_, err := ParseInvestigationResult(raw)
	if err == nil {
		t.Fatalf("expected parse failure")
	}

	var pe ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("expected ParseError, got %T", err)
	}
	if string(pe.RawPayload) != string(raw) {
		t.Fatalf("raw payload not preserved")
	}
}
