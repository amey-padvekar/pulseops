package telemetry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/certainelf/pulseops/agent/internal/config"
	"github.com/certainelf/pulseops/agent/internal/platform"
)

const (
	schemaVersion       = "1.0.0"
	defaultEndpoint     = "/telemetry"
	statusUnknown       = "unknown"
	maxRecentLogSize    = 5
	defaultLogLimit     = 10
	serviceCheckTimeout = 700 * time.Millisecond
	logCollectTimeout   = 600 * time.Millisecond
	networkCheckTimeout = 600 * time.Millisecond
	// CIM-based metrics collection on Windows commonly exceeds 1s.
	metricsCheckTimeout = 3 * time.Second
)

var defaultRetryBackoffs = []time.Duration{250 * time.Millisecond, 750 * time.Millisecond}

// Payload matches the shared telemetry schema for the Phase 2 heartbeat stream.
type Payload struct {
	SchemaVersion    string   `json:"schemaVersion"`
	DeviceID         string   `json:"deviceId"`
	Timestamp        string   `json:"timestamp"`
	Heartbeat        bool     `json:"heartbeat"`
	ServiceName      string   `json:"serviceName"`
	ServiceStatus    string   `json:"serviceStatus"`
	NetworkReachable bool     `json:"networkReachable"`
	CPUUsage         float64  `json:"cpuUsage"`
	MemoryUsage      float64  `json:"memoryUsage"`
	RecentLogs       []string `json:"recentLogs"`
}

// SnapshotProvider supplies the non-transport telemetry fields on each heartbeat.
type SnapshotProvider interface {
	Snapshot(context.Context) Snapshot
}

// Snapshot carries the mutable heartbeat details that later phases will populate with real checks.
type Snapshot struct {
	ServiceStatus    string
	NetworkReachable bool
	CPUUsage         float64
	MemoryUsage      float64
	RecentLogs       []string
}

// DeliveryResult reports how a heartbeat POST finished.
type DeliveryResult struct {
	Attempts   int
	StatusCode int
	RequestID  string
}

// Runner owns the ticker-driven heartbeat loop.
type Runner struct {
	config   config.RuntimeConfig
	provider SnapshotProvider
	client   *Client
	logger   *log.Logger
}

// Client posts telemetry payloads to the backend.
type Client struct {
	endpoint      string
	httpClient    *http.Client
	retryBackoffs []time.Duration
}

// NewRunner wires the Phase 2 heartbeat loop to the current config contract.
func NewRunner(cfg config.RuntimeConfig, logger *log.Logger) (*Runner, error) {
	client, err := NewClient(cfg.BackendBaseURL, cfg.RequestTimeout)
	if err != nil {
		return nil, err
	}

	return &Runner{
		config:   cfg,
		provider: NewPlatformSnapshotProvider(cfg, logger),
		client:   client,
		logger:   logger,
	}, nil
}

// Run starts the heartbeat loop and exits when the context is canceled.
func (r *Runner) Run(ctx context.Context) error {
	if err := r.emitOnce(ctx); err != nil {
		r.logger.Printf("heartbeat delivery failed: %v", err)
	}

	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.emitOnce(ctx); err != nil {
				r.logger.Printf("heartbeat delivery failed: %v", err)
			}
		}
	}
}

func (r *Runner) emitOnce(ctx context.Context) error {
	payload := buildPayload(r.config, r.provider.Snapshot(ctx), time.Now().UTC())
	result, err := r.client.Send(ctx, payload)
	if err != nil {
		return fmt.Errorf("device_id=%s request_id=%s attempts=%d: %w", r.config.DeviceID, result.RequestID, result.Attempts, err)
	}

	r.logger.Printf(
		"heartbeat delivery succeeded device_id=%s request_id=%s service=%s timestamp=%s attempts=%d status_code=%d",
		payload.DeviceID,
		result.RequestID,
		payload.ServiceName,
		payload.Timestamp,
		result.Attempts,
		result.StatusCode,
	)
	return nil
}

// NewClient builds the HTTP sender for telemetry heartbeats.
func NewClient(baseURL string, timeout time.Duration) (*Client, error) {
	endpoint, err := joinTelemetryEndpoint(baseURL)
	if err != nil {
		return nil, err
	}

	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		retryBackoffs: append([]time.Duration(nil), defaultRetryBackoffs...),
	}, nil
}

