package agentbuilder

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// fakeSummaryStore is an in-memory SummaryStore for orchestrator tests.
type fakeSummaryStore struct {
	inc         incidents.Incident
	found       bool
	saved       *incidents.FinalSummary
	savedStatus string
	savedReqID  string
	statusCalls []string
	saveErr     error
	statusErr   error
}

func (f *fakeSummaryStore) GetByID(string) (incidents.Incident, bool) {
	return f.inc, f.found
}

func (f *fakeSummaryStore) SetSummaryStatus(_, status, requestID string) (incidents.Incident, error) {
	if f.statusErr != nil {
		return incidents.Incident{}, f.statusErr
	}
	f.statusCalls = append(f.statusCalls, status)
	f.inc.SummaryStatus = status
	if requestID != "" {
		f.inc.SummaryRequestID = requestID
	}
	return f.inc, nil
}

func (f *fakeSummaryStore) SaveFinalSummary(_ string, summary incidents.FinalSummary, requestID, status string, at time.Time) (incidents.Incident, error) {
	if f.saveErr != nil {
		return incidents.Incident{}, f.saveErr
	}
	s := summary
	f.saved = &s
	f.savedStatus = status
	f.savedReqID = requestID
	f.inc.FinalSummary = &s
	f.inc.SummaryStatus = status
	f.inc.SummaryGeneratedAt = &at
	return f.inc, nil
}

// stubSummarySubmitter returns a canned payload or error.
type stubSummarySubmitter struct {
	raw json.RawMessage
	err error
}

func (s stubSummarySubmitter) SubmitSummary(context.Context, ADKSummaryRequestPayload) (json.RawMessage, error) {
	return s.raw, s.err
}

func terminalIncident() incidents.Incident {
	inc := resolvedIncident() // from summary_request_test.go
	inc.SummaryStatus = ""
	return inc
}

func TestGenerateAndStoreSummary_LiveSuccess(t *testing.T) {
	raw, _ := json.Marshal(validSummary())
	store := &fakeSummaryStore{inc: terminalIncident(), found: true}
	submitter := stubSummarySubmitter{raw: raw}

	updated, changed, err := GenerateAndStoreSummary(context.Background(), store, submitter, "inc-1", SummaryGenerationConfig{Now: time.Unix(1000, 0).UTC()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if store.savedStatus != SummaryStatusGenerated {
		t.Fatalf("status = %q, want generated", store.savedStatus)
	}
	if updated.FinalSummary == nil || updated.FinalSummary.RootCause == "" {
		t.Fatalf("expected stored summary content: %+v", updated.FinalSummary)
	}
	// Pending must be set before the final save (idempotency in-flight marker).
	if len(store.statusCalls) == 0 || store.statusCalls[0] != SummaryStatusPending {
		t.Fatalf("expected pending status set first, got %v", store.statusCalls)
	}
}

func TestGenerateAndStoreSummary_FallbackOnSubmitError(t *testing.T) {
	store := &fakeSummaryStore{inc: terminalIncident(), found: true}
	submitter := stubSummarySubmitter{err: errors.New("boom")}

	updated, changed, err := GenerateAndStoreSummary(context.Background(), store, submitter, "inc-1", SummaryGenerationConfig{})
	if err != nil {
		t.Fatalf("fallback should not be an error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if store.savedStatus != SummaryStatusFallback {
		t.Fatalf("status = %q, want fallback", store.savedStatus)
	}
	if updated.FinalSummary == nil || updated.FinalSummary.Result == "" {
		t.Fatalf("expected deterministic fallback stored: %+v", updated.FinalSummary)
	}
}

func TestGenerateAndStoreSummary_FallbackOnMalformedResponse(t *testing.T) {
	store := &fakeSummaryStore{inc: terminalIncident(), found: true}
	submitter := stubSummarySubmitter{raw: json.RawMessage(`{"garbage":true}`)}

	_, changed, err := GenerateAndStoreSummary(context.Background(), store, submitter, "inc-1", SummaryGenerationConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || store.savedStatus != SummaryStatusFallback {
		t.Fatalf("expected fallback on malformed response, got status=%q changed=%v", store.savedStatus, changed)
	}
}

func TestGenerateAndStoreSummary_NoSubmitterUsesFallback(t *testing.T) {
	store := &fakeSummaryStore{inc: terminalIncident(), found: true}

	_, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, "inc-1", SummaryGenerationConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || store.savedStatus != SummaryStatusFallback {
		t.Fatalf("expected fallback with no submitter, got status=%q", store.savedStatus)
	}
}

func TestGenerateAndStoreSummary_SkipsNonTerminal(t *testing.T) {
	inc := terminalIncident()
	inc.State = incidents.StateValidating
	store := &fakeSummaryStore{inc: inc, found: true}

	_, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, "inc-1", SummaryGenerationConfig{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("non-terminal incident must not generate a summary")
	}
	if store.saved != nil {
		t.Fatal("nothing should be stored for a non-terminal incident")
	}
}

func TestGenerateAndStoreSummary_IdempotentOnExisting(t *testing.T) {
	inc := terminalIncident()
	inc.SummaryStatus = SummaryStatusGenerated
	store := &fakeSummaryStore{inc: inc, found: true}

	_, changed, err := GenerateAndStoreSummary(context.Background(), store, nil, "inc-1", SummaryGenerationConfig{Mode: SummaryTriggerAuto})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed {
		t.Fatal("auto trigger must not regenerate an existing summary")
	}

	// A fallback summary is likewise not auto-regenerated.
	inc.SummaryStatus = SummaryStatusFallback
	store2 := &fakeSummaryStore{inc: inc, found: true}
	if _, changed, _ := GenerateAndStoreSummary(context.Background(), store2, nil, "inc-1", SummaryGenerationConfig{}); changed {
		t.Fatal("auto trigger must not regenerate a fallback summary")
	}

	// Operator mode may regenerate.
	store3 := &fakeSummaryStore{inc: inc, found: true}
	if _, changed, _ := GenerateAndStoreSummary(context.Background(), store3, nil, "inc-1", SummaryGenerationConfig{Mode: SummaryTriggerOperator}); !changed {
		t.Fatal("operator trigger should regenerate")
	}
}

func TestGenerateAndStoreSummary_NotFound(t *testing.T) {
	store := &fakeSummaryStore{found: false}
	if _, _, err := GenerateAndStoreSummary(context.Background(), store, nil, "missing", SummaryGenerationConfig{}); !errors.Is(err, incidents.ErrIncidentNotFound) {
		t.Fatalf("got %v, want ErrIncidentNotFound", err)
	}
}

func TestGenerateAndStoreSummary_TimeoutFallsBack(t *testing.T) {
	store := &fakeSummaryStore{inc: terminalIncident(), found: true}
	// Submitter that respects context cancellation.
	slow := slowSubmitter{}

	ctx := context.Background()
	_, changed, err := GenerateAndStoreSummary(ctx, store, slow, "inc-1", SummaryGenerationConfig{Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("timeout should fall back, not error: %v", err)
	}
	if !changed || store.savedStatus != SummaryStatusFallback {
		t.Fatalf("expected fallback on timeout, got status=%q", store.savedStatus)
	}
}

type slowSubmitter struct{}

func (slowSubmitter) SubmitSummary(ctx context.Context, _ ADKSummaryRequestPayload) (json.RawMessage, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestGenerateAndStoreSummary_RequiresStore(t *testing.T) {
	if _, _, err := GenerateAndStoreSummary(context.Background(), nil, nil, "inc-1", SummaryGenerationConfig{}); err == nil {
		t.Fatal("expected error for nil store")
	}
}
