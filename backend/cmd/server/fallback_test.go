package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/agentbuilder"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

// TestSubmitAgentBuilderRequest_Fallback ensures that when the agent client fails
// and AGENT_BUILDER_FALLBACK_MODE=local_stub, the fallback investigation result
// is saved into the incident store.
func TestSubmitAgentBuilderRequest_Fallback(t *testing.T) {
	// arrange
	os.Setenv("AGENT_BUILDER_FALLBACK_MODE", "local_stub")
	defer os.Unsetenv("AGENT_BUILDER_FALLBACK_MODE")

	incidentStore := incidents.NewStore()
	deviceStore := store.NewDeviceStore()
	hub := ws.NewHub()

	seed := incidents.NewIncident("", "dev-x", "svc-x", "stopped", incidents.SeverityMedium, "reason")
	inc, _ := incidentStore.CreateOrGetActive("k2", seed)

	// ensure device store has the device so agentbuilder request can be built
	deviceStore.Upsert(store.DeviceState{
		DeviceID:      "dev-x",
		ServiceName:   "svc-x",
		ServiceStatus: "stopped",
		RecentLogs:    []string{"seed"},
	})

	// stub client returns error
	stub := &agentbuilder.StubClient{Err: errStub}

	agentCfg := &agentbuilder.Config{Enabled: true, Timeout: 50 * time.Millisecond, Endpoint: "http://stub"}

	// act
	submitAgentBuilderRequest(stub, agentCfg, incidentStore, deviceStore, inc, nil, hub, nil)

	// assert: incident should have fallback result saved
	got, ok := incidentStore.GetByID(inc.IncidentID)
	if !ok {
		t.Fatalf("incident missing after request")
	}
	if got.InvestigationStatus != "fallback" && got.InvestigationStatus != "completed" {
		t.Fatalf("unexpected investigation status: %s", got.InvestigationStatus)
	}
}

// TestSubmitAgentBuilderRequest_FallbackWithActionsPromotes ensures the demo-only
// local_stub_actions mode synthesizes a concrete, catalog-valid recommendation and
// promotes the incident to awaiting_approval, so the Phase 8 approval gate is reachable
// offline. It mirrors the real flow where the incident is investigating at this point.
func TestSubmitAgentBuilderRequest_FallbackWithActionsPromotes(t *testing.T) {
	os.Setenv("AGENT_BUILDER_FALLBACK_MODE", "local_stub_actions")
	defer os.Unsetenv("AGENT_BUILDER_FALLBACK_MODE")

	incidentStore := incidents.NewStore()
	deviceStore := store.NewDeviceStore()
	hub := ws.NewHub()

	seed := incidents.NewIncident("", "dev-act", "OpenVPNService", "stopped", incidents.SeverityHigh, "reason")
	inc, _ := incidentStore.CreateOrGetActive("k-actions", seed)
	if _, err := incidentStore.UpdateState(inc.IncidentID, incidents.StateInvestigating, "triage"); err != nil {
		t.Fatalf("transition to investigating: %v", err)
	}

	deviceStore.Upsert(store.DeviceState{
		DeviceID:      "dev-act",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "stopped",
		RecentLogs:    []string{"seed"},
	})

	stub := &agentbuilder.StubClient{Err: errStub}
	agentCfg := &agentbuilder.Config{Enabled: true, Timeout: 50 * time.Millisecond, Endpoint: "http://stub"}

	submitAgentBuilderRequest(stub, agentCfg, incidentStore, deviceStore, inc, nil, hub, nil)

	got, ok := incidentStore.GetByID(inc.IncidentID)
	if !ok {
		t.Fatalf("incident missing after request")
	}
	if got.InvestigationStatus != "completed" {
		t.Fatalf("investigation status = %q, want %q", got.InvestigationStatus, "completed")
	}
	if len(got.RecommendedActions) == 0 || got.RecommendedActions[0].ActionID != "restart_service" {
		t.Fatalf("expected restart_service recommendation, got %+v", got.RecommendedActions)
	}
	if got.RecommendedActions[0].Target != "OpenVPNService" {
		t.Fatalf("expected target from incident service name, got %q", got.RecommendedActions[0].Target)
	}
	if got.State != incidents.StateAwaitingApproval {
		t.Fatalf("state = %q, want %q", got.State, incidents.StateAwaitingApproval)
	}
}

var errStub = &agentbuilder.ParseError{Err: os.ErrPermission}

type timeoutClient struct{}

func (c timeoutClient) SubmitInvestigation(ctx context.Context, req agentbuilder.AgentBuilderRequest) (agentbuilder.AgentBuilderResponse, error) {
	<-ctx.Done()
	return agentbuilder.AgentBuilderResponse{RequestID: req.RequestID, TraceID: "trace-timeout"}, ctx.Err()
}

func TestSubmitAgentBuilderRequest_TimeoutStoresFailureAndKeepsIncidentActionable(t *testing.T) {
	os.Unsetenv("AGENT_BUILDER_FALLBACK_MODE")

	incidentStore := incidents.NewStore()
	deviceStore := store.NewDeviceStore()
	hub := ws.NewHub()

	seed := incidents.NewIncident("", "dev-timeout", "svc-timeout", "stopped", incidents.SeverityHigh, "reason")
	inc, _ := incidentStore.CreateOrGetActive("k-timeout", seed)

	deviceStore.Upsert(store.DeviceState{
		DeviceID:      "dev-timeout",
		ServiceName:   "svc-timeout",
		ServiceStatus: "stopped",
		RecentLogs:    []string{"seed"},
	})

	agentCfg := &agentbuilder.Config{Enabled: true, Timeout: 5 * time.Millisecond, Endpoint: "http://stub"}
	submitAgentBuilderRequest(timeoutClient{}, agentCfg, incidentStore, deviceStore, inc, nil, hub, nil)

	got, ok := incidentStore.GetByID(inc.IncidentID)
	if !ok {
		t.Fatalf("incident missing after timeout")
	}
	if got.InvestigationStatus != "timeout" {
		t.Fatalf("investigation status = %q, want %q", got.InvestigationStatus, "timeout")
	}
	if got.AgentBuilderTraceID != "trace-timeout" {
		t.Fatalf("trace id = %q, want %q", got.AgentBuilderTraceID, "trace-timeout")
	}
	if got.InvestigationError == "" {
		t.Fatalf("expected investigation error on timeout")
	}
	if !got.Active {
		t.Fatalf("incident should remain actionable (active=true)")
	}
	if got.State != incidents.StateDetected {
		t.Fatalf("incident lifecycle state changed: got %q want %q", got.State, incidents.StateDetected)
	}
}
