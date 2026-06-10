package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/remediation"
)

func queuedCommand(incidentID, deviceID string) remediation.Command {
	return remediation.Command{
		IncidentID: incidentID,
		DeviceID:   deviceID,
		ApprovedBy: "demo.operator",
		ApprovedAt: time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC),
		Actions:    []remediation.Action{{ActionID: "restart_service", Target: "OpenVPNService"}},
		Status:     remediation.StatusQueued,
	}
}

func doCommandsRequest(t *testing.T, q *remediation.Queue, method, deviceID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/devices/"+deviceID+"/commands", nil)
	req.SetPathValue("deviceId", deviceID)
	rec := httptest.NewRecorder()
	PendingCommandsHandler(q, nil, nil)(rec, req)
	return rec
}

func TestPendingCommandsHandler_DispatchesQueuedCommand(t *testing.T) {
	q := remediation.NewQueue()
	q.Enqueue(queuedCommand("INC-1", "dev-1"))

	rec := doCommandsRequest(t, q, http.MethodGet, "dev-1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}

	var resp PendingCommandsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.DeviceID != "dev-1" || len(resp.Commands) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	dc := resp.Commands[0]
	if dc.IncidentID != "INC-1" || dc.RequestID == "" || dc.DispatchedAt.IsZero() {
		t.Fatalf("dispatch payload not populated: %+v", dc)
	}

	// Fetching dispatched it; a second poll must return an empty array, not re-dispatch.
	rec2 := doCommandsRequest(t, q, http.MethodGet, "dev-1")
	var resp2 PendingCommandsResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode second body: %v", err)
	}
	if resp2.Commands == nil {
		t.Fatal("commands should serialize as [] not null")
	}
	if len(resp2.Commands) != 0 {
		t.Fatalf("command re-dispatched on second poll: %+v", resp2.Commands)
	}
}

func TestPendingCommandsHandler_EmptyWhenNoneForDevice(t *testing.T) {
	q := remediation.NewQueue()
	q.Enqueue(queuedCommand("INC-1", "dev-1"))

	rec := doCommandsRequest(t, q, http.MethodGet, "dev-2")
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rec.Code)
	}
	var resp PendingCommandsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(resp.Commands) != 0 {
		t.Fatalf("expected no commands for dev-2, got %+v", resp.Commands)
	}
}

func TestPendingCommandsHandler_RejectsNonGet(t *testing.T) {
	q := remediation.NewQueue()
	rec := doCommandsRequest(t, q, http.MethodPost, "dev-1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d want 405", rec.Code)
	}
}

func TestPendingCommandsHandler_MissingDevice(t *testing.T) {
	q := remediation.NewQueue()
	rec := doCommandsRequest(t, q, http.MethodGet, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", rec.Code)
	}
}
