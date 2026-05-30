package agentbuilder

import (
	"fmt"
	"strings"
)

// FormatRequestLog returns a redacted, single-line summary for request logging.
func FormatRequestLog(req AgentBuilderRequest, endpoint string) string {
	return fmt.Sprintf(
		"agent_builder_request request_id=%s incident_id=%s device_id=%s service_name=%s incident_state=%s severity=%s endpoint=%s time_window_start=%s time_window_end=%s logs=%d actions=%d",
		req.RequestID,
		req.IncidentID,
		req.DeviceID,
		req.ServiceName,
		req.IncidentState,
		req.Severity,
		strings.TrimSpace(endpoint),
		req.TimeWindow.Start.Format(timeFormatRFC3339),
		req.TimeWindow.End.Format(timeFormatRFC3339),
		len(req.RecentLogs),
		len(req.AvailableActions),
	)
}

// FormatResponseLog returns a single-line summary for response logging.
func FormatResponseLog(resp AgentBuilderResponse) string {
	return fmt.Sprintf(
		"agent_builder_response request_id=%s trace_id=%s status_transport=%s status_workflow=%s received_at=%s",
		resp.RequestID,
		resp.TraceID,
		resp.Status.Transport,
		resp.Status.Workflow,
		resp.ReceivedAt.Format(timeFormatRFC3339),
	)
}

const timeFormatRFC3339 = "2006-01-02T15:04:05Z07:00"
