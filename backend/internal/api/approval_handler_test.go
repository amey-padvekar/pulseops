package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/api"
	"github.com/certainelf/pulseops/backend/internal/incidents"
)

// awaitingIncident builds an incident that has completed investigation and been
// promoted to awaiting_approval with a concrete recommendation attached.
func awaitingIncident(t *testing.T, s *incidents.Store) incidents.Incident {
	t.Helper()

	inc, _ := s.CreateOrGetActive(
		"dev-1|OpenVPNService|service_stopped",
		incidents.NewIncidentAt("inc-1", "dev-1", "OpenVPNService", "stopped", incidents.SeverityHigh, "service stopped", time.Now().UTC().Add(-1*time.Minute)),
	)
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, "triage"); err != nil {
		t.Fatalf("investigating transition: %v", err)
	}
	recs := []incidents.RecommendedAction{
		{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart the crashed service"},
		{ActionID: "flush_dns", Target: "endpoint", Description: "flush resolver cache"},
	}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "service crashed", 0.82, recs, []string{"verify running"}, "summary", time.Now().UTC(), "trace-1", "completed"); err != nil {
		t.Fatalf("save investigation: %v", err)
	}
	if _, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil || !ok {
		t.Fatalf("promote to awaiting_approval: ok=%v err=%v", ok, err)
	}
	got, _ := s.GetByID(inc.IncidentID)
	return got
}

// approveRequest builds a POST request with the incidentId path value set,
// mirroring what the ServeMux pattern provides at runtime.
func approveRequest(incidentID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/incidents/"+incidentID+"/approve", strings.NewReader(body))
	req.SetPathValue("incidentId", incidentID)
	return req
}

func TestApprovalHandler_Success(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)

	var broadcast incidents.Incident
	broadcastCalled := false
	h := api.IncidentApprovalHandler(s, func(updated incidents.Incident) {
		broadcast = updated
		broadcastCalled = true
	})

	body := `{"approvedBy":"demo.operator","selectedActionIds":["restart_service"],"note":"looks good"}`
	rr := httptest.NewRecorder()
	h(rr, approveRequest(inc.IncidentID, body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", rr.Code, rr.Body.String())
	}

	var resp api.ApprovalResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.State != "approved" || resp.ApprovedBy != "demo.operator" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.ApprovedAt.IsZero() {
		t.Fatal("expected approvedAt to be set")
	}
	if len(resp.QueuedActions) != 1 || resp.QueuedActions[0].ActionID != "restart_service" || resp.QueuedActions[0].Target != "OpenVPNService" {
		t.Fatalf("queued actions not derived from recommendation: %+v", resp.QueuedActions)
	}
	if !broadcastCalled || broadcast.State != incidents.StateApproved {
		t.Fatalf("expected broadcast of approved incident, called=%v state=%q", broadcastCalled, broadcast.State)
	}

	// State persisted in the store.
	reread, _ := s.GetByID(inc.IncidentID)
	if reread.State != incidents.StateApproved {
		t.Fatalf("approval not persisted: %q", reread.State)
	}
}