// Send posts one telemetry heartbeat with simple retry behavior for transient failures.
func (c *Client) Send(ctx context.Context, payload Payload) (DeliveryResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("marshal telemetry payload: %w", err)
	}

	requestID, err := newRequestID()
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("generate request id: %w", err)
	}

	attempts := len(c.retryBackoffs) + 1
	result := DeliveryResult{RequestID: requestID}

	for attempt := 1; attempt <= attempts; attempt++ {
		result.Attempts = attempt

		statusCode, sendErr := c.sendAttempt(ctx, body, payload, requestID, attempt)
		result.StatusCode = statusCode
		if sendErr == nil {
			return result, nil
		}

		if attempt == attempts || !isTransientStatus(statusCode) {
			return result, sendErr
		}

		if err := sleepWithContext(ctx, c.retryBackoffs[attempt-1]); err != nil {
			return result, err
		}
	}

	return result, fmt.Errorf("heartbeat delivery exhausted retries")
}

func (c *Client) sendAttempt(ctx context.Context, body []byte, payload Payload, requestID string, attempt int) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("build telemetry request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-PulseOps-Device-ID", payload.DeviceID)
	req.Header.Set("X-PulseOps-Schema-Version", payload.SchemaVersion)
	req.Header.Set("X-PulseOps-Request-ID", requestID)
	req.Header.Set("X-PulseOps-Request-Attempt", strconv.Itoa(attempt))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("backend connectivity error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if readErr != nil {
			return resp.StatusCode, fmt.Errorf("backend rejected telemetry with status %d", resp.StatusCode)
		}
		message := strings.TrimSpace(string(responseBody))
		if message == "" {
			message = http.StatusText(resp.StatusCode)
		}
		return resp.StatusCode, fmt.Errorf("backend rejected telemetry with status %d: %s", resp.StatusCode, message)
	}

	return resp.StatusCode, nil
}

func newRequestID() (string, error) {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func buildPayload(cfg config.RuntimeConfig, snapshot Snapshot, now time.Time) Payload {
	serviceStatus := strings.TrimSpace(snapshot.ServiceStatus)
	if serviceStatus == "" {
		serviceStatus = statusUnknown
	}

	recentLogs := snapshot.RecentLogs
	if recentLogs == nil {
		recentLogs = []string{}
	}

	return Payload{
		SchemaVersion:    schemaVersion,
		DeviceID:         cfg.DeviceID,
		Timestamp:        now.Format(time.RFC3339),
		Heartbeat:        true,
		ServiceName:      cfg.MonitoredServiceName,
		ServiceStatus:    serviceStatus,
		NetworkReachable: snapshot.NetworkReachable,
		CPUUsage:         snapshot.CPUUsage,
		MemoryUsage:      snapshot.MemoryUsage,
		RecentLogs:       truncateLogs(recentLogs, maxRecentLogSize),
	}
}

func joinTelemetryEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse backend base URL: %w", err)
	}

	joined, err := parsed.Parse(defaultEndpoint)
	if err != nil {
		return "", fmt.Errorf("build telemetry endpoint: %w", err)
	}

	return joined.String(), nil
}

