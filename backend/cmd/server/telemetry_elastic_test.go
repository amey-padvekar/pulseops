package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/certainelf/pulseops/backend/internal/elastic"
	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

type failingElasticClient struct{}

func (f *failingElasticClient) Enabled() bool {
	return true
}

func (f *failingElasticClient) IndexTelemetryEvent(
	ctx context.Context,
	doc elastic.TelemetryEventDocument,
) error {
	return errors.New("elastic unavailable")
}

func (f *failingElasticClient) IndexIncidentEvent(
	ctx context.Context,
	doc elastic.IncidentEventDocument,
) error {
	return errors.New("elastic unavailable")
}

func (f *failingElasticClient) IndexRecentLogs(
	ctx context.Context,
	deviceID string,
	serviceName string,
	incidentID string,
	logs []string,
) error {
	return errors.New("elastic unavailable")
}

func TestTelemetryHandler_ElasticFailureDoesNotBreakIngestion(
	t *testing.T,
) {

	deviceStore := store.NewDeviceStore()
	incidentStore := incidents.NewStore()

	handler := makeTelemetryHandler(
		deviceStore,
		incidentStore,
		ws.NewHub(),
		&failingElasticClient{},
	)

	body := `{
		"schemaVersion":"1.0.0",
		"deviceId":"DEV-01",
		"timestamp":"2026-05-25T10:00:00Z",
		"heartbeat":true,
		"serviceName":"OpenVPNService",
		"serviceStatus":"running",
		"networkReachable":true,
		"cpuUsage":10,
		"memoryUsage":20,
		"recentLogs":["heartbeat ok"]
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/telemetry",
		bytes.NewBufferString(body),
	)

	resp := httptest.NewRecorder()

	handler(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf(
			"expected telemetry acceptance despite elastic failure, got %d",
			resp.Code,
		)
	}

	_, ok := deviceStore.Get("DEV-01")

	if !ok {
		t.Fatal("expected telemetry to still update store")
	}
}