func TestApprovalHandler_MethodNotAllowed(t *testing.T) {
	s := incidents.NewStore()
	h := api.IncidentApprovalHandler(s, nil)

	req := httptest.NewRequest(http.MethodGet, "/incidents/inc-1/approve", nil)
	req.SetPathValue("incidentId", "inc-1")
	rr := httptest.NewRecorder()
	h(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestApprovalHandler_InvalidBody(t *testing.T) {
	s := incidents.NewStore()
	h := api.IncidentApprovalHandler(s, nil)

	rr := httptest.NewRecorder()
	h(rr, approveRequest("inc-1", "{not json"))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestApprovalHandler_MissingApprover(t *testing.T) {
	s := incidents.NewStore()
	awaitingIncident(t, s)
	h := api.IncidentApprovalHandler(s, nil)

	rr := httptest.NewRecorder()
	h(rr, approveRequest("inc-1", `{"selectedActionIds":["restart_service"]}`))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing approver, got %d", rr.Code)
	}
}

func TestApprovalHandler_IncidentNotFound(t *testing.T) {
	s := incidents.NewStore()
	h := api.IncidentApprovalHandler(s, nil)

	rr := httptest.NewRecorder()
	h(rr, approveRequest("missing", `{"approvedBy":"op","selectedActionIds":["restart_service"]}`))

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestApprovalHandler_NoRecommendation(t *testing.T) {
	s := incidents.NewStore()
	// Incident exists but never got a recommendation: still investigating, empty recs.
	inc, _ := s.CreateOrGetActive("dev-1|svc|service_stopped",
		incidents.NewIncidentAt("inc-1", "dev-1", "svc", "stopped", incidents.SeverityHigh, "stopped", time.Now().UTC()))
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, "triage"); err != nil {
		t.Fatalf("investigating transition: %v", err)
	}
	h := api.IncidentApprovalHandler(s, nil)

	rr := httptest.NewRecorder()
	h(rr, approveRequest(inc.IncidentID, `{"approvedBy":"op","selectedActionIds":["restart_service"]}`))

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for missing recommendation, got %d (body=%s)", rr.Code, rr.Body.String())
	}
}

func TestApprovalHandler_ActionNotInRecommendation(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)
	h := api.IncidentApprovalHandler(s, nil)

	rr := httptest.NewRecorder()
	// reconnect_vpn is a valid catalog action but was not recommended for this incident.
	h(rr, approveRequest(inc.IncidentID, `{"approvedBy":"op","selectedActionIds":["reconnect_vpn"]}`))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for action outside recommendation, got %d", rr.Code)
	}

	// State must be unchanged after a rejected approval.
	reread, _ := s.GetByID(inc.IncidentID)
	if reread.State != incidents.StateAwaitingApproval {
		t.Fatalf("state changed on rejected approval: %q", reread.State)
	}
}

