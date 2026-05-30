package elastic

import (
	"context"
	"strings"
	"time"
)

const MaxLogsPerCycle = 10

func (c *Client) IndexRecentLogs(
	ctx context.Context,
	deviceID string,
	serviceName string,
	incidentID string,
	logs []string,
) error {

	if len(logs) == 0 {
		return nil
	}

	seen := make(map[string]struct{})

	start := 0
	if len(logs) > MaxLogsPerCycle {
		start = len(logs) - MaxLogsPerCycle
	}

	for _, line := range logs[start:] {

		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		if _, exists := seen[line]; exists {
			continue
		}

		seen[line] = struct{}{}

		doc := LogEventDocument{
			SchemaVersion: "v1",

			EventType: "endpoint_log",
			Timestamp: time.Now().UTC(),

			DeviceID:    deviceID,
			ServiceName: serviceName,
			IncidentID:  incidentID,

			Message: line,
			Source:  "agent_recent_logs",
		}

		if err := c.IndexLogEvent(ctx, doc); err != nil {
			return err
		}
	}

	return nil
}
