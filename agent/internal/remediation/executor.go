package remediation

import (
	"context"
	"errors"
	"log"
	"time"
)

// ResultReporter delivers a completed execution result back to the backend. It is the
// seam for Phase 9 step 4.7; the Executor builds the result regardless of whether a
// reporter is configured.
type ResultReporter interface {
	Report(ctx context.Context, result ExecutionResult) error
}

// Executor runs an approved remediation Command action-by-action through the Mapper,
// captures each action's structured outcome (status, stdout, stderr, exit code,
// duration), and assembles a normalized ExecutionResult (Phase 9 step 4.6). It is the
// single controlled execution path for remediation and implements CommandHandler so the
// poller can drive it.
type Executor struct {
	mapper   *Mapper
	reporter ResultReporter
	now      func() time.Time
	logger   *log.Logger
	dedup    *requestDeduper
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithReporter attaches a result reporter (step 4.7).
func WithReporter(r ResultReporter) ExecutorOption {
	return func(e *Executor) { e.reporter = r }
}

// WithClock overrides the time source (for deterministic tests).
func WithClock(now func() time.Time) ExecutorOption {
	return func(e *Executor) { e.now = now }
}

// WithLogger overrides the logger.
func WithLogger(l *log.Logger) ExecutorOption {
	return func(e *Executor) { e.logger = l }
}

// NewExecutor builds an Executor over a Mapper.
func NewExecutor(mapper *Mapper, opts ...ExecutorOption) *Executor {
	e := &Executor{
		mapper: mapper,
		now:    time.Now,
		logger: log.Default(),
		dedup:  newRequestDeduper(),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Handle satisfies CommandHandler: it executes the command and, when a reporter is
// configured, reports the result. Errors from reporting are returned so the poller can
// log them; execution failures are captured inside the result, not returned as errors.
//
// A command whose request ID was already executed is ignored (Phase 9 task 4.10.2):
// the action is not run again and the duplicate is logged. The request ID is recorded
// as done after execution regardless of outcome, so a confirmed failed action is never
// silently re-run under the same ID.
func (e *Executor) Handle(ctx context.Context, cmd Command) error {
	if e.dedup.done(cmd.RequestID) {
		e.logger.Printf(
			"remediation duplicate ignored incident_id=%s request_id=%s reason=already_completed",
			cmd.IncidentID, cmd.RequestID,
		)
		return nil
	}

	result := e.Execute(ctx, cmd)
	e.dedup.markDone(cmd.RequestID)

	if e.reporter != nil {
		return e.reporter.Report(ctx, result)
	}
	return nil
}

// Execute runs every action in cmd and returns the assembled execution result. Logs are
// bounded and timestamps are UTC. The overall status is the worst per-action outcome.
func (e *Executor) Execute(ctx context.Context, cmd Command) ExecutionResult {
	started := e.now().UTC()

	results := make([]ActionResult, 0, len(cmd.Actions))
	for _, action := range cmd.Actions {
		results = append(results, e.runAction(ctx, action))
	}

	finished := e.now().UTC()
	result := ExecutionResult{
		IncidentID: cmd.IncidentID,
		DeviceID:   cmd.DeviceID,
		RequestID:  cmd.RequestID,
		Status:     overallStatus(results),
		StartedAt:  started,
		FinishedAt: finished,
		Results:    results,
	}

	e.logger.Printf(
		"remediation execution finished incident_id=%s device_id=%s request_id=%s status=%s actions=%d duration_ms=%d",
		result.IncidentID, result.DeviceID, result.RequestID, result.Status, len(result.Results),
		finished.Sub(started).Milliseconds(),
	)
	return result
}

// runAction executes a single action and normalizes its outcome into an ActionResult.
// Actions rejected before any command runs (unknown action, missing target) get a
// "rejected" status with no exit code/duration; a command that ran but failed gets
// "failed" with the captured exit code; a clean run gets "succeeded".
func (e *Executor) runAction(ctx context.Context, action Action) ActionResult {
	ar := ActionResult{ActionID: action.ActionID, Target: action.Target}

	res, err := e.mapper.Execute(ctx, action)

	if errors.Is(err, ErrActionNotWhitelisted) || errors.Is(err, ErrMissingTarget) {
		// Nothing executed: do not fabricate an exit code or duration.
		ar.Status = ExecStatusRejected
		ar.Stderr = BoundLog(err.Error())
		return ar
	}

	// A command was attempted — record its captured structured outcome.
	exit := res.ExitCode
	ar.ExitCode = &exit
	ar.DurationMs = res.Duration.Milliseconds()
	ar.Stdout = BoundLog(res.Stdout)
	ar.Stderr = BoundLog(res.Stderr)

	if err != nil {
		ar.Status = ExecStatusFailed
		if ar.Stderr == "" {
			ar.Stderr = BoundLog(err.Error())
		}
		return ar
	}

	ar.Status = ExecStatusSucceeded
	return ar
}

// overallStatus collapses per-action statuses into one command-level status: any
// failure makes the command failed; otherwise any rejection makes it rejected;
// otherwise it succeeded. An empty action set is treated as failed (nothing ran).
func overallStatus(results []ActionResult) ExecutionStatus {
	if len(results) == 0 {
		return ExecStatusFailed
	}
	worst := ExecStatusSucceeded
	for _, r := range results {
		switch r.Status {
		case ExecStatusFailed:
			return ExecStatusFailed
		case ExecStatusRejected:
			worst = ExecStatusRejected
		}
	}
	return worst
}
