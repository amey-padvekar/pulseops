package remediation

import (
	"context"
	"log"
	"time"

	"github.com/certainelf/pulseops/agent/internal/config"
)

// maxPollBackoff caps the error backoff so the agent keeps trying at a sane floor rate
// even while the backend is unreachable.
const maxPollBackoff = 60 * time.Second

// CommandHandler processes a single received remediation command. It is the seam for
// Phase 9 execution (steps 4.5+); step 4.4 only needs to discover and log commands, so
// a nil handler is valid and means "receipt only".
type CommandHandler interface {
	Handle(ctx context.Context, cmd Command) error
}

// commandFetcher is the subset of Client the poller depends on, so the loop can be
// tested without real HTTP.
type commandFetcher interface {
	Fetch(ctx context.Context) ([]Command, error)
}

// Poller runs the agent's remediation command retrieval loop. It is independent of the
// telemetry heartbeat loop (it runs in its own goroutine), so polling and command
// handling never block heartbeat collection.
type Poller struct {
	deviceID string
	interval time.Duration
	fetcher  commandFetcher
	handler  CommandHandler
	logger   *log.Logger
}

// NewPoller wires a remediation poller for the given runtime config. handler may be
// nil to run in receipt-only mode (step 4.4); later phases supply an executor.
func NewPoller(cfg config.RuntimeConfig, fetcher commandFetcher, handler CommandHandler, logger *log.Logger) *Poller {
	if logger == nil {
		logger = log.Default()
	}
	return &Poller{
		deviceID: cfg.DeviceID,
		interval: cfg.RemediationPollInterval,
		fetcher:  fetcher,
		handler:  handler,
		logger:   logger,
	}
}

// Run polls for pending commands until ctx is canceled. On a successful poll it resets
// to the configured interval; on a fetch error it backs off exponentially (capped) so a
// flaky or down backend does not produce a tight error loop. It returns nil on
// graceful shutdown.
func (p *Poller) Run(ctx context.Context) error {
	delay := p.interval
	timer := time.NewTimer(0) // poll immediately on start
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}

		if err := p.pollOnce(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			delay = nextBackoff(delay)
			p.logger.Printf(
				"remediation poll failed device_id=%s error=%v next_retry_sec=%.0f",
				p.deviceID, err, delay.Seconds(),
			)
		} else {
			delay = p.interval
		}

		timer.Reset(delay)
	}
}

// pollOnce fetches pending commands and hands each to the handler. Command handling is
// sequential within this goroutine; because the poller runs separately from the
// heartbeat loop, slow handling never stalls telemetry.
func (p *Poller) pollOnce(ctx context.Context) error {
	commands, err := p.fetcher.Fetch(ctx)
	if err != nil {
		return err
	}

	for _, cmd := range commands {
		p.logger.Printf(
			"remediation command received device_id=%s incident_id=%s request_id=%s approved_by=%s actions=%d dispatched_at=%s",
			cmd.DeviceID, cmd.IncidentID, cmd.RequestID, cmd.ApprovedBy, len(cmd.Actions), cmd.DispatchedAt.Format(time.RFC3339),
		)

		if p.handler == nil {
			continue
		}
		if herr := p.handler.Handle(ctx, cmd); herr != nil {
			p.logger.Printf(
				"remediation command handling failed device_id=%s incident_id=%s request_id=%s error=%v",
				cmd.DeviceID, cmd.IncidentID, cmd.RequestID, herr,
			)
		}
	}
	return nil
}

// nextBackoff doubles the current delay up to maxPollBackoff.
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > maxPollBackoff {
		return maxPollBackoff
	}
	return next
}
