package safety

import (
	"errors"
	"strings"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

var (
	ErrMissingTask           = errors.New("task is required")
	ErrMissingPrompt         = errors.New("prompt is required")
	ErrMissingIncidentID     = errors.New("metadata.incident_id is required")
	ErrMissingDeviceID       = errors.New("metadata.device_id is required")
	ErrMissingRequestID      = errors.New("metadata.request_id is required")
	ErrEmptyAvailableActions = errors.New("available_actions must be non-empty")
)

func ValidateInvestigateRequest(req domain.InvestigateRequest) error {
	if strings.TrimSpace(req.Task) == "" {
		return ErrMissingTask
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return ErrMissingPrompt
	}
	if strings.TrimSpace(req.Metadata.IncidentID) == "" {
		return ErrMissingIncidentID
	}
	if strings.TrimSpace(req.Metadata.DeviceID) == "" {
		return ErrMissingDeviceID
	}
	if strings.TrimSpace(req.Metadata.RequestID) == "" {
		return ErrMissingRequestID
	}
	if len(req.AvailableActions) == 0 {
		return ErrEmptyAvailableActions
	}
	return nil
}
