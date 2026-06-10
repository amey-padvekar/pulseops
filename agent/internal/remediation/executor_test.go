package remediation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/certainelf/pulseops/agent/internal/platform"
)

// stepClock returns a time source that advances by one second per call, so an
// ExecutionResult gets distinct started/finished timestamps deterministically.
func stepClock() func() time.Time {
	base := time.Date(2026, 5, 23, 22, 10, 0, 0, time.UTC)
	n := 0
	return func() time.Time {
		t := base.Add(time.Duration(n) * time.Second)
		n++
		return t
	}
}

func newTestExecutor(rem platform.Remediator) *Executor {
	return NewExecutor(NewMapper(rem), WithClock(stepClock()), WithLogger(quietLogger()))
}

func sampleCommand(actions ...Action) Command {
	return Command{IncidentID: "INC-1", DeviceID: "dev-1", RequestID: "rem-1", Actions: actions}
}

func TestExecutor_Execute_SuccessCapturesStructuredResult(t *testing.T) {
	rem := &fakeRemediator{out: platform.CommandResult{Stdout: "Service restarted", ExitCode: 0, Duration: 1500 * time.Millisecond}}
	e := newTestExecutor(rem)

	result := e.Execute(context.Background(), sampleCommand(Action{ActionID: ActionRestartService, Target: "OpenVPNService"}))

	if result.Status != ExecStatusSucceeded {
		t.Fatalf("status: got %q want succeeded", result.Status)
	}
	if result.StartedAt.IsZero() || !result.FinishedAt.After(result.StartedAt) {
		t.Fatalf("timestamps not set/ordered: %v -> %v", result.StartedAt, result.FinishedAt)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 action result, got %d", len(result.Results))
	}
	ar := result.Results[0]
	if ar.Status != ExecStatusSucceeded || ar.Stdout != "Service restarted" {
		t.Fatalf("action result not captured: %+v", ar)
	}
	if ar.ExitCode == nil || *ar.ExitCode != 0 {
		t.Fatalf("exit code not captured: %+v", ar.ExitCode)
	}
	if ar.DurationMs != 1500 {
		t.Fatalf("duration not captured: got %d want 1500", ar.DurationMs)
	}
}

func TestExecutor_Execute_FailureCapturesExitAndStderr(t *testing.T) {
	rem := &fakeRemediator{
		out: platform.CommandResult{Stderr: "access denied", ExitCode: 5, Duration: 200 * time.Millisecond},
		err: errors.New("command failed"),
	}
	e := newTestExecutor(rem)

	result := e.Execute(context.Background(), sampleCommand(Action{ActionID: ActionFlushDNS}))

	if result.Status != ExecStatusFailed {
		t.Fatalf("status: got %q want failed", result.Status)
	}
	ar := result.Results[0]
	if ar.Status != ExecStatusFailed {
		t.Fatalf("action status: got %q want failed", ar.Status)
	}
	if ar.ExitCode == nil || *ar.ExitCode != 5 {
		t.Fatalf("exit code not captured on failure: %+v", ar.ExitCode)
	}
	if ar.Stderr != "access denied" {
		t.Fatalf("stderr not captured: %q", ar.Stderr)
	}
}

func TestExecutor_Execute_RejectsUnknownActionWithoutRunning(t *testing.T) {
	rem := &fakeRemediator{}
	e := newTestExecutor(rem)

	result := e.Execute(context.Background(), sampleCommand(Action{ActionID: "danger", Target: "/"}))

	if result.Status != ExecStatusRejected {
		t.Fatalf("status: got %q want rejected", result.Status)
	}
	ar := result.Results[0]
	if ar.Status != ExecStatusRejected {
		t.Fatalf("action status: got %q want rejected", ar.Status)
	}
	if ar.ExitCode != nil || ar.DurationMs != 0 {
		t.Fatalf("rejected action must not carry exit code/duration: %+v", ar)
	}
	if rem.op != "" {
		t.Fatal("a rejected action must never reach a platform operation")
	}
}

