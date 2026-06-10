package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"pulseops/google-agent-service/cloudrun-adapter/internal/adk"
	"pulseops/google-agent-service/cloudrun-adapter/internal/domain"
	"pulseops/google-agent-service/cloudrun-adapter/internal/obs"
	"pulseops/google-agent-service/cloudrun-adapter/internal/safety"
)

const maxBodyBytes = 1 << 20

const (
	totalRequestBudget  = 10 * time.Second
	modelParseBudget    = 6 * time.Second
	maxTransientRetries = 1
)

type Handler struct {
	adk     adk.Client
	logger  *slog.Logger
	metrics *obs.Metrics
}

func NewHandler(client adk.Client, logger *slog.Logger) *Handler {
	return &Handler{adk: client, logger: logger, metrics: obs.NewMetrics()}
}

// Metrics exposes the registry so callers (e.g. controlled tests) can inspect counters.
func (h *Handler) Metrics() *obs.Metrics {
	return h.metrics
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/metrics", h.metricsEndpoint)
	mux.HandleFunc("/investigate", h.investigate)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) metricsEndpoint(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	h.metrics.WritePrometheus(w)
}

// requestObs accumulates the per-request observability fields emitted once, on exit.
type requestObs struct {
	start          time.Time
	requestID      string
	incidentID     string
	deviceID       string
	traceID        string
	confidence     float64
	actionIDs      []string
	enrichmentUsed bool
	evidenceLines  int
}

func (h *Handler) investigate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !isAuthorized(r, os.Getenv("INBOUND_AUTH_TOKEN")) {
		writeJSON(w, http.StatusUnauthorized, errorEnvelope("", "", errCodeUnauthorized))
		return
	}

	ro := &requestObs{start: time.Now()}

	// finish records metrics, emits the per-request structured log, and writes
	// the response exactly once for every terminal path of the handler.
	finish := func(status int, env domain.InvestigateResponse, outcome string) {
		ro.requestID = env.RequestID
		ro.traceID = env.TraceID
		if env.Result != nil {
			ro.confidence = env.Result.Confidence
			ro.actionIDs = actionIDsOf(env.Result.RecommendedActions)
		}
		latency := time.Since(ro.start)
		h.metrics.Observe(outcome, float64(latency.Milliseconds()))
		h.logRequest(env, ro, outcome, latency)
		writeJSON(w, status, env)
	}

	requestCtx, cancelRequest := context.WithTimeout(r.Context(), totalRequestBudget)
	defer cancelRequest()

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

	var req domain.InvestigateRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		statusCode, code := decodeError(err)
		finish(statusCode, errorEnvelope("", "", code), obs.OutcomeValidationFail)
		return
	}

	normalizeRequest(&req)
	ro.incidentID = req.Metadata.IncidentID
	ro.deviceID = req.Metadata.DeviceID
	ro.enrichmentUsed = enrichmentEligible(req)
	ro.evidenceLines = countEvidenceLines(req.EvidenceSummary)

	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		finish(http.StatusBadRequest, errorEnvelope(req.Metadata.RequestID, "", errCodeInvalidJSON), obs.OutcomeValidationFail)
		return
	}

	if err := safety.ValidateInvestigateRequest(req); err != nil {
		finish(http.StatusBadRequest, errorEnvelope(req.Metadata.RequestID, "", errCodeValidationFailed), obs.OutcomeValidationFail)
		return
	}

	result, traceID, err := h.investigateWithRetry(requestCtx, req)
	if err != nil {
		h.logger.Error("adk investigate failed", "request_id", req.Metadata.RequestID, "error", err)
		outcome := obs.OutcomeFail
		if isTimeoutErr(err) {
			outcome = obs.OutcomeTimeout
		}
		finish(http.StatusBadGateway, errorEnvelope(req.Metadata.RequestID, traceID, errCodeADKUpstream), outcome)
		return
	}

	if err := safety.ValidateInvestigationResult(result, req.AvailableActions); err != nil {
		h.logger.Error("invalid model output", "request_id", req.Metadata.RequestID, "error", err)
		finish(http.StatusBadGateway, errorEnvelope(req.Metadata.RequestID, traceID, errCodeModelOutputInvalid), obs.OutcomeValidationFail)
		return
	}

	finish(http.StatusOK, successEnvelope(req.Metadata.RequestID, traceID, result), obs.OutcomeSuccess)
}

