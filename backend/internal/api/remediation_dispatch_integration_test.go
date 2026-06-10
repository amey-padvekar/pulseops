package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/remediation"
)

// awaitingApprovalIncident seeds an incident that has a recommendation and is awaiting
// approval (but is NOT approved), returning its ID.
func awaitingApprovalIncident(t *testing.T, s *incidents.Store, incidentID, deviceID string) string {
	t.Helper()
	seed := incidents.NewIncident(incidentID, deviceID, "OpenVPNService", "stopped", incidents.SeverityHigh, "down")
	inc, _ := s.CreateOrGetActive(deviceID+":OpenVPNService", seed)
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, ""); err != nil {
		t.Fatalf("investigating: %v", err)
	}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "service stopped", 0.9,
		[]incidents.RecommendedAction{{ActionID: "restart_service", Target: "OpenVPNService"}},
		[]string{"verify"}, "summary", time.Now().UTC(), "", "completed"); err != nil {
		t.Fatalf("save investigation: %v", err)
	}
	if _, _, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	return inc.IncidentID
}

func getCommands(t *testing.T, q *remediation.Queue, store *incidents.Store, deviceID string) PendingCommandsResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/devices/"+deviceID+"/commands", nil)
	req.SetPathValue("deviceId", deviceID)
	rec := httptest.NewRecorder()
	PendingCommandsHandler(q, store, nil)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("commands: got %d want 200", rec.Code)
	}
	var resp PendingCommandsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp
}

// TestDispatch_OnlyAfterApproval composes the real approval -> queue -> dispatch path:
// an approved incident becomes a dispatchable command and the dispatch moves it to
// executing, while an awaiting-approval incident never produces a command to dispatch.
func TestDispatch_OnlyAfterApproval(t *testing.T) {
	store := incidents.NewStore()
	q := remediation.NewQueue()

	// Approved incident on dev-1: a command can be built and queued (mirrors main.go's
	// onApproved wiring), then dispatched.
	approvedID := approvedIncidentForResult(t, store)
	approvedInc, _ := store.GetByID(approvedID)
	cmd, err := remediation.NewCommand(approvedInc)
	if err != nil {
		t.Fatalf("NewCommand for approved incident: %v", err)
	}
	q.Enqueue(cmd)

	resp := getCommands(t, q, store, "dev-1")
	if len(resp.Commands) != 1 || resp.Commands[0].IncidentID != approvedID {
		t.Fatalf("approved incident should dispatch exactly its command: %+v", resp.Commands)
	}
	dispatched, _ := store.GetByID(approvedID)
	if dispatched.State != incidents.StateExecuting {
		t.Fatalf("dispatch should move incident to executing, got %q", dispatched.State)
	}

	// Awaiting-approval incident on dev-2: the command gate rejects it, so nothing is
	// queued and the dispatch endpoint returns no commands for that device.
	awaitingID := awaitingApprovalIncident(t, store, "INC-2", "dev-2")
	awaitingInc, _ := store.GetByID(awaitingID)
	if _, err := remediation.NewCommand(awaitingInc); !errors.Is(err, remediation.ErrNotApproved) {
		t.Fatalf("expected ErrNotApproved for un-approved incident, got %v", err)
	}

	respDev2 := getCommands(t, q, store, "dev-2")
	if len(respDev2.Commands) != 0 {
		t.Fatalf("un-approved incident must not be dispatchable, got %+v", respDev2.Commands)
	}
	stillAwaiting, _ := store.GetByID(awaitingID)
	if stillAwaiting.State != incidents.StateAwaitingApproval {
		t.Fatalf("un-approved incident state should be unchanged, got %q", stillAwaiting.State)
	}
}
