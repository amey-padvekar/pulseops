package platform

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type OSCommandExecutor struct{}

func NewOSCommandExecutor() *OSCommandExecutor {
	return &OSCommandExecutor{}
}

func (e *OSCommandExecutor) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("command %s %v failed: %w", name, args, err)
	}
	return text, nil
}

// RunCaptured runs a command and captures its stdout, stderr, exit code, and wall-clock
// duration separately (Phase 9 step 4.6). It is the controlled execution primitive for
// remediation, where the structured outcome — not just combined text — must be reported.
//
// On a non-zero exit it returns the populated CommandResult alongside a non-nil error,
// so callers always get the captured detail even on failure. ExitCode is -1 when the
// process never started or was terminated by a signal.
func (e *OSCommandExecutor) RunCaptured(ctx context.Context, name string, args ...string) (CommandResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	duration := time.Since(start)

	result := CommandResult{
		Stdout:   strings.TrimSpace(stdout.String()),
		Stderr:   strings.TrimSpace(stderr.String()),
		Duration: duration,
		ExitCode: -1,
	}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return result, fmt.Errorf("command %s %v failed: %w", name, args, err)
	}
	return result, nil
}
