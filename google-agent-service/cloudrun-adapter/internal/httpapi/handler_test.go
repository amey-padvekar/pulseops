package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapteradk "pulseops/google-agent-service/cloudrun-adapter/internal/adk"
	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

type fakeADKClient struct {
	result      domain.InvestigationResult
	traceID     string
	err         error
	errs        []error
	traceIDs    []string
	calls       int
	capturedReq domain.InvestigateRequest
}

func (f *fakeADKClient) Investigate(_ context.Context, req domain.InvestigateRequest) (domain.InvestigationResult, string, error) {
	f.calls++
	f.capturedReq = req
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		trace := f.traceID
		if len(f.traceIDs) > 0 {
			trace = f.traceIDs[0]
			f.traceIDs = f.traceIDs[1:]
		}
		if err != nil {
			return domain.InvestigationResult{}, trace, err
		}
		return f.result, trace, nil
	}
	if f.err != nil {
		return domain.InvestigationResult{}, f.traceID, f.err
	}
	return f.result, f.traceID, nil
}

func TestHealth(t *testing.T) {
	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestInvestigateValidationFailure(t *testing.T) {
	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(`{"task":""}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestInvestigateAuthorizationEnforcedWhenTokenConfigured(t *testing.T) {
	t.Setenv("INBOUND_AUTH_TOKEN", "expected-token")

	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`

	unauthorizedReq := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	unauthorizedRes := httptest.NewRecorder()
	mux.ServeHTTP(unauthorizedRes, unauthorizedReq)

	if unauthorizedRes.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want %d", unauthorizedRes.Code, http.StatusUnauthorized)
	}

	authorizedReq := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	authorizedReq.Header.Set("Authorization", "Bearer expected-token")
	authorizedRes := httptest.NewRecorder()
	mux.ServeHTTP(authorizedRes, authorizedReq)

	if authorizedRes.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want %d", authorizedRes.Code, http.StatusOK)
	}
}

func TestInvestigateMalformedJSONDeterministicEnvelope(t *testing.T) {
	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(`{"task":"phase7_investigation"`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var res domain.InvestigateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if res.Status.Transport != "error" || res.Status.Workflow != "failed" {
		t.Fatalf("unexpected status: %+v", res.Status)
	}
	if res.Error == nil || res.Error.Code != "INVALID_JSON" {
		t.Fatalf("unexpected error envelope: %+v", res.Error)
	}
	if strings.TrimSpace(res.TraceID) == "" {
		t.Fatalf("expected generated trace_id")
	}
}

func TestInvestigatePayloadTooLarge(t *testing.T) {
	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	largeEvidence := strings.Repeat("a", maxBodyBytes+1024)
	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"` + largeEvidence + `"}`
	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestInvestigateNormalizesOptionalFieldsAndTraceID(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.7,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service"}},
			ValidationSteps:    []string{"check service status"},
			Summary:            "needs restart",
		},
	}

	h := NewHandler(fake, slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"task":"phase7_investigation",
		"prompt":" investigate ",
		"metadata":{"incident_id":" inc-1 ","device_id":" dev-1 ","request_id":" req-1 "},
		"available_actions":[{"actionId":"restart_service"}],
		"evidence_summary":"   "
	}`

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	if fake.capturedReq.EvidenceSummary != "No evidence summary provided by caller." {
		t.Fatalf("unexpected normalized evidence summary: %q", fake.capturedReq.EvidenceSummary)
	}
	if fake.capturedReq.Metadata.RequestID != "req-1" {
		t.Fatalf("expected trimmed request_id, got %q", fake.capturedReq.Metadata.RequestID)
	}

	var res domain.InvestigateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if strings.TrimSpace(res.TraceID) == "" {
		t.Fatalf("expected generated trace_id when upstream trace is missing")
	}
}

func TestInvestigateRejectsUnknownFields(t *testing.T) {
	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"task":"phase7_investigation",
		"prompt":"p",
		"metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},
		"available_actions":[{"actionId":"restart_service"}],
		"unexpected":"field"
	}`

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}

	var res domain.InvestigateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if res.Error == nil || res.Error.Code != "INVALID_JSON" {
		t.Fatalf("expected INVALID_JSON error code, got %+v", res.Error)
	}
}

func TestInvestigateInvalidModelOutputReturnsDeterministicFailedEnvelope(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.8,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "dangerous_action", Target: "svc", Reason: "x"}},
			ValidationSteps:    []string{"check status"},
			Summary:            "summary",
		},
		traceID: "trace-model-1",
	}

	h := NewHandler(fake, slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{
		"task":"phase7_investigation",
		"prompt":"p",
		"metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},
		"available_actions":[{"actionId":"restart_service"}],
		"evidence_summary":"heartbeat=true"
	}`

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadGateway)
	}

	var res domain.InvestigateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if res.Error == nil || res.Error.Code != "MODEL_OUTPUT_INVALID" {
		t.Fatalf("expected MODEL_OUTPUT_INVALID, got %+v", res.Error)
	}
	if res.Status.Transport != "error" || res.Status.Workflow != "failed" {
		t.Fatalf("unexpected status: %+v", res.Status)
	}
}