func TestApprovalHandler_DuplicateApproval(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)
	h := api.IncidentApprovalHandler(s, nil)

	body := `{"approvedBy":"op","selectedActionIds":["restart_service"]}`
	first := httptest.NewRecorder()
	h(first, approveRequest(inc.IncidentID, body))
	if first.Code != http.StatusOK {
		t.Fatalf("first approval expected 200, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	h(second, approveRequest(inc.IncidentID, body))
	if second.Code != http.StatusConflict {
		t.Fatalf("duplicate approval expected 409, got %d", second.Code)
	}
}

// TestApprovalHandler_InvalidStateTransition covers the Phase 8 task 4.10 "invalid
// state transition" matrix item at the HTTP layer: an incident that has a recommendation
// but has not been promoted to awaiting_approval must be rejected with 409, distinct
// from the already-approved (duplicate) path, and must not mutate state.
func TestApprovalHandler_InvalidStateTransition(t *testing.T) {
	s := incidents.NewStore()
	inc, _ := s.CreateOrGetActive("dev-1|OpenVPNService|service_stopped",
		incidents.NewIncidentAt("inc-1", "dev-1", "OpenVPNService", "stopped", incidents.SeverityHigh, "stopped", time.Now().UTC()))
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, "triage"); err != nil {
		t.Fatalf("investigating transition: %v", err)
	}
	// Recommendation present, but the incident is left in investigating (not promoted).
	recs := []incidents.RecommendedAction{{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart"}}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "cause", 0.8, recs, []string{"verify"}, "summary", time.Now().UTC(), "trace", "completed"); err != nil {
		t.Fatalf("save investigation: %v", err)
	}

	h := api.IncidentApprovalHandler(s, nil)
	rr := httptest.NewRecorder()
	h(rr, approveRequest(inc.IncidentID, `{"approvedBy":"op","selectedActionIds":["restart_service"]}`))

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409 for approval before awaiting_approval, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	reread, _ := s.GetByID(inc.IncidentID)
	if reread.State != incidents.StateInvestigating {
		t.Fatalf("state changed on rejected approval: got %q want investigating", reread.State)
	}
	if reread.ApprovedBy != "" || reread.ApprovedAt != nil {
		t.Fatalf("approval metadata leaked on rejected transition: approvedBy=%q approvedAt=%v", reread.ApprovedBy, reread.ApprovedAt)
	}
}

// TestApprovalHandler_RejectsActionNotInCatalog covers Phase 8 task 4.9.3: even when a
// selected action is present in the (poisoned) recommendation snapshot, it must be
// rejected if it is not in the backend remediation catalog — before any state change.
func TestApprovalHandler_RejectsActionNotInCatalog(t *testing.T) {
	s := incidents.NewStore()
	inc, _ := s.CreateOrGetActive("dev-1|svc|service_stopped",
		incidents.NewIncidentAt("inc-1", "dev-1", "svc", "stopped", incidents.SeverityHigh, "stopped", time.Now().UTC()))
	if _, err := s.UpdateState(inc.IncidentID, incidents.StateInvestigating, "triage"); err != nil {
		t.Fatalf("investigating transition: %v", err)
	}
	// Recommendation snapshot carrying a non-catalog action.
	poisoned := []incidents.RecommendedAction{{ActionID: "danger_cmd", Target: "host", Description: "not a catalog action"}}
	if _, err := s.SaveInvestigationResult(inc.IncidentID, "cause", 0.5, poisoned, []string{"verify"}, "summary", time.Now().UTC(), "trace", "completed"); err != nil {
		t.Fatalf("save investigation: %v", err)
	}
	if _, ok, err := s.PromoteToAwaitingApproval(inc.IncidentID); err != nil || !ok {
		t.Fatalf("promote: ok=%v err=%v", ok, err)
	}

	h := api.IncidentApprovalHandler(s, nil)
	rr := httptest.NewRecorder()
	h(rr, approveRequest(inc.IncidentID, `{"approvedBy":"op","selectedActionIds":["danger_cmd"]}`))

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-catalog action, got %d (body=%s)", rr.Code, rr.Body.String())
	}
	// State must be untouched: the catalog gate runs before any mutation.
	reread, _ := s.GetByID(inc.IncidentID)
	if reread.State != incidents.StateAwaitingApproval {
		t.Fatalf("state changed on rejected non-catalog approval: %q", reread.State)
	}
}

// TestApprovalHandler_IgnoresArbitraryClientFields covers Phase 8 task 4.9.1: the
// approval API surface cannot accept shell commands or arbitrary parameters. An extra
// "command" field in the body is silently ignored; only whitelisted action IDs drive
// the outcome.
func TestApprovalHandler_IgnoresArbitraryClientFields(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)
	h := api.IncidentApprovalHandler(s, nil)

	body := `{"approvedBy":"op","selectedActionIds":["restart_service"],"command":"rm -rf /","note":"x","extra":{"k":"v"}}`
	rr := httptest.NewRecorder()
	h(rr, approveRequest(inc.IncidentID, body))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (extra fields ignored), got %d (body=%s)", rr.Code, rr.Body.String())
	}
	var resp api.ApprovalResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Queued action derives only from the catalog/recommendation, never the "command" field.
	if len(resp.QueuedActions) != 1 || resp.QueuedActions[0].ActionID != "restart_service" {
		t.Fatalf("arbitrary client field leaked into queued actions: %+v", resp.QueuedActions)
	}
}

