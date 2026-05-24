package ws_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
	"github.com/certainelf/pulseops/backend/internal/ws"
)

type eventDecode struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

func TestNewEventMessage_EnvelopeShape(t *testing.T) {
	msg, err := ws.NewEventMessage(ws.EventTypeIncidentUpdated, map[string]string{"id": "inc-1"})
	if err != nil {
		t.Fatalf("NewEventMessage error: %v", err)
	}

	var envelope eventDecode
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("json.Unmarshal envelope: %v", err)
	}
	if envelope.Type != ws.EventTypeIncidentUpdated {
		t.Fatalf("type: got %q want %q", envelope.Type, ws.EventTypeIncidentUpdated)
	}

	var payload map[string]string
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("json.Unmarshal payload: %v", err)
	}
	if payload["id"] != "inc-1" {
		t.Fatalf("payload id: got %q", payload["id"])
	}
}

func TestBroadcastTelemetryUpdated_UsesTelemetryType(t *testing.T) {
	h := ws.NewHub()
	go h.Run()

	conn, cleanup := dialHub(t, h)
	defer cleanup()

	time.Sleep(50 * time.Millisecond)

	ws.BroadcastTelemetryUpdated(h, store.DeviceState{
		DeviceID:      "LAPTOP-22",
		ServiceName:   "OpenVPNService",
		ServiceStatus: "running",
		Heartbeat:     true,
	})

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	var envelope eventDecode
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("json.Unmarshal envelope: %v", err)
	}
	if envelope.Type != ws.EventTypeTelemetryUpdated {
		t.Fatalf("type: got %q want %q", envelope.Type, ws.EventTypeTelemetryUpdated)
	}
}

func TestBroadcastIncidentUpdated_UsesIncidentType(t *testing.T) {
	h := ws.NewHub()
	go h.Run()

	conn, cleanup := dialHub(t, h)
	defer cleanup()

	time.Sleep(50 * time.Millisecond)

	ws.BroadcastIncidentUpdated(h, incidents.NewIncidentAt(
		"inc-1",
		"LAPTOP-22",
		"OpenVPNService",
		"stopped",
		incidents.SeverityHigh,
		"service stopped while heartbeat is present",
		time.Now().UTC(),
	))

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	var envelope eventDecode
	if err := json.Unmarshal(msg, &envelope); err != nil {
		t.Fatalf("json.Unmarshal envelope: %v", err)
	}
	if envelope.Type != ws.EventTypeIncidentUpdated {
		t.Fatalf("type: got %q want %q", envelope.Type, ws.EventTypeIncidentUpdated)
	}
}
