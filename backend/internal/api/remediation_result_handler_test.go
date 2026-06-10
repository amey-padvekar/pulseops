package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/remediation"
)

// approvedIncident seeds an approved incident in the store and returns its ID.
func approvedIncidentForResult(t *testing.T, s *incidents.Store) string {
	t.Helper()
	seed := incidents.NewIncident("INC-1", "dev-1", "OpenVPNService", "stopped", incidents.SeverityHigh, "down")
	inc, _ := s.CreateOrGetActive("dev-1:OpenVPNService", seed)
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, ""); err != nil {
		t.Fatalf("investigating: %v", err)
	}
	// Attach a recommendation, promote, approve.
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "service stopped", 0.9,
		[]incidents.RecommendedAction{{ActionID: "restart_service", Target: "OpenVPNService"}},
		[]string{"verify"}, "summary", time.Now().UTC(), "", "completed"); err != nil {
		t.Fatalf("save investigation: %v", err)
	}
	if _, _, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if _, err := s.Approve(inc.IncidentID, "demo.operator", []string{"restart_service"}, "", time.Now().UTC()); err != nil {
		t.Fatalf("approve: %v", err)
	}
	return inc.IncidentID
}

// dispatchedQueue enqueues a command for the incident and dispatches it so it carries a
// requestID, returning the queue and that requestID.
func dispatchedQueue(incidentID, deviceID string) (*remediation.Queue, string) {
	q := remediation.NewQueue()
	q.Enqueue(remediation.Command{
		IncidentID: incidentID,
		DeviceID:   deviceID,
		Actions:    []remediation.Action{{ActionID: "restart_service", Target: "OpenVPNService"}},
		Status:     remediation.StatusQueued,
	})
	dispatched := q.ClaimPendingForDevice(deviceID, time.Now().UTC(), func() string { return "rem-1" })
	return q, dispatched[0].RequestID
}

func postResult(t *testing.T, store *incidents.Store, q *remediation.Queue, body remediation.ExecutionResult) (*httptest.ResponseRecorder, *incidents.Incident) {
	t.Helper()
	var broadcast *incidents.Incident
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/remediation/results", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	RemediationResultHandler(store, q, func(updated incidents.Incident) { broadcast = &updated })(rec, req)
	return rec, broadcast
}

func successResult(incidentID, deviceID, requestID string) remediation.ExecutionResult {
	exit := 0
	return remediation.ExecutionResult{
		IncidentID: incidentID,
		DeviceID:   deviceID,
		RequestID:  requestID,
		Status:     remediation.ExecStatusSucceeded,
		StartedAt:  time.Date(2026, 5, 23, 22, 10, 6, 0, time.UTC),
		FinishedAt: time.Date(2026, 5, 23, 22, 10, 8, 0, time.UTC),
		Results: []remediation.ActionResult{
			{ActionID: "restart_service", Target: "OpenVPNService", Status: remediation.ExecStatusSucceeded, Stdout: "ok", ExitCode: &exit, DurationMs: 1500},
		},
	}
}

func TestRemediationResultHandler_SuccessPersistsAndMovesToValidating(t *testing.T) {
	store := incidents.NewStore()
	id := approvedIncidentForResult(t, store)
	q, reqID := dispatchedQueue(id, "dev-1")

	rec, broadcast := postResult(t, store, q, successResult(id, "dev-1", reqID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var updated incidents.Incident
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if updated.State != incidents.StateValidating {
		t.Fatalf("state: got %q want validating", updated.State)
	}
	if updated.RemediationStatus != "succeeded" || len(updated.RemediationResults) != 1 {
		t.Fatalf("result not persisted: %+v", updated)
	}
	if broadcast == nil || broadcast.State != incidents.StateValidating {
		t.Fatalf("expected broadcast of updated incident, got %+v", broadcast)
	}
}

func TestRemediationResultHandler_FailureMovesToFailed(t *testing.T) {
	store := incidents.NewStore()
	id := approvedIncidentForResult(t, store)
	q, reqID := dispatchedQueue(id, "dev-1")

	body := successResult(id, "dev-1", reqID)
	body.Status = remediation.ExecStatusFailed
	body.Results[0].Status = remediation.ExecStatusFailed

	rec, _ := postResult(t, store, q, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	var updated incidents.Incident
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.State != incidents.StateFailed {
		t.Fatalf("state: got %q want failed", updated.State)
	}
}

func TestRemediationResultHandler_DuplicateResultIgnored(t *testing.T) {
	store := incidents.NewStore()
	id := approvedIncidentForResult(t, store)
	q, reqID := dispatchedQueue(id, "dev-1")

	// First result: processed, command acknowledged.
	rec1, broadcast1 := postResult(t, store, q, successResult(id, "dev-1", reqID))
	if rec1.Code != http.StatusOK || broadcast1 == nil {
		t.Fatalf("first result not processed: code=%d broadcast=%v", rec1.Code, broadcast1)
	}

	// Duplicate delivery of the same requestId: idempotent 200, no re-broadcast.
	rec2, broadcast2 := postResult(t, store, q, successResult(id, "dev-1", reqID))
	if rec2.Code != http.StatusOK {
		t.Fatalf("duplicate result: got %d want 200", rec2.Code)
	}
	if broadcast2 != nil {
		t.Fatal("duplicate result must not re-broadcast")
	}

	// The incident timeline must not have grown from the duplicate (started/finished
	// appended once only).
	got, _ := store.GetByID(id)
	finished := 0
	for _, e := range got.Timeline {
		if e.Type == incidents.EventCommandFinished {
			finished++
		}
	}
	if finished != 1 {
		t.Fatalf("expected exactly one finished event, got %d (timeline=%v)", finished, got.Timeline)
	}
}

func TestRemediationResultHandler_Validations(t *testing.T) {
	store := incidents.NewStore()
	id := approvedIncidentForResult(t, store)
	q, reqID := dispatchedQueue(id, "dev-1")

	t.Run("method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/remediation/results", nil)
		rec := httptest.NewRecorder()
		RemediationResultHandler(store, q, nil)(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("got %d want 405", rec.Code)
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		rec, _ := postResult(t, store, q, remediation.ExecutionResult{Status: remediation.ExecStatusSucceeded})
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d want 400", rec.Code)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		body := successResult(id, "dev-1", reqID)
		body.Status = "done"
		rec, _ := postResult(t, store, q, body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("got %d want 400", rec.Code)
		}
	})

	t.Run("unknown incident", func(t *testing.T) {
		rec, _ := postResult(t, store, q, successResult("INC-MISSING", "dev-1", reqID))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("got %d want 404", rec.Code)
		}
	})

	t.Run("device mismatch", func(t *testing.T) {
		rec, _ := postResult(t, store, q, successResult(id, "dev-OTHER", reqID))
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d want 409", rec.Code)
		}
	})

	t.Run("requestId mismatch", func(t *testing.T) {
		rec, _ := postResult(t, store, q, successResult(id, "dev-1", "rem-WRONG"))
		if rec.Code != http.StatusConflict {
			t.Fatalf("got %d want 409", rec.Code)
		}
	})
}
