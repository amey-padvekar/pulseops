package platform

import (
	"context"
	"strings"
	"testing"
)

// recordingExecutor captures the argv of each RunCaptured call and returns a canned result.
type recordingExecutor struct {
	calls  [][]string
	result CommandResult
	err    error
}

func (e *recordingExecutor) RunCaptured(_ context.Context, name string, args ...string) (CommandResult, error) {
	e.calls = append(e.calls, append([]string{name}, args...))
	return e.result, e.err
}

func (e *recordingExecutor) last() []string {
	if len(e.calls) == 0 {
		return nil
	}
	return e.calls[len(e.calls)-1]
}

func TestWindowsRemediator_MapsToFixedCommands(t *testing.T) {
	exec := &recordingExecutor{result: CommandResult{Stdout: "ok"}}
	r := &WindowsRemediator{executor: exec}
	ctx := context.Background()

	if _, err := r.RestartService(ctx, "OpenVPNService"); err != nil {
		t.Fatalf("RestartService: %v", err)
	}
	got := strings.Join(r0last(exec), " ")
	if !strings.Contains(got, "powershell.exe") || !strings.Contains(got, "Restart-Service") || !strings.Contains(got, "OpenVPNService") {
		t.Fatalf("restart did not map to Restart-Service: %q", got)
	}
	// The service name must be a discrete argv element, not interpolated into a string.
	if !containsArg(exec.last(), "OpenVPNService") {
		t.Fatalf("service name not passed as a discrete argument: %v", exec.last())
	}

	if _, err := r.FlushDNS(ctx); err != nil {
		t.Fatalf("FlushDNS: %v", err)
	}
	if got := strings.Join(exec.last(), " "); !strings.Contains(got, "ipconfig") || !strings.Contains(got, "/flushdns") {
		t.Fatalf("flush did not map to ipconfig /flushdns: %q", got)
	}

	// ReconnectVPN is a bounded restart of the VPN service.
	if _, err := r.ReconnectVPN(ctx, "OpenVPNService"); err != nil {
		t.Fatalf("ReconnectVPN: %v", err)
	}
	if got := strings.Join(exec.last(), " "); !strings.Contains(got, "Restart-Service") {
		t.Fatalf("reconnect did not map to a service restart: %q", got)
	}
}

func TestLinuxRemediator_MapsToFixedCommands(t *testing.T) {
	exec := &recordingExecutor{result: CommandResult{Stdout: "ok"}}
	r := &LinuxRemediator{executor: exec}
	ctx := context.Background()

	_, _ = r.RestartService(ctx, "openvpn")
	if got := exec.last(); got[0] != "systemctl" || got[1] != "restart" || got[2] != "openvpn" {
		t.Fatalf("restart did not map to systemctl restart openvpn: %v", got)
	}

	_, _ = r.FlushDNS(ctx)
	if got := strings.Join(exec.last(), " "); !strings.Contains(got, "flush-caches") {
		t.Fatalf("flush did not map to a cache flush: %q", got)
	}
}

func TestUnsupportedRemediator_FailsSafely(t *testing.T) {
	r := &UnsupportedRemediator{os: "plan9"}
	ctx := context.Background()

	if _, err := r.RestartService(ctx, "svc"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
	if _, err := r.FlushDNS(ctx); err == nil {
		t.Fatal("expected unsupported platform error")
	}
	if _, err := r.ReconnectVPN(ctx, "svc"); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestNewRemediator_ReturnsNonNil(t *testing.T) {
	if NewRemediator(&recordingExecutor{}) == nil {
		t.Fatal("NewRemediator returned nil")
	}
}

func r0last(e *recordingExecutor) []string { return e.calls[0] }

func containsArg(argv []string, want string) bool {
	for _, a := range argv {
		if a == want {
			return true
		}
	}
	return false
}
