package api

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
)

func validRequest() ApprovalRequest {
	return ApprovalRequest{
		ApprovedBy:        "demo.operator",
		SelectedActionIDs: []string{"restart_service"},
		Note:              "looks correct",
	}
}

func TestApprovalRequest_Validate_Success(t *testing.T) {
	r := validRequest()
	r.Normalize()
	if err := r.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}
}

func TestApprovalRequest_Normalize_TrimsFields(t *testing.T) {
	r := ApprovalRequest{
		ApprovedBy:        "  demo.operator  ",
		SelectedActionIDs: []string{"  restart_service  "},
		Note:              "  note  ",
	}
	r.Normalize()
	if r.ApprovedBy != "demo.operator" {
		t.Fatalf("approvedBy not trimmed: %q", r.ApprovedBy)
	}
	if r.SelectedActionIDs[0] != "restart_service" {
		t.Fatalf("action id not trimmed: %q", r.SelectedActionIDs[0])
	}
	if r.Note != "note" {
		t.Fatalf("note not trimmed: %q", r.Note)
	}
}

func TestApprovalRequest_Validate_MissingApprover(t *testing.T) {
	r := validRequest()
	r.ApprovedBy = "   "
	r.Normalize()
	if err := r.Validate(); !errors.Is(err, ErrApprovalMissingApprover) {
		t.Fatalf("expected ErrApprovalMissingApprover, got %v", err)
	}
}

func TestApprovalRequest_Validate_NoActions(t *testing.T) {
	r := validRequest()
	r.SelectedActionIDs = nil
	r.Normalize()
	if err := r.Validate(); !errors.Is(err, ErrApprovalNoActions) {
		t.Fatalf("expected ErrApprovalNoActions, got %v", err)
	}
}

func TestApprovalRequest_Validate_BlankActionID(t *testing.T) {
	r := validRequest()
	r.SelectedActionIDs = []string{"restart_service", "  "}
	r.Normalize()
	if err := r.Validate(); !errors.Is(err, ErrApprovalBlankActionID) {
		t.Fatalf("expected ErrApprovalBlankActionID, got %v", err)
	}
}

func TestApprovalRequest_Validate_DuplicateActionID(t *testing.T) {
	r := validRequest()
	r.SelectedActionIDs = []string{"restart_service", "restart_service"}
	r.Normalize()
	if err := r.Validate(); !errors.Is(err, ErrApprovalDuplicateAction) {
		t.Fatalf("expected ErrApprovalDuplicateAction, got %v", err)
	}
}

func TestApprovalRequest_Validate_NoteTooLong(t *testing.T) {
	r := validRequest()
	r.Note = strings.Repeat("x", MaxApprovalNoteLength+1)
	r.Normalize()
	if err := r.Validate(); !errors.Is(err, ErrApprovalNoteTooLong) {
		t.Fatalf("expected ErrApprovalNoteTooLong, got %v", err)
	}

	// Exactly at the limit is allowed.
	r.Note = strings.Repeat("x", MaxApprovalNoteLength)
	if err := r.Validate(); err != nil {
		t.Fatalf("note at limit should be valid, got %v", err)
	}
}

func TestQueuedActionsFor_DerivesTargetsFromRecommendation(t *testing.T) {
	recommended := []incidents.RecommendedAction{
		{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart"},
		{ActionID: "flush_dns", Target: "endpoint", Description: "flush"},
	}

	queued, err := QueuedActionsFor(recommended, []string{"restart_service"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("expected 1 queued action, got %d", len(queued))
	}
	if queued[0].ActionID != "restart_service" || queued[0].Target != "OpenVPNService" {
		t.Fatalf("target not derived from recommendation: %+v", queued[0])
	}
}

func TestQueuedActionsFor_RejectsActionNotInRecommendation(t *testing.T) {
	recommended := []incidents.RecommendedAction{
		{ActionID: "restart_service", Target: "svc", Description: "restart"},
	}
	_, err := QueuedActionsFor(recommended, []string{"reconnect_vpn"})
	if !errors.Is(err, ErrApprovalActionNotInRecommendation) {
		t.Fatalf("expected ErrApprovalActionNotInRecommendation, got %v", err)
	}
}

func TestNewApprovalResponse_BuildsFromApprovedIncident(t *testing.T) {
	approvedAt := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	inc := incidents.Incident{
		IncidentID: "INC-1001",
		State:      incidents.StateApproved,
		ApprovedBy: "demo.operator",
		ApprovedAt: &approvedAt,
		RecommendedActions: []incidents.RecommendedAction{
			{ActionID: "restart_service", Target: "OpenVPNService", Description: "restart"},
			{ActionID: "flush_dns", Target: "endpoint", Description: "flush"},
		},
		ApprovedActions: []string{"restart_service"},
	}

	resp := NewApprovalResponse(inc)
	if resp.IncidentID != "INC-1001" || resp.State != "approved" || resp.ApprovedBy != "demo.operator" {
		t.Fatalf("unexpected response header fields: %+v", resp)
	}
	if !resp.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("approvedAt: got %v want %v", resp.ApprovedAt, approvedAt)
	}
	if len(resp.QueuedActions) != 1 || resp.QueuedActions[0].Target != "OpenVPNService" {
		t.Fatalf("queued actions not derived correctly: %+v", resp.QueuedActions)
	}
}
