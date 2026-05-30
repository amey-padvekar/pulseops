package agentbuilder

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/incidents"
	"github.com/certainelf/pulseops/backend/internal/store"
)

const (
	defaultSchemaVersion  = "v1"
	defaultMaxLogs        = 10
	defaultTelemetryIndex = "telemetry-events-*"
	defaultIncidentsIndex = "incident-events-*"
	defaultLogsIndex      = "endpoint-logs-*"
	requestIDTimeFormat   = "20060102T150405.000000000"
)

// IncidentGetter fetches incidents by ID.
type IncidentGetter interface {
	GetByID(incidentID string) (incidents.Incident, bool)
}

// DeviceGetter fetches the latest device state by ID.
type DeviceGetter interface {
	Get(deviceID string) (store.DeviceState, bool)
}

// BuildRequestOptions controls how the request payload is assembled.
type BuildRequestOptions struct {
	RequestID          string
	RequestedAt        time.Time
	MaxLogs            int
	RecentLogs         []string
	AvailableActions   []ActionOption
	IndexPatterns      []string
	RecommendedQueries []string
	SchemaVersion      string
	ElasticIndexConfig *ElasticIndexConfig
}

// ElasticIndexConfig mirrors Elastic index base names for hint construction.
type ElasticIndexConfig struct {
	Telemetry string
	Incidents string
	Logs      string
}

// BuildRequest assembles an Agent Builder request using incident and device state.
func BuildRequest(
	incidentID string,
	incidentStore IncidentGetter,
	deviceStore DeviceGetter,
	opts BuildRequestOptions,
) (AgentBuilderRequest, error) {
	if incidentID == "" {
		return AgentBuilderRequest{}, errors.New("incidentID is required")
	}
	if incidentStore == nil {
		return AgentBuilderRequest{}, errors.New("incidentStore is required")
	}
	if deviceStore == nil {
		return AgentBuilderRequest{}, errors.New("deviceStore is required")
	}

	incident, ok := incidentStore.GetByID(incidentID)
	if !ok {
		return AgentBuilderRequest{}, fmt.Errorf("incident not found: %s", incidentID)
	}

	deviceID := strings.TrimSpace(incident.DeviceID)
	if deviceID == "" {
		return AgentBuilderRequest{}, fmt.Errorf("incident missing deviceId: %s", incidentID)
	}

	device, ok := deviceStore.Get(deviceID)
	if !ok {
		return AgentBuilderRequest{}, fmt.Errorf("device not found: %s", deviceID)
	}

	requestedAt := opts.RequestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Now().UTC()
	} else {
		requestedAt = requestedAt.UTC()
	}

	requestID := strings.TrimSpace(opts.RequestID)
	if requestID == "" {
		requestID = requestedAt.Format(requestIDTimeFormat)
	}

	schemaVersion := strings.TrimSpace(opts.SchemaVersion)
	if schemaVersion == "" {
		schemaVersion = defaultSchemaVersion
	}

	recentLogs := opts.RecentLogs
	if recentLogs == nil {
		recentLogs = device.RecentLogs
	}
	maxLogs := opts.MaxLogs
	if maxLogs <= 0 {
		maxLogs = defaultMaxLogs
	}
	recentLogs = normalizeLogs(recentLogs, maxLogs)

	actions := opts.AvailableActions
	if len(actions) == 0 {
		actions = defaultActionOptions()
	}

	windowStart, windowEnd := timeWindowForIncident(incident, requestedAt)

	serviceName := strings.TrimSpace(incident.ServiceName)
	if serviceName == "" {
		serviceName = strings.TrimSpace(device.ServiceName)
	}

	recommendedQueries := opts.RecommendedQueries
	if len(recommendedQueries) == 0 {
		recommendedQueries = defaultRecommendedQueries(deviceID, serviceName, incident.IncidentID)
	}

	indexPatterns := opts.IndexPatterns
	if len(indexPatterns) == 0 {
		indexPatterns = defaultIndexPatternsFromConfig(opts.ElasticIndexConfig)
	}

	snapshotTimestamp := device.LastSeenAt
	if snapshotTimestamp.IsZero() {
		snapshotTimestamp = requestedAt
	}

	return AgentBuilderRequest{
		SchemaVersion: schemaVersion,
		RequestID:     requestID,
		IncidentID:    incident.IncidentID,
		DeviceID:      deviceID,
		ServiceName:   serviceName,
		IncidentState: string(incident.State),
		Severity:      string(incident.Severity),
		RequestedAt:   requestedAt,
		TimeWindow: TimeWindow{
			Start: windowStart,
			End:   windowEnd,
		},
		TelemetrySnapshot: TelemetrySnapshot{
			Timestamp:        snapshotTimestamp,
			ServiceStatus:    strings.TrimSpace(device.ServiceStatus),
			NetworkReachable: device.NetworkReachable,
			CPUUsage:         device.CPUUsage,
			MemoryUsage:      device.MemoryUsage,
			Heartbeat:        device.Heartbeat,
		},
		RecentLogs:       recentLogs,
		IncidentSummary:  buildIncidentSummary(incident, deviceID, serviceName),
		AvailableActions: actions,
		ElasticContextHints: ElasticContextHints{
			DeviceID:           deviceID,
			IncidentID:         incident.IncidentID,
			ServiceName:        serviceName,
			TimeRangeStart:     windowStart,
			TimeRangeEnd:       windowEnd,
			IndexPatterns:      indexPatterns,
			RecommendedQueries: recommendedQueries,
		},
	}, nil
}