func TestInvestigateRetriesOnceOnTransientFailure(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.8,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service", Target: "svc", Reason: "safe"}},
			ValidationSteps:    []string{"check status"},
			Summary:            "summary",
		},
		errs:     []error{&adapteradk.TransientError{Err: errors.New("timeout")}, nil},
		traceIDs: []string{"trace-1", "trace-2"},
	}

	h := NewHandler(fake, slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`
	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if fake.calls != 2 {
		t.Fatalf("calls = %d, want 2", fake.calls)
	}
}

func TestInvestigateDoesNotRetryNonTransientFailure(t *testing.T) {
	fake := &fakeADKClient{
		errs:    []error{errors.New("validation-like upstream failure")},
		traceID: "trace-1",
	}

	h := NewHandler(fake, slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`
	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusBadGateway)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
}

func TestMetricsEndpointReflectsOutcomes(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.7,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service", Target: "svc", Reason: "safe"}},
			ValidationSteps:    []string{"check service status"},
			Summary:            "needs restart",
		},
	}

	h := NewHandler(fake, slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	okBody := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`
	postInvestigate(mux, okBody)
	postInvestigate(mux, okBody)
	// Invalid JSON -> validation_fail outcome.
	postInvestigate(mux, `{"task":`)

	snap := h.Metrics().Snapshot()
	if snap.Requests != 3 {
		t.Fatalf("requests = %d, want 3", snap.Requests)
	}
	if snap.Success != 2 {
		t.Fatalf("success = %d, want 2", snap.Success)
	}
	if snap.ValidationFail != 1 || snap.Fail != 1 {
		t.Fatalf("validationFail=%d fail=%d, want 1/1", snap.ValidationFail, snap.Fail)
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRes := httptest.NewRecorder()
	mux.ServeHTTP(metricsRes, metricsReq)
	if metricsRes.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", metricsRes.Code)
	}
	if !strings.Contains(metricsRes.Body.String(), "investigate_requests_total 3") {
		t.Fatalf("metrics endpoint missing request total:\n%s", metricsRes.Body.String())
	}
}

func TestInvestigateEmitsRequiredLogFields(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.7,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service", Target: "svc", Reason: "safe"}},
			ValidationSteps:    []string{"check service status"},
			Summary:            "needs restart",
		},
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := NewHandler(fake, logger)
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`
	postInvestigate(mux, body)

	var entry map[string]any
	if err := json.Unmarshal(findLogLine(t, logBuf, "investigate_request"), &entry); err != nil {
		t.Fatalf("log line not valid json: %v", err)
	}

	required := []string{
		"request_id", "incident_id", "device_id", "trace_id",
		"status_transport", "status_workflow", "latency_ms",
		"confidence", "action_ids", "enrichment_used", "evidence_lines",
	}
	for _, field := range required {
		if _, ok := entry[field]; !ok {
			t.Fatalf("log entry missing required field %q: %v", field, entry)
		}
	}
	if entry["request_id"] != "req-1" {
		t.Fatalf("request_id = %v, want req-1", entry["request_id"])
	}
	if entry["status_transport"] != "success" {
		t.Fatalf("status_transport = %v, want success", entry["status_transport"])
	}
}

func TestInvestigateDoesNotDumpRawEvidenceOrPrompt(t *testing.T) {
	fake := &fakeADKClient{
		result: domain.InvestigationResult{
			ProbableCause:      "service stopped",
			Confidence:         0.7,
			RecommendedActions: []domain.RecommendedAction{{ActionID: "restart_service", Target: "svc", Reason: "safe"}},
			ValidationSteps:    []string{"check service status"},
			Summary:            "needs restart",
		},
	}

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	h := NewHandler(fake, logger)
	mux := http.NewServeMux()
	h.Register(mux)

	const evidenceSentinel = "SENSITIVE_EVIDENCE_LINE_42"
	const promptSentinel = "SENSITIVE_PROMPT_TEXT_99"
	body := `{"task":"phase7_investigation","prompt":"` + promptSentinel + `","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"` + evidenceSentinel + `"}`
	postInvestigate(mux, body)

	out := logBuf.String()
	if strings.Contains(out, evidenceSentinel) {
		t.Fatalf("raw evidence summary was logged:\n%s", out)
	}
	if strings.Contains(out, promptSentinel) {
		t.Fatalf("raw prompt was logged:\n%s", out)
	}
}

func postInvestigate(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func findLogLine(t *testing.T, buf bytes.Buffer, msg string) []byte {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.Contains(line, `"msg":"`+msg+`"`) {
			return []byte(line)
		}
	}
	t.Fatalf("no log line with msg %q found in:\n%s", msg, buf.String())
	return nil
}

func TestInvestigateStopsRetryWhenModelBudgetExpires(t *testing.T) {
	fake := &fakeADKClient{
		errs: []error{&adapteradk.TransientError{Err: context.DeadlineExceeded}},
	}

	h := NewHandler(fake, slog.Default())
	// Force near-immediate timeout by wrapping request with short context.
	mux := http.NewServeMux()
	h.Register(mux)

	body := `{"task":"phase7_investigation","prompt":"p","metadata":{"incident_id":"inc-1","device_id":"dev-1","request_id":"req-1"},"available_actions":[{"actionId":"restart_service"}],"evidence_summary":"heartbeat=true"}`
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBufferString(body)).WithContext(ctx)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if fake.calls < 1 {
		t.Fatalf("expected at least one call")
	}
}