// TestApprovalHandler_RestDurabilityAfterApproval locks Phase 8 task 4.7.4: after an
// approval, a fresh REST read (the hard-refresh path) must reproduce the approved
// state and approval metadata, and the approved incident must still be returned by
// the active-incident list the dashboard fetches on load.
func TestApprovalHandler_RestDurabilityAfterApproval(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)

	approve := api.IncidentApprovalHandler(s, nil)
	body := `{"approvedBy":"demo.operator","selectedActionIds":["restart_service"],"note":"looks good"}`
	approveRR := httptest.NewRecorder()
	approve(approveRR, approveRequest(inc.IncidentID, body))
	if approveRR.Code != http.StatusOK {
		t.Fatalf("approve expected 200, got %d (body=%s)", approveRR.Code, approveRR.Body.String())
	}

	// Hard refresh, single incident: GET /incidents/{id}.
	byID := api.IncidentByIDHandler(s)
	getRR := httptest.NewRecorder()
	byID(getRR, httptest.NewRequest(http.MethodGet, "/incidents/"+inc.IncidentID, nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET by id expected 200, got %d", getRR.Code)
	}
	var got incidents.Incident
	if err := json.NewDecoder(getRR.Body).Decode(&got); err != nil {
		t.Fatalf("decode by-id: %v", err)
	}
	if got.State != incidents.StateApproved {
		t.Fatalf("state not durable: got %q", got.State)
	}
	if got.ApprovedBy != "demo.operator" || got.ApprovedAt == nil {
		t.Fatalf("approval metadata not durable: approvedBy=%q approvedAt=%v", got.ApprovedBy, got.ApprovedAt)
	}
	if len(got.ApprovedActions) != 1 || got.ApprovedActions[0] != "restart_service" {
		t.Fatalf("approved actions not durable: %v", got.ApprovedActions)
	}
	if got.ApprovalNote != "looks good" {
		t.Fatalf("approval note not durable: %q", got.ApprovalNote)
	}

	// Hard refresh, dashboard load: GET /incidents?active=true must still include it,
	// because approval is not a terminal state and keeps the incident active.
	list := api.IncidentsHandler(s)
	listRR := httptest.NewRecorder()
	list(listRR, httptest.NewRequest(http.MethodGet, "/incidents?active=true", nil))
	if listRR.Code != http.StatusOK {
		t.Fatalf("GET list expected 200, got %d", listRR.Code)
	}
	var listed []incidents.Incident
	if err := json.NewDecoder(listRR.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	found := false
	for _, item := range listed {
		if item.IncidentID == inc.IncidentID {
			found = true
			if item.State != incidents.StateApproved || item.ApprovedBy != "demo.operator" {
				t.Fatalf("listed incident missing durable approval: %+v", item)
			}
		}
	}
	if !found {
		t.Fatal("approved incident not returned by active-incident list after refresh")
	}
}

// TestApprovalHandler_RoutingThroughMux validates that the Go 1.22 method+wildcard
// pattern coexists with the /incidents/ subtree without a registration panic, and
// that POST approve and GET by-id both route correctly.
func TestApprovalHandler_RoutingThroughMux(t *testing.T) {
	s := incidents.NewStore()
	inc := awaitingIncident(t, s)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /incidents/{incidentId}/approve", api.IncidentApprovalHandler(s, nil))
	mux.HandleFunc("/incidents/", api.IncidentByIDHandler(s))

	// POST approve routes to the approval handler.
	body := `{"approvedBy":"op","selectedActionIds":["restart_service"]}`
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, httptest.NewRequest(http.MethodPost, "/incidents/"+inc.IncidentID+"/approve", bytes.NewBufferString(body)))
	if postRR.Code != http.StatusOK {
		t.Fatalf("POST approve through mux expected 200, got %d (body=%s)", postRR.Code, postRR.Body.String())
	}

	// GET by id still routes to the by-id handler.
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, httptest.NewRequest(http.MethodGet, "/incidents/"+inc.IncidentID, nil))
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET by id through mux expected 200, got %d", getRR.Code)
	}
	var got incidents.Incident
	if err := json.NewDecoder(getRR.Body).Decode(&got); err != nil {
		t.Fatalf("decode by-id response: %v", err)
	}
	if got.State != incidents.StateApproved {
		t.Fatalf("expected approved state after approval, got %q", got.State)
	}
}
