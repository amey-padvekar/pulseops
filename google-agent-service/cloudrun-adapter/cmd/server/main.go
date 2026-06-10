package main

import (
	"log"
	"net/http"
	"os"
	"time"

	adapteradk "pulseops/google-agent-service/cloudrun-adapter/internal/adk"
	"pulseops/google-agent-service/cloudrun-adapter/internal/httpapi"
	"pulseops/google-agent-service/cloudrun-adapter/internal/obs"
	"pulseops/google-agent-service/cloudrun-adapter/internal/safety"
)

func main() {
	logger := obs.NewLogger()

	// Phase E3 deployment gate: refuse to start an unauthenticated service in production.
	sec := safety.CheckSecurityConfig(os.Getenv)
	for _, warn := range sec.Warnings {
		logger.Warn("security check warning", "detail", warn)
	}
	if !sec.OK() {
		for _, e := range sec.Errors {
			logger.Error("security check failed", "detail", e)
		}
		log.Fatal("startup security check failed")
	}

	client := adapteradk.NewStubClient()
	handler := httpapi.NewHandler(client, logger)

	mux := http.NewServeMux()
	handler.Register(mux)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	logger.Info("cloudrun adapter starting", "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
