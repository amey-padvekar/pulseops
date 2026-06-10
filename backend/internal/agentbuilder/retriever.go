package agentbuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/certainelf/pulseops/backend/internal/elastic"
)

const (
	defaultQueryWindowMinutes = 30
	maxSummaryHitsPerSection  = 5
)

// RetrieveAndSummarizeEvidence queries Elastic for telemetry, incidents and logs
// using the provided hints, and returns a compact, operator-friendly summary
// suitable for including in the Gemini prompt.
func RetrieveAndSummarizeEvidence(ctx context.Context, esClient *elastic.Client, hints ElasticContextHints) (string, error) {
	if esClient == nil || !esClient.Enabled() {
		return "", fmt.Errorf("elastic client not available")
	}

	if strings.TrimSpace(hints.DeviceID) == "" {
		return "", fmt.Errorf("elastic context missing deviceId")
	}
	if strings.TrimSpace(hints.IncidentID) == "" {
		return "", fmt.Errorf("elastic context missing incidentId")
	}

	windowStart, windowEnd := boundedTimeRange(hints.TimeRangeStart, hints.TimeRangeEnd, defaultQueryWindowMinutes)
	start := windowStart.Format(time.RFC3339)
	end := windowEnd.Format(time.RFC3339)

	telemetryIndex, incidentsIndex, logsIndex := splitIndexPatterns(hints.IndexPatterns)

	telemetryMust := []any{
		termClause(elastic.FieldDeviceID, hints.DeviceID),
		rangeClause(elastic.FieldTimestamp, start, end),
	}
	if service := strings.TrimSpace(hints.ServiceName); service != "" {
		telemetryMust = append(telemetryMust, termClause(elastic.FieldServiceName, service))
	}

	incidentMust := []any{
		termClause(elastic.FieldIncidentID, hints.IncidentID),
		rangeClause(elastic.FieldTimestamp, start, end),
	}
	if service := strings.TrimSpace(hints.ServiceName); service != "" {
		incidentMust = append(incidentMust, termClause(elastic.FieldServiceName, service))
	}

	logsMust := []any{
		termClause(elastic.FieldDeviceID, hints.DeviceID),
		rangeClause(elastic.FieldTimestamp, start, end),
	}
	if service := strings.TrimSpace(hints.ServiceName); service != "" {
		logsMust = append(logsMust, termClause(elastic.FieldServiceName, service))
	}

	telemetryQuery := buildSearchQuery(telemetryMust, maxSummaryHitsPerSection)
	incidentsQuery := buildSearchQuery(incidentMust, maxSummaryHitsPerSection)
	logsQuery := buildSearchQuery(logsMust, 10)

	var sb strings.Builder
	sb.WriteString("Evidence summary:\n")
	sb.WriteString(fmt.Sprintf("- QueryWindow: %s to %s (bounded=%dm)\n", start, end, defaultQueryWindowMinutes))

	telemetryBody, err := esClient.Search(ctx, telemetryIndex, telemetryQuery, maxSummaryHitsPerSection)
	if err != nil {
		return "", fmt.Errorf("telemetry query failed: %w", err)
	}
	if len(telemetryBody) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(telemetryBody, &parsed); err == nil {
			appendHitsSummary(&sb, parsed, "Telemetry", []string{"timestamp", "serviceStatus", "heartbeat", "cpuUsage", "memoryUsage"})
		}
	}

	incidentsBody, err := esClient.Search(ctx, incidentsIndex, incidentsQuery, maxSummaryHitsPerSection)
	if err != nil {
		return "", fmt.Errorf("incidents query failed: %w", err)
	}
	if len(incidentsBody) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(incidentsBody, &parsed); err == nil {
			appendHitsSummary(&sb, parsed, "IncidentEvents", []string{"timestamp", "state", "severity", "reason"})
		}
	}

	logsBody, err := esClient.Search(ctx, logsIndex, logsQuery, 10)
	if err != nil {
		return "", fmt.Errorf("logs query failed: %w", err)
	}
	if len(logsBody) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(logsBody, &parsed); err == nil {
			appendHitsSummary(&sb, parsed, "Logs", []string{"timestamp", "message", "source"})
		}
	}

	return sb.String(), nil
}

// appendHitsSummary extracts top hit sources and appends readable lines to the builder.
func appendHitsSummary(sb *strings.Builder, parsed map[string]any, section string, fields []string) {
	hitsRoot, ok := parsed["hits"].(map[string]any)
	if !ok {
		return
	}
	hitsArr, ok := hitsRoot["hits"].([]any)
	if !ok || len(hitsArr) == 0 {
		return
	}

	sb.WriteString(fmt.Sprintf("- %s (top %d):\n", section, min(len(hitsArr), maxSummaryHitsPerSection)))

	for i, h := range hitsArr {
		if i >= maxSummaryHitsPerSection {
			break
		}
		hit, ok := h.(map[string]any)
		if !ok {
			continue
		}
		src, _ := hit["_source"].(map[string]any)
		var parts []string
		for _, f := range fields {
			if v, ok := src[f]; ok {
				parts = append(parts, fmt.Sprintf("%s=%v", f, v))
			}
		}
		if len(parts) > 0 {
			sb.WriteString(fmt.Sprintf("  - %s\n", strings.Join(parts, "; ")))
		}
	}
}

func buildSearchQuery(mustClauses []any, size int) map[string]any {
	return map[string]any{
		"size": size,
		"sort": []map[string]map[string]string{{elastic.FieldTimestamp: {"order": "desc"}}},
		"query": map[string]any{
			"bool": map[string]any{
				"must": mustClauses,
			},
		},
	}
}

func termClause(field string, value string) map[string]any {
	return map[string]any{"term": map[string]any{field: value}}
}

func rangeClause(field string, gte string, lte string) map[string]any {
	return map[string]any{"range": map[string]any{field: map[string]any{"gte": gte, "lte": lte}}}
}

func splitIndexPatterns(indexPatterns []string) (telemetry []string, incidents []string, logs []string) {
	defaults := []string{"telemetry-events-*", "incident-events-*", "endpoint-logs-*"}
	if len(indexPatterns) == 0 {
		return []string{defaults[0]}, []string{defaults[1]}, []string{defaults[2]}
	}

	get := func(i int) []string {
		if i >= 0 && i < len(indexPatterns) && strings.TrimSpace(indexPatterns[i]) != "" {
			return []string{strings.TrimSpace(indexPatterns[i])}
		}
		return []string{defaults[i]}
	}

	if len(indexPatterns) >= 3 {
		return get(0), get(1), get(2)
	}

	// Backward-compatible fallback: if a single pattern is provided, use it for all.
	first := get(0)
	return first, first, first
}

func boundedTimeRange(start time.Time, end time.Time, maxMinutes int) (time.Time, time.Time) {
	now := time.Now().UTC()
	if start.IsZero() {
		start = now.Add(-time.Duration(maxMinutes) * time.Minute)
	}
	if end.IsZero() {
		end = now
	}
	start = start.UTC()
	end = end.UTC()

	if end.Before(start) {
		end = start
	}

	maxDuration := time.Duration(maxMinutes) * time.Minute
	if end.Sub(start) > maxDuration {
		start = end.Add(-maxDuration)
	}

	return start, end
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