func isTransientStatus(statusCode int) bool {
	if statusCode == 0 {
		return true
	}
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func truncateLogs(logs []string, maxItems int) []string {
	if len(logs) <= maxItems {
		return logs
	}
	return append([]string(nil), logs[len(logs)-maxItems:]...)
}

// PlatformSnapshotProvider composes platform adapters behind stable interfaces.
type PlatformSnapshotProvider struct {
	config           config.RuntimeConfig
	serviceChecker   platform.ServiceChecker
	logCollector     platform.LogCollector
	networkChecker   platform.NetworkChecker
	metricsCollector platform.SystemMetricsCollector
	count            int
	logger           *log.Logger
}

// NewPlatformSnapshotProvider wires service and log collection adapters for heartbeat snapshots.
func NewPlatformSnapshotProvider(cfg config.RuntimeConfig, logger *log.Logger) *PlatformSnapshotProvider {
	executor := platform.NewOSCommandExecutor()
	return newPlatformSnapshotProviderWithAdapters(
		cfg,
		logger,
		platform.NewServiceChecker(executor),
		platform.NewLogCollector(executor),
		platform.NewNetworkChecker(),
		platform.NewSystemMetricsCollector(executor),
	)
}

func newPlatformSnapshotProviderWithAdapters(
	cfg config.RuntimeConfig,
	logger *log.Logger,
	serviceChecker platform.ServiceChecker,
	logCollector platform.LogCollector,
	networkChecker platform.NetworkChecker,
	metricsCollector platform.SystemMetricsCollector,
) *PlatformSnapshotProvider {
	return &PlatformSnapshotProvider{
		config:           cfg,
		serviceChecker:   serviceChecker,
		logCollector:     logCollector,
		networkChecker:   networkChecker,
		metricsCollector: metricsCollector,
		logger:           logger,
	}
}

// Snapshot returns service-aware data while keeping other fields placeholder-safe for this phase slice.
func (p *PlatformSnapshotProvider) Snapshot(ctx context.Context) Snapshot {
	p.count++

	serviceStatus := platform.ServiceStateUnknown
	serviceCtx, cancelService := context.WithTimeout(ctx, serviceCheckTimeout)
	status, err := p.serviceChecker.CheckService(serviceCtx, p.config.MonitoredServiceName)
	cancelService()
	if err != nil {
		p.logger.Printf("service check failed service=%s err=%v", p.config.MonitoredServiceName, err)
	} else {
		serviceStatus = normalizeServiceStatus(status)
	}

	networkReachable := false
	networkCtx, cancelNetwork := context.WithTimeout(ctx, networkCheckTimeout)
	networkReachable, err = p.networkChecker.Reachable(networkCtx, p.config.NetworkCheckHost)
	cancelNetwork()
	if err != nil {
		p.logger.Printf("network check failed host=%s err=%v", p.config.NetworkCheckHost, err)
	}

	metrics := platform.SystemMetrics{}
	metricsCtx, cancelMetrics := context.WithTimeout(ctx, metricsCheckTimeout)
	metrics, err = p.metricsCollector.Collect(metricsCtx)
	cancelMetrics()
	if err != nil {
		p.logger.Printf("metrics collection failed err=%v", err)
	}

	logsCtx, cancelLogs := context.WithTimeout(ctx, logCollectTimeout)
	logs, err := p.logCollector.CollectRecent(logsCtx, p.config.MonitoredServiceName, defaultLogLimit)
	cancelLogs()
	if err != nil {
		p.logger.Printf("log collection failed service=%s err=%v", p.config.MonitoredServiceName, err)
		logs = []string{}
	}

	if p.config.EnableSimulatedLogs && len(logs) == 0 {
		logs = simulatedLogs(p.count, p.config.DeviceID, p.config.MonitoredServiceName, serviceStatus)
	}

	logs = normalizeRecentLogs(logs)
	if len(logs) == 0 {
		logs = []string{"no recent logs available"}
	}

	return Snapshot{
		ServiceStatus:    serviceStatus,
		NetworkReachable: networkReachable,
		CPUUsage:         metrics.CPUUsage,
		MemoryUsage:      metrics.MemoryUsage,
		RecentLogs:       logs,
	}
}

func normalizeServiceStatus(status string) string {
	normalized := strings.TrimSpace(strings.ToLower(status))
	if normalized == platform.ServiceStateRunning ||
		normalized == platform.ServiceStateStopped ||
		normalized == platform.ServiceStateDegraded ||
		normalized == platform.ServiceStateUnknown {
		return normalized
	}
	return statusUnknown
}

func simulatedLogs(cycle int, deviceID string, serviceName string, serviceStatus string) []string {
	return []string{
		fmt.Sprintf("cycle=%d service=%s status=%s", cycle, serviceName, serviceStatus),
		fmt.Sprintf("device=%s observed service heartbeat", deviceID),
	}
}

func normalizeRecentLogs(logs []string) []string {
	normalized := make([]string, 0, len(logs))
	for _, line := range logs {
		clean := strings.Join(strings.Fields(strings.TrimSpace(line)), " ")
		if clean == "" {
			continue
		}
		if len(clean) > 2048 {
			clean = clean[:2048]
		}
		normalized = append(normalized, clean)
	}
	return normalized
}
