package remediation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func sampleResult() ExecutionResult {
	exit := 0
	return ExecutionResult{
		IncidentID: "INC-1",
		DeviceID:   "dev-1",
		RequestID:  "rem-1",
		Status:     ExecStatusSucceeded,
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
		Results:    []ActionResult{{ActionID: "restart_service", Status: ExecStatusSucceeded, ExitCode: &exit, DurationMs: 1500}},
	}
}

func TestHTTPReporter_Report_PostsResult(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody ExecutionResult
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	reporter, err := NewHTTPReporter(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("NewHTTPReporter: %v", err)
	}
	if err := reporter.Report(context.Background(), sampleResult()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/remediation/results" {
		t.Fatalf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if gotBody.IncidentID != "INC-1" || gotBody.RequestID != "rem-1" {
		t.Fatalf("result not posted faithfully: %+v", gotBody)
	}
}

func TestHTTPReporter_Report_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "conflict", http.StatusConflict)
	}))
	defer srv.Close()

	reporter, err := NewHTTPReporter(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("NewHTTPReporter: %v", err)
	}
	if err := reporter.Report(context.Background(), sampleResult()); err == nil {
		t.Fatal("expected an error for a 409 response")
	}
}

func TestJoinResultsEndpoint(t *testing.T) {
	got, err := joinResultsEndpoint("http://localhost:8080")
	if err != nil {
		t.Fatalf("joinResultsEndpoint: %v", err)
	}
	if got != "http://localhost:8080/remediation/results" {
		t.Fatalf("got %q", got)
	}
	if _, err := joinResultsEndpoint("not-a-url"); err == nil {
		t.Fatal("expected error for non-absolute base URL")
	}
}
