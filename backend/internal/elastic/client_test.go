package elastic

import (
	"context"
	"testing"
	"time"
)

func TestDisabledClient_AllowsSafeIndexing(t *testing.T) {

	cfg := &Config{
		Enabled: false,
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	doc := TelemetryEventDocument{
		EventType:     "telemetry_received",
		Timestamp:     time.Now().UTC(),
		DeviceID:      "DEV-01",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
	}

	err = client.IndexTelemetryEvent(
		context.Background(),
		doc,
	)

	if err != nil {
		t.Fatalf(
			"disabled client should not fail indexing: %v",
			err,
		)
	}
}
