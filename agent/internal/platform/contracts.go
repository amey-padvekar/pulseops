package platform

import "context"

const (
	ServiceStateRunning  = "running"
	ServiceStateStopped  = "stopped"
	ServiceStateDegraded = "degraded"
	ServiceStateUnknown  = "unknown"
)

// ServiceChecker inspects the current state of a monitored service.
type ServiceChecker interface {
	CheckService(ctx context.Context, serviceName string) (string, error)
}

// LogCollector retrieves recent service-related logs.
type LogCollector interface {
	CollectRecent(ctx context.Context, serviceName string, limit int) ([]string, error)
}

// CommandExecutor runs platform-specific system commands.
type CommandExecutor interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// NetworkChecker probes whether a target is currently reachable.
type NetworkChecker interface {
	Reachable(ctx context.Context, target string) (bool, error)
}

// SystemMetrics captures lightweight CPU and memory usage percentages.
type SystemMetrics struct {
	CPUUsage    float64
	MemoryUsage float64
}

// SystemMetricsCollector gathers coarse system usage values.
type SystemMetricsCollector interface {
	Collect(ctx context.Context) (SystemMetrics, error)
}