// logRequest emits the single Phase E2 per-request log line with all required fields.
func (h *Handler) logRequest(env domain.InvestigateResponse, ro *requestObs, outcome string, latency time.Duration) {
	h.logger.Info("investigate_request",
		"request_id", ro.requestID,
		"incident_id", ro.incidentID,
		"device_id", ro.deviceID,
		"trace_id", ro.traceID,
		"status_transport", env.Status.Transport,
		"status_workflow", env.Status.Workflow,
		"latency_ms", latency.Milliseconds(),
		"confidence", ro.confidence,
		"action_ids", ro.actionIDs,
		"enrichment_used", ro.enrichmentUsed,
		"evidence_lines", ro.evidenceLines,
		"outcome", outcome,
	)
}

func actionIDsOf(actions []domain.RecommendedAction) []string {
	ids := make([]string, 0, len(actions))
	for _, a := range actions {
		ids = append(ids, a.ActionID)
	}
	return ids
}

// enrichmentEligible reports whether Elastic MCP enrichment was both enabled and
// supplied with usable hints for this request. It mirrors the agent-side gate so
// the adapter log truthfully reflects whether enrichment was attempted.
func enrichmentEligible(req domain.InvestigateRequest) bool {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("ELASTIC_MCP_ENABLED")), "true") {
		return false
	}
	h := req.ElasticContextHints
	return h.DeviceID != "" || h.IncidentID != "" || h.ServiceName != "" ||
		len(h.IndexPatterns) > 0 || len(h.RecommendedQueries) > 0
}

func countEvidenceLines(summary string) int {
	count := 0
	for _, line := range strings.Split(summary, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func isTimeoutErr(err error) bool {
	return errors.Is(err, context.DeadlineExceeded)
}

func isAuthorized(r *http.Request, expectedToken string) bool {
	expectedToken = strings.TrimSpace(expectedToken)
	if expectedToken == "" {
		return true
	}

	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if authorization == "" {
		return false
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authorization, bearerPrefix) {
		return false
	}

	receivedToken := strings.TrimSpace(strings.TrimPrefix(authorization, bearerPrefix))
	if receivedToken == "" {
		return false
	}

	return receivedToken == expectedToken
}

func (h *Handler) investigateWithRetry(ctx context.Context, req domain.InvestigateRequest) (domain.InvestigationResult, string, error) {
	deadline := time.Now().Add(modelParseBudget)
	var lastErr error
	var traceID string

	for attempt := 0; attempt <= maxTransientRetries; attempt++ {
		attemptCtx, cancel := context.WithDeadline(ctx, deadline)
		result, tID, err := h.adk.Investigate(attemptCtx, req)
		cancel()
		traceID = tID

		if err == nil {
			return result, traceID, nil
		}

		lastErr = err
		if attempt == maxTransientRetries || !adk.IsTransient(err) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
	}

	return domain.InvestigationResult{}, traceID, lastErr
}

func normalizeRequest(req *domain.InvestigateRequest) {
	req.Task = strings.TrimSpace(req.Task)
	req.Prompt = strings.TrimSpace(req.Prompt)
	req.Metadata.IncidentID = strings.TrimSpace(req.Metadata.IncidentID)
	req.Metadata.DeviceID = strings.TrimSpace(req.Metadata.DeviceID)
	req.Metadata.RequestID = strings.TrimSpace(req.Metadata.RequestID)

	req.EvidenceSummary = strings.TrimSpace(req.EvidenceSummary)
	if req.EvidenceSummary == "" {
		req.EvidenceSummary = "No evidence summary provided by caller."
	}

	req.ElasticContextHints.DeviceID = strings.TrimSpace(req.ElasticContextHints.DeviceID)
	req.ElasticContextHints.IncidentID = strings.TrimSpace(req.ElasticContextHints.IncidentID)
	req.ElasticContextHints.ServiceName = strings.TrimSpace(req.ElasticContextHints.ServiceName)
	req.ElasticContextHints.TimeRangeStart = strings.TrimSpace(req.ElasticContextHints.TimeRangeStart)
	req.ElasticContextHints.TimeRangeEnd = strings.TrimSpace(req.ElasticContextHints.TimeRangeEnd)
	if req.ElasticContextHints.IndexPatterns == nil {
		req.ElasticContextHints.IndexPatterns = []string{}
	}
	if req.ElasticContextHints.RecommendedQueries == nil {
		req.ElasticContextHints.RecommendedQueries = []string{}
	}
}

func decodeError(err error) (int, string) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return http.StatusRequestEntityTooLarge, errCodePayloadTooLarge
	}
	return http.StatusBadRequest, errCodeInvalidJSON
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
