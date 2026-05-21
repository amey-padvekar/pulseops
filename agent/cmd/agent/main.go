package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
)

// Config is a small startup config stub for Phase 1.
type Config struct {
	DeviceID             string
	HeartbeatIntervalSec int
}

func loadConfig() Config {
	cfg := Config{
		DeviceID:             getenvOrDefault("AGENT_DEVICE_ID", "DEV-AGENT-01"),
		HeartbeatIntervalSec: getenvIntOrDefault("AGENT_HEARTBEAT_INTERVAL_SEC", 10),
	}
	return cfg
}

func getenvOrDefault(key string, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvIntOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func main() {
	cfg := loadConfig()
	log.Printf("agent starting device_id=%s heartbeat_interval_sec=%d", cfg.DeviceID, cfg.HeartbeatIntervalSec)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	fmt.Println("agent shutting down")
}
