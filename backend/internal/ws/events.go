package ws

import (
	"encoding/json"
	"log"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
)

const (
	EventTypeTelemetryUpdated = "telemetry.updated"
	EventTypeIncidentUpdated  = "incident.updated"
)

// EventEnvelope is the websocket message envelope sent to frontend clients.
type EventEnvelope struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// NewEventMessage builds a websocket message with a typed event envelope.
func NewEventMessage(eventType string, payload any) ([]byte, error) {
	return json.Marshal(EventEnvelope{
		Type:    eventType,
		Payload: payload,
	})
}

// BroadcastTelemetryUpdated emits telemetry updates in a typed envelope.
func BroadcastTelemetryUpdated(hub *Hub, state store.DeviceState) {
	broadcastTypedEvent(hub, EventTypeTelemetryUpdated, state)
}

// BroadcastIncidentUpdated emits incident updates in a typed envelope.
func BroadcastIncidentUpdated(hub *Hub, incident incidents.Incident) {
	broadcastTypedEvent(hub, EventTypeIncidentUpdated, incident)
}

func broadcastTypedEvent(hub *Hub, eventType string, payload any) {
	if hub == nil {
		return
	}

	msg, err := NewEventMessage(eventType, payload)
	if err != nil {
		log.Printf("ws events: failed to marshal %s event: %v", eventType, err)
		return
	}

	hub.Broadcast(msg)
}
