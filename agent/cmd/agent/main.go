package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/certainelf/pulseops/agent/internal/config"
	"github.com/certainelf/pulseops/agent/internal/telemetry"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load runtime config: %v", err)
	}

	runner, err := telemetry.NewRunner(cfg, log.Default())
	if err != nil {
		log.Fatalf("initialize telemetry runner: %v", err)
	}

	log.Printf(
		"agent starting env=%s device_id=%s heartbeat_interval_sec=%d monitored_service=%s backend_base_url=%s simulated_logs=%t network_check_host=%s timeout_ms=%d",
		cfg.AppEnv,
		cfg.DeviceID,
		cfg.HeartbeatIntervalSec,
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

	<-ctx.Done()
	if err := <-errCh; err != nil {
		log.Printf("telemetry runner stopped with error: %v", err)
	}
	fmt.Println("agent shutting down")
}
