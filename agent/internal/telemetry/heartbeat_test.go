package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/certainelf/pulseops/agent/internal/config"
	"github.com/certainelf/pulseops/agent/internal/platform"
)

type fakeSnapshotProvider struct {
	snapshot Snapshot
}

func (p *fakeSnapshotProvider) Snapshot(context.Context) Snapshot {
	return p.snapshot
}

type fakeServiceChecker struct {
	status string
	err    error
}

func (c *fakeServiceChecker) CheckService(context.Context, string) (string, error) {
	return c.status, c.err
}

type fakeLogCollector struct {
	logs []string
	err  error
}

func (c *fakeLogCollector) CollectRecent(context.Context, string, int) ([]string, error) {
	return c.logs, c.err
}

type fakeNetworkChecker struct {
	reachable bool
	err       error
}

func (c *fakeNetworkChecker) Reachable(context.Context, string) (bool, error) {
	return c.reachable, c.err
}

type fakeMetricsCollector struct {
	metrics platform.SystemMetrics
	err     error
}

func (c *fakeMetricsCollector) Collect(context.Context) (platform.SystemMetrics, error) {
	return c.metrics, c.err
}

func TestBuildPayloadMatchesPhase2Contract(t *testing.T) {
	cfg := config.RuntimeConfig{
		DeviceID:             "DEV-AGENT-01",
		MonitoredServiceName: "OpenVPNService",
	}

	payload := buildPayload(cfg, Snapshot{
		ServiceStatus:    "running",
		NetworkReachable: true,
		CPUUsage:         12.5,
		MemoryUsage:      43.2,
		RecentLogs:       []string{"one", "two"},
	}, time.Date(2026, time.May, 21, 12, 0, 0, 0, time.UTC))

	if payload.SchemaVersion != schemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", payload.SchemaVersion, schemaVersion)
	}
	if !payload.Heartbeat {
		t.Fatal("Heartbeat = false, want true")
	}
	if payload.Timestamp != "2026-05-21T12:00:00Z" {
		t.Fatalf("Timestamp = %q, want %q", payload.Timestamp, "2026-05-21T12:00:00Z")
	}
	if payload.ServiceName != "OpenVPNService" {
		t.Fatalf("ServiceName = %q, want %q", payload.ServiceName, "OpenVPNService")
	}
}

