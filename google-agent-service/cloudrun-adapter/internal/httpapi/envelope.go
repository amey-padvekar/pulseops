package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
)

const (
	errCodeInvalidJSON        = "INVALID_JSON"
	errCodePayloadTooLarge    = "PAYLOAD_TOO_LARGE"
	errCodeValidationFailed   = "VALIDATION_FAILED"
	errCodeUnauthorized       = "UNAUTHORIZED"
	errCodeADKUpstream        = "ADK_UPSTREAM_ERROR"
	errCodeModelOutputInvalid = "MODEL_OUTPUT_INVALID"
)

func successEnvelope(requestID, traceID string, result domain.InvestigationResult) domain.InvestigateResponse {
	return domain.InvestigateResponse{
		RequestID: strings.TrimSpace(requestID),
		TraceID:   ensureTraceID(traceID),
		Status:    domain.Status{Transport: "success", Workflow: "completed"},
		Result:    &result,
	}
}

func errorEnvelope(requestID, traceID, code string) domain.InvestigateResponse {
	return domain.InvestigateResponse{
		RequestID: strings.TrimSpace(requestID),
		TraceID:   ensureTraceID(traceID),
		Status:    domain.Status{Transport: "error", Workflow: "failed"},
		Error: &domain.ErrorEnvelope{
			Code:    code,
			Message: safeErrorMessage(code),
		},
	}
}

func safeErrorMessage(code string) string {
	switch code {
	case errCodePayloadTooLarge:
		return "request payload too large"
	case errCodeADKUpstream:
		return "investigation request failed"
	case errCodeModelOutputInvalid:
		return "model output validation failed"
	case errCodeUnauthorized:
		return "unauthorized"
	case errCodeValidationFailed, errCodeInvalidJSON:
		return "request validation failed"
	default:
		return "request failed"
	}
}

func ensureTraceID(value string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}

	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return "trace-" + hex.EncodeToString(buf)
	}

	return fmt.Sprintf("trace-fallback-%d", time.Now().UnixNano())
}
