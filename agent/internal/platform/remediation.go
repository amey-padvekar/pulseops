package platform

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

// CommandResult is the structured outcome of one executed command (Phase 9 step 4.6):
// stdout and stderr captured separately, the process exit code, and wall-clock
// duration. ExitCode is -1 when the process never started or was signal-terminated.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
}

// CapturingCommandExecutor runs a command and returns its structured CommandResult.
// It is a richer sibling of CommandExecutor used by the remediation path, where exit
// code / separate streams / duration must be reported. *OSCommandExecutor implements
// both interfaces.
type CapturingCommandExecutor interface {
	RunCaptured(ctx context.Context, name string, args ...string) (CommandResult, error)
}

// Remediator performs bounded, explicit remediation operations. Each method maps to a
// fixed platform command sequence executed via CapturingCommandExecutor with discrete
// argv elements — never arbitrary shell text assembled at runtime. This keeps endpoint
// execution tightly bounded and auditable (Phase 9 step 4.5) while capturing a full
// structured result (step 4.6).
type Remediator interface {
	// RestartService restarts a single named service.
	RestartService(ctx context.Context, serviceName string) (CommandResult, error)
	// FlushDNS flushes the local DNS resolver cache.
	FlushDNS(ctx context.Context) (CommandResult, error)
	// ReconnectVPN re-establishes the VPN by restarting its service.
	ReconnectVPN(ctx context.Context, serviceName string) (CommandResult, error)
}

// NewRemediator selects the platform-specific implementation, mirroring how
// NewServiceChecker dispatches on runtime.GOOS. Unknown platforms get an
// implementation that fails safely rather than running anything.
func NewRemediator(executor CapturingCommandExecutor) Remediator {
	switch runtime.GOOS {
	case "windows":
		return &WindowsRemediator{executor: executor}
	case "linux":
		return &LinuxRemediator{executor: executor}
	default:
		return &UnsupportedRemediator{os: runtime.GOOS}
	}
}

// WindowsRemediator maps remediation operations to fixed Windows commands.
type WindowsRemediator struct {
	executor CapturingCommandExecutor
}

func (r *WindowsRemediator) RestartService(ctx context.Context, serviceName string) (CommandResult, error) {
	// Restart-Service blocks until the service is restarted, giving a deterministic
	// outcome. serviceName is passed as a discrete argument, not interpolated into a
	// shell string.
	return r.executor.RunCaptured(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Restart-Service", "-Name", serviceName, "-Force")
}

func (r *WindowsRemediator) FlushDNS(ctx context.Context) (CommandResult, error) {
	return r.executor.RunCaptured(ctx, "ipconfig", "/flushdns")
}

func (r *WindowsRemediator) ReconnectVPN(ctx context.Context, serviceName string) (CommandResult, error) {
	return r.RestartService(ctx, serviceName)
}

// LinuxRemediator maps remediation operations to fixed Linux commands.
type LinuxRemediator struct {
	executor CapturingCommandExecutor
}

func (r *LinuxRemediator) RestartService(ctx context.Context, serviceName string) (CommandResult, error) {
	return r.executor.RunCaptured(ctx, "systemctl", "restart", serviceName)
}

func (r *LinuxRemediator) FlushDNS(ctx context.Context) (CommandResult, error) {
	return r.executor.RunCaptured(ctx, "resolvectl", "flush-caches")
}

func (r *LinuxRemediator) ReconnectVPN(ctx context.Context, serviceName string) (CommandResult, error) {
	return r.RestartService(ctx, serviceName)
}

// UnsupportedRemediator fails safely on platforms with no defined mapping.
type UnsupportedRemediator struct {
	os string
}

func (r *UnsupportedRemediator) RestartService(context.Context, string) (CommandResult, error) {
	return CommandResult{ExitCode: -1}, r.unsupported("restart_service")
}

func (r *UnsupportedRemediator) FlushDNS(context.Context) (CommandResult, error) {
	return CommandResult{ExitCode: -1}, r.unsupported("flush_dns")
}

func (r *UnsupportedRemediator) ReconnectVPN(context.Context, string) (CommandResult, error) {
	return CommandResult{ExitCode: -1}, r.unsupported("reconnect_vpn")
}

func (r *UnsupportedRemediator) unsupported(op string) error {
	return fmt.Errorf("remediation %q is not supported on platform %q", op, r.os)
}