func TestClientSendRetriesTransientFailure(t *testing.T) {
	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/telemetry" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/telemetry")
		}
		if r.Header.Get("X-PulseOps-Request-ID") == "" {
			t.Fatal("missing X-PulseOps-Request-ID header")
		}

		attempt := attempts.Add(1)
		if r.Header.Get("X-PulseOps-Request-Attempt") != strconv.Itoa(int(attempt)) {
			t.Fatalf("X-PulseOps-Request-Attempt = %q, want %q", r.Header.Get("X-PulseOps-Request-Attempt"), strconv.Itoa(int(attempt)))
		}
		if attempt == 1 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte("temporary upstream issue"))
			return
		}

		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}

		var payload Payload
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}
		if payload.DeviceID != "DEV-AGENT-01" {
			t.Fatalf("payload.DeviceID = %q, want %q", payload.DeviceID, "DEV-AGENT-01")
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	client.retryBackoffs = []time.Duration{time.Millisecond}

	result, err := client.Send(context.Background(), Payload{
		SchemaVersion:    schemaVersion,
		DeviceID:         "DEV-AGENT-01",
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Heartbeat:        true,
		ServiceName:      "OpenVPNService",
		ServiceStatus:    statusUnknown,
		NetworkReachable: true,
		CPUUsage:         0,
		MemoryUsage:      0,
		RecentLogs:       []string{"placeholder"},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Attempts != 2 {
		t.Fatalf("Attempts = %d, want %d", result.Attempts, 2)
	}
	if result.StatusCode != http.StatusAccepted {
		t.Fatalf("StatusCode = %d, want %d", result.StatusCode, http.StatusAccepted)
	}
	if result.RequestID == "" {
		t.Fatal("RequestID = empty, want non-empty request id")
	}
}

func TestRunnerEmitsHeartbeatOnInterval(t *testing.T) {
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	cfg := config.RuntimeConfig{
		AppEnv:               "development",
		DeviceID:             "DEV-AGENT-01",
		HeartbeatInterval:    20 * time.Millisecond,
		HeartbeatIntervalSec: 1,
		MonitoredServiceName: "OpenVPNService",
		BackendBaseURL:       server.URL,
		RequestTimeout:       time.Second,
		RequestTimeoutMS:     1000,
		EnableSimulatedLogs:  true,
		NetworkCheckHost:     "8.8.8.8",
	}

	runner, err := NewRunner(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	runner.provider = &fakeSnapshotProvider{snapshot: Snapshot{ServiceStatus: "running", RecentLogs: []string{"ok"}}}

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()

	if err := runner.Run(ctx); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if requests.Load() < 2 {
		t.Fatalf("request count = %d, want at least %d", requests.Load(), 2)
	}
}

func TestNormalizeServiceStatus(t *testing.T) {
	if got := normalizeServiceStatus("RUNNING"); got != "running" {
		t.Fatalf("normalizeServiceStatus(RUNNING) = %q, want %q", got, "running")
	}
	if got := normalizeServiceStatus("weird"); got != statusUnknown {
		t.Fatalf("normalizeServiceStatus(weird) = %q, want %q", got, statusUnknown)
	}
}

func TestPlatformSnapshotProviderIncludesNetworkAndMetrics(t *testing.T) {
	cfg := config.RuntimeConfig{
		DeviceID:             "DEV-AGENT-01",
		MonitoredServiceName: "OpenVPNService",
		NetworkCheckHost:     "8.8.8.8",
		EnableSimulatedLogs:  true,
	}

	provider := newPlatformSnapshotProviderWithAdapters(
		cfg,
		log.New(io.Discard, "", 0),
		&fakeServiceChecker{status: "running"},
		&fakeLogCollector{logs: []string{"log-a"}},
		&fakeNetworkChecker{reachable: true},
		&fakeMetricsCollector{metrics: platform.SystemMetrics{CPUUsage: 21.5, MemoryUsage: 64.3}},
	)

	snapshot := provider.Snapshot(context.Background())

	if snapshot.ServiceStatus != "running" {
		t.Fatalf("ServiceStatus = %q, want %q", snapshot.ServiceStatus, "running")
	}
	if !snapshot.NetworkReachable {
		t.Fatal("NetworkReachable = false, want true")
	}
	if snapshot.CPUUsage != 21.5 {
		t.Fatalf("CPUUsage = %v, want %v", snapshot.CPUUsage, 21.5)
	}
	if snapshot.MemoryUsage != 64.3 {
		t.Fatalf("MemoryUsage = %v, want %v", snapshot.MemoryUsage, 64.3)
	}
	if len(snapshot.RecentLogs) != 1 {
		t.Fatalf("RecentLogs length = %d, want %d", len(snapshot.RecentLogs), 1)
	}
	if snapshot.RecentLogs[0] != "log-a" {
		t.Fatalf("RecentLogs[0] = %q, want %q", snapshot.RecentLogs[0], "log-a")
	}
}

func TestPlatformSnapshotProviderFallsBackOnErrors(t *testing.T) {
	cfg := config.RuntimeConfig{
		DeviceID:             "DEV-AGENT-01",
		MonitoredServiceName: "OpenVPNService",
		NetworkCheckHost:     "8.8.8.8",
		EnableSimulatedLogs:  false,
	}

	provider := newPlatformSnapshotProviderWithAdapters(
		cfg,
		log.New(io.Discard, "", 0),
		&fakeServiceChecker{err: errors.New("service unavailable")},
		&fakeLogCollector{err: errors.New("logs unavailable")},
		&fakeNetworkChecker{err: errors.New("network unavailable")},
		&fakeMetricsCollector{err: errors.New("metrics unavailable")},
	)

	snapshot := provider.Snapshot(context.Background())

	if snapshot.ServiceStatus != platform.ServiceStateUnknown {
		t.Fatalf("ServiceStatus = %q, want %q", snapshot.ServiceStatus, platform.ServiceStateUnknown)
	}
	if snapshot.NetworkReachable {
		t.Fatal("NetworkReachable = true, want false")
	}
	if snapshot.CPUUsage != 0 {
		t.Fatalf("CPUUsage = %v, want %v", snapshot.CPUUsage, 0)
	}
	if snapshot.MemoryUsage != 0 {
		t.Fatalf("MemoryUsage = %v, want %v", snapshot.MemoryUsage, 0)
	}
	if len(snapshot.RecentLogs) != 1 {
		t.Fatalf("RecentLogs length = %d, want %d", len(snapshot.RecentLogs), 1)
	}
	if snapshot.RecentLogs[0] != "no recent logs available" {
		t.Fatalf("RecentLogs[0] = %q, want %q", snapshot.RecentLogs[0], "no recent logs available")
	}
}

func TestPlatformSnapshotProviderUsesSimulatedLogsWhenEnabled(t *testing.T) {
	cfg := config.RuntimeConfig{
		DeviceID:             "DEV-AGENT-01",
		MonitoredServiceName: "OpenVPNService",
		NetworkCheckHost:     "8.8.8.8",
		EnableSimulatedLogs:  true,
	}

	provider := newPlatformSnapshotProviderWithAdapters(
		cfg,
		log.New(io.Discard, "", 0),
		&fakeServiceChecker{status: "running"},
		&fakeLogCollector{logs: []string{}},
		&fakeNetworkChecker{reachable: true},
		&fakeMetricsCollector{metrics: platform.SystemMetrics{CPUUsage: 1, MemoryUsage: 2}},
	)

	snapshot := provider.Snapshot(context.Background())

	if len(snapshot.RecentLogs) < 2 {
		t.Fatalf("RecentLogs length = %d, want at least 2", len(snapshot.RecentLogs))
	}
}

func TestNormalizeRecentLogs(t *testing.T) {
	logs := normalizeRecentLogs([]string{"  a   b  ", "", "x\n\ty"})
	if len(logs) != 2 {
		t.Fatalf("len(normalizeRecentLogs) = %d, want %d", len(logs), 2)
	}
	if logs[0] != "a b" {
		t.Fatalf("logs[0] = %q, want %q", logs[0], "a b")
	}
	if logs[1] != "x y" {
		t.Fatalf("logs[1] = %q, want %q", logs[1], "x y")
	}
}
