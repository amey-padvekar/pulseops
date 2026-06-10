package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	adapteradk "pulseops/google-agent-service/cloudrun-adapter/internal/adk"
	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

func TestInvestigateRoundTripWithFrozenFixture(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to resolve runtime caller path")
	}

	fixturePath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "docs", "contracts", "adk_request_fixture.json"))
	body, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read request fixture %s: %v", fixturePath, err)
	}

	h := NewHandler(adapteradk.NewStubClient(), slog.Default())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "/investigate", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var resp domain.InvestigateResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response json: %v", err)
	}

	if resp.Status.Transport != "success" || resp.Status.Workflow != "completed" {
		t.Fatalf("unexpected status: %+v", resp.Status)
	}
	if resp.Result == nil {
		t.Fatalf("expected result payload")
	}
	if resp.Result.ProbableCause == "" || resp.Result.Summary == "" {
		t.Fatalf("result missing required fields: %+v", resp.Result)
	}
}
