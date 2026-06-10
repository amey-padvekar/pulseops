package remediation

import (
	"context"
	"errors"
	"testing"

	"github.com/certainelf/pulseops/agent/internal/platform"
)

// fakeRemediator records which platform operation the mapper invoked and how many
// times any operation ran (for duplicate-execution assertions).
type fakeRemediator struct {
	op     string
	target string
	calls  int
	out    platform.CommandResult
	err    error
}

func (f *fakeRemediator) RestartService(_ context.Context, serviceName string) (platform.CommandResult, error) {
	f.op, f.target = "restart", serviceName
	f.calls++
	return f.out, f.err
}

func (f *fakeRemediator) FlushDNS(context.Context) (platform.CommandResult, error) {
	f.op = "flush"
	f.calls++
	return f.out, f.err
}

func (f *fakeRemediator) ReconnectVPN(_ context.Context, serviceName string) (platform.CommandResult, error) {
	f.op, f.target = "reconnect", serviceName
	f.calls++
	return f.out, f.err
}

func TestMapper_Execute_MapsWhitelistedActions(t *testing.T) {
	cases := []struct {
		action     Action
		wantOp     string
		wantTarget string
	}{
		{Action{ActionID: ActionRestartService, Target: "OpenVPNService"}, "restart", "OpenVPNService"},
		{Action{ActionID: ActionFlushDNS}, "flush", ""},
		{Action{ActionID: ActionReconnectVPN, Target: "OpenVPNService"}, "reconnect", "OpenVPNService"},
	}
	for _, tc := range cases {
		fake := &fakeRemediator{out: platform.CommandResult{Stdout: "done", ExitCode: 0}}
		m := NewMapper(fake)
		out, err := m.Execute(context.Background(), tc.action)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.action.ActionID, err)
		}
		if out.Stdout != "done" {
			t.Fatalf("%s: output not returned: %+v", tc.action.ActionID, out)
		}
		if fake.op != tc.wantOp || fake.target != tc.wantTarget {
			t.Fatalf("%s mapped to op=%q target=%q want op=%q target=%q",
				tc.action.ActionID, fake.op, fake.target, tc.wantOp, tc.wantTarget)
		}
	}
}

func TestMapper_Execute_RejectsUnknownAction(t *testing.T) {
	fake := &fakeRemediator{}
	m := NewMapper(fake)

	_, err := m.Execute(context.Background(), Action{ActionID: "rm_rf_root", Target: "/"})
	if !errors.Is(err, ErrActionNotWhitelisted) {
		t.Fatalf("expected ErrActionNotWhitelisted, got %v", err)
	}
	if fake.op != "" {
		t.Fatal("a non-whitelisted action must not reach any platform operation")
	}
}

func TestMapper_Execute_RejectsMissingTarget(t *testing.T) {
	for _, id := range []string{ActionRestartService, ActionReconnectVPN} {
		fake := &fakeRemediator{}
		m := NewMapper(fake)
		if _, err := m.Execute(context.Background(), Action{ActionID: id}); !errors.Is(err, ErrMissingTarget) {
			t.Fatalf("%s: expected ErrMissingTarget, got %v", id, err)
		}
		if fake.op != "" {
			t.Fatalf("%s: must not invoke platform op without a target", id)
		}
	}
}

func TestWhitelist(t *testing.T) {
	if !IsWhitelisted(ActionRestartService) || !IsWhitelisted(ActionFlushDNS) || !IsWhitelisted(ActionReconnectVPN) {
		t.Fatal("MVP actions must be whitelisted")
	}
	if IsWhitelisted("danger") || IsWhitelisted("") {
		t.Fatal("non-catalog actions must not be whitelisted")
	}
	got := WhitelistedActionIDs()
	want := []string{"flush_dns", "reconnect_vpn", "restart_service"}
	if len(got) != len(want) {
		t.Fatalf("whitelist size: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("whitelist[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}