func TestExecutor_Execute_OverallStatusFailureDominates(t *testing.T) {
	// First action succeeds, second is rejected (missing target): overall is failed only
	// if a failure exists; here no failure, so overall should be rejected.
	rem := &fakeRemediator{out: platform.CommandResult{Stdout: "ok", ExitCode: 0}}
	e := newTestExecutor(rem)

	result := e.Execute(context.Background(), sampleCommand(
		Action{ActionID: ActionFlushDNS},
		Action{ActionID: ActionRestartService}, // missing target -> rejected
	))
	if result.Status != ExecStatusRejected {
		t.Fatalf("status: got %q want rejected (no failures, one rejection)", result.Status)
	}
}

func TestExecutor_Handle_ReportsWhenReporterSet(t *testing.T) {
	rem := &fakeRemediator{out: platform.CommandResult{Stdout: "ok", ExitCode: 0}}
	reporter := &captureReporter{}
	e := NewExecutor(NewMapper(rem), WithClock(stepClock()), WithLogger(quietLogger()), WithReporter(reporter))

	if err := e.Handle(context.Background(), sampleCommand(Action{ActionID: ActionFlushDNS})); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if reporter.reported == nil {
		t.Fatal("reporter was not called")
	}
	if reporter.reported.IncidentID != "INC-1" {
		t.Fatalf("wrong result reported: %+v", reporter.reported)
	}
}

func TestExecutor_Handle_NoReporterIsNoError(t *testing.T) {
	rem := &fakeRemediator{out: platform.CommandResult{ExitCode: 0}}
	e := newTestExecutor(rem)
	if err := e.Handle(context.Background(), sampleCommand(Action{ActionID: ActionFlushDNS})); err != nil {
		t.Fatalf("Handle without reporter should not error: %v", err)
	}
}

type captureReporter struct {
	reported *ExecutionResult
	calls    int
}

func (c *captureReporter) Report(_ context.Context, result ExecutionResult) error {
	c.reported = &result
	c.calls++
	return nil
}

func TestExecutor_Handle_IgnoresDuplicateRequestID(t *testing.T) {
	rem := &fakeRemediator{out: platform.CommandResult{Stdout: "ok", ExitCode: 0}}
	reporter := &captureReporter{}
	e := NewExecutor(NewMapper(rem), WithClock(stepClock()), WithLogger(quietLogger()), WithReporter(reporter))

	cmd := sampleCommand(Action{ActionID: ActionFlushDNS})
	cmd.RequestID = "rem-dup"

	if err := e.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("first Handle: %v", err)
	}
	// Same request ID delivered again must not re-run the action or re-report.
	if err := e.Handle(context.Background(), cmd); err != nil {
		t.Fatalf("second Handle: %v", err)
	}

	if rem.calls != 1 {
		t.Fatalf("action ran %d times, want exactly 1 (duplicate must be ignored)", rem.calls)
	}
	if reporter.calls != 1 {
		t.Fatalf("reporter called %d times, want exactly 1", reporter.calls)
	}
}

func TestExecutor_Handle_FailedActionNotReRun(t *testing.T) {
	// A confirmed failed action must not be re-run under the same request ID (task 4.10.4).
	rem := &fakeRemediator{out: platform.CommandResult{Stderr: "boom", ExitCode: 1}, err: errContext()}
	e := NewExecutor(NewMapper(rem), WithClock(stepClock()), WithLogger(quietLogger()))

	cmd := sampleCommand(Action{ActionID: ActionFlushDNS})
	cmd.RequestID = "rem-fail"

	_ = e.Handle(context.Background(), cmd)
	_ = e.Handle(context.Background(), cmd)
	if rem.calls != 1 {
		t.Fatalf("failed action ran %d times, want exactly 1", rem.calls)
	}
}

func errContext() error { return errors.New("command failed") }
