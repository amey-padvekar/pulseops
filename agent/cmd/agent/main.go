package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/certainelf/pulseops/agent/internal/config"
	"github.com/certainelf/pulseops/agent/internal/platform"
	"github.com/certainelf/pulseops/agent/internal/remediation"
	"github.com/certainelf/pulseops/agent/internal/telemetry"
)

// loadDotEnv loads the nearest .env walking up from the working directory, so the
// agent picks up agent/.env whether run from the module root or cmd/agent. Real
// environment variables still win (godotenv does not overwrite existing vars).
func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for i := 0; i < 6; i++ {
		candidate := filepath.Join(dir, ".env")
		if _, statErr := os.Stat(candidate); statErr == nil {
			_ = godotenv.Load(candidate)
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

func main() {
	loadDotEnv()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load runtime config: %v", err)
	}

	runner, err := telemetry.NewRunner(cfg, log.Default())
	if err != nil {
		log.Fatalf("initialize telemetry runner: %v", err)
	}

	// Phase 9 step 4.4: the remediation poller discovers approved commands for this
	// device. It runs in its own goroutine so command retrieval never blocks heartbeat
	// collection.
	remediationClient, err := remediation.NewClient(
		cfg.BackendBaseURL,
		cfg.DeviceID,
		&http.Client{Timeout: cfg.RequestTimeout},
	)
	if err != nil {
		log.Fatalf("initialize remediation client: %v", err)
	}

	// Phase 9 steps 4.5/4.6/4.7: map approved action IDs to bounded platform operations,
	// execute them through one controlled execution path capturing structured results,
	// and report the outcome back to the backend.
	remediationReporter, err := remediation.NewHTTPReporter(
		cfg.BackendBaseURL,
		&http.Client{Timeout: cfg.RequestTimeout},
	)
	if err != nil {
		log.Fatalf("initialize remediation reporter: %v", err)
	}
	remediationExecutor := remediation.NewExecutor(
		remediation.NewMapper(platform.NewRemediator(platform.NewOSCommandExecutor())),
		remediation.WithLogger(log.Default()),
		remediation.WithReporter(remediationReporter),
	)
	remediationPoller := remediation.NewPoller(cfg, remediationClient, remediationExecutor, log.Default())

	log.Printf(
		"agent starting env=%s device_id=%s heartbeat_interval_sec=%d remediation_poll_interval_sec=%d monitored_service=%s backend_base_url=%s simulated_logs=%t network_check_host=%s timeout_ms=%d",
		cfg.AppEnv,
		cfg.DeviceID,
		cfg.HeartbeatIntervalSec,
		cfg.RemediationPollIntervalSec,
		cfg.MonitoredServiceName,
		cfg.BackendBaseURL,
		cfg.EnableSimulatedLogs,
		cfg.NetworkCheckHost,
		cfg.RequestTimeoutMS,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runner.Run(ctx)
	}()

	go func() {
		if err := remediationPoller.Run(ctx); err != nil {
			log.Printf("remediation poller stopped with error: %v", err)
		}
	}()

	<-ctx.Done()
	if err := <-errCh; err != nil {
		log.Printf("telemetry runner stopped with error: %v", err)
	}
	fmt.Println("agent shutting down")
}
