package elastic

import "context"

type Indexer interface {
	Enabled() bool

	IndexTelemetryEvent(
		ctx context.Context,
		doc TelemetryEventDocument,
	) error

	IndexIncidentEvent(
		ctx context.Context,
		doc IncidentEventDocument,
	) error

	IndexRecentLogs(
		ctx context.Context,
		deviceID string,
		serviceName string,
		incidentID string,
		logs []string,
	) error
}