func normalizeLogs(logs []string, maxLogs int) []string {
	if len(logs) == 0 {
		return nil
	}

	clean := make([]string, 0, len(logs))
	for _, line := range logs {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		clean = append(clean, trimmed)
	}

	if len(clean) <= maxLogs {
		return clean
	}

	return append([]string{}, clean[len(clean)-maxLogs:]...)
}

func timeWindowForIncident(incident incidents.Incident, fallback time.Time) (time.Time, time.Time) {
	start := incident.DetectedAt
	if start.IsZero() {
		start = incident.CreatedAt
	}
	if start.IsZero() {
		start = fallback
	}

	end := incident.LastSeenAt
	if end.IsZero() {
		end = incident.UpdatedAt
	}
	if end.IsZero() {
		end = fallback
	}

	if end.Before(start) {
		end = start
	}

	return start.UTC(), end.UTC()
}

func buildIncidentSummary(incident incidents.Incident, deviceID string, serviceName string) string {
	reason := strings.TrimSpace(incident.Reason)
	if reason == "" {
		reason = "no reason provided"
	}

	return fmt.Sprintf(
		"Incident %s on %s/%s is %s (%s): %s",
		incident.IncidentID,
		deviceID,
		serviceName,
		strings.TrimSpace(string(incident.State)),
		strings.TrimSpace(string(incident.Severity)),
		reason,
	)
}

func defaultActionOptions() []ActionOption {
	return []ActionOption{
		{
			ActionID:         "restart_service",
			Target:           "service",
			Description:      "Restart the affected service",
			AllowedPlatforms: []string{"windows", "linux", "darwin"},
		},
		{
			ActionID:         "flush_dns",
			Target:           "network",
			Description:      "Flush DNS cache",
			AllowedPlatforms: []string{"windows", "linux", "darwin"},
		},
		{
			ActionID:         "reconnect_vpn",
			Target:           "network",
			Description:      "Reconnect VPN tunnel",
			AllowedPlatforms: []string{"windows", "linux", "darwin"},
		},
	}
}

func defaultIndexPatterns() []string {
	return []string{
		defaultTelemetryIndex,
		defaultIncidentsIndex,
		defaultLogsIndex,
	}
}

func defaultIndexPatternsFromConfig(cfg *ElasticIndexConfig) []string {
	if cfg == nil {
		return defaultIndexPatterns()
	}

	telemetry := indexPatternFromBase(cfg.Telemetry, defaultTelemetryIndex)
	incidents := indexPatternFromBase(cfg.Incidents, defaultIncidentsIndex)
	logs := indexPatternFromBase(cfg.Logs, defaultLogsIndex)

	return []string{telemetry, incidents, logs}
}

func indexPatternFromBase(base string, fallback string) string {
	clean := strings.TrimSpace(base)
	if clean == "" {
		return fallback
	}
	if strings.Contains(clean, "*") {
		return clean
	}
	return clean + "-*"
}

func defaultRecommendedQueries(deviceID string, serviceName string, incidentID string) []string {
	return []string{
		fmt.Sprintf("telemetry for device %s in incident window", deviceID),
		fmt.Sprintf("incident events for incident %s", incidentID),
		fmt.Sprintf("endpoint logs for %s/%s in incident window", deviceID, serviceName),
	}
}
