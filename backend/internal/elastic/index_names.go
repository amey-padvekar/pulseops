// backend/internal/elastic/index_names.go

package elastic

import "time"

// IndexName returns a UTC date-suffixed index name based on the event time.
// e.g. "telemetry-events-2026.05.23"
func IndexName(base string, eventTime time.Time) string {
	return base + "-" + eventTime.UTC().Format("2006.01.02")
}
