package adk

import (
	"context"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

type Client interface {
	Investigate(ctx context.Context, req domain.InvestigateRequest) (domain.InvestigationResult, string, error)
}

type StubClient struct{}

func NewStubClient() *StubClient {
	return &StubClient{}
}

func (c *StubClient) Investigate(_ context.Context, req domain.InvestigateRequest) (domain.InvestigationResult, string, error) {
	target := req.Metadata.DeviceID
	for _, action := range req.AvailableActions {
		if action.ActionID == "restart_service" {
			if action.Target != "" {
				target = action.Target
			}
			break
		}
	}

	res := domain.InvestigationResult{
		ProbableCause: "service stopped",
		Confidence:    0.5,
		RecommendedActions: []domain.RecommendedAction{
			{ActionID: "restart_service", Target: target, Reason: "baseline scaffold response"},
		},
		ValidationSteps: []string{"verify service state is running"},
		Summary:         "Scaffold response from cloudrun adapter.",
	}
	return res, "trace-scaffold", nil
}
