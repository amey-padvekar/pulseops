// backend/internal/elastic/config.go

package elastic

import (
	"fmt"
	"os"
	"strings"
)

// recommended — matches the plan's language throughout
type Config struct {
	Endpoint       string
	APIKey         string
	IndexTelemetry string // was TelemetryBase
	IndexIncidents string // was IncidentsBase
	IndexLogs      string // was LogsBase
	Enabled        bool
}

func NewConfig() (*Config, error) {
	cfg := &Config{
		Endpoint:       strings.TrimRight(strings.TrimSpace(os.Getenv("ELASTIC_ENDPOINT")), "/"),
		APIKey:         strings.TrimSpace(os.Getenv("ELASTIC_API_KEY")),
		IndexTelemetry: os.Getenv("ELASTIC_INDEX_TELEMETRY"),
		IndexIncidents: os.Getenv("ELASTIC_INDEX_INCIDENTS"),
		IndexLogs:      os.Getenv("ELASTIC_INDEX_LOGS"),
	}

	if cfg.IndexTelemetry == "" {
		cfg.IndexTelemetry = "telemetry-events"
	}
	if cfg.IndexIncidents == "" {
		cfg.IndexIncidents = "incident-events"
	}
	if cfg.IndexLogs == "" {
		cfg.IndexLogs = "endpoint-logs"
	}

	enabledEnv := strings.ToLower(strings.TrimSpace(os.Getenv("ELASTIC_ENABLED")))
	switch enabledEnv {
	case "true", "1", "yes":
		cfg.Enabled = true
	case "false", "0", "no":
		cfg.Enabled = false
	default:
		cfg.Enabled = cfg.Endpoint != "" && cfg.APIKey != ""
	}

	if cfg.Enabled {
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("Elastic enabled but ELASTIC_ENDPOINT is not set")
		}
		if cfg.APIKey == "" {
			return nil, fmt.Errorf("Elastic enabled but ELASTIC_API_KEY is not set")
		}
		if !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://") {
			return nil, fmt.Errorf("ELASTIC_ENDPOINT must begin with http:// or https://")
		}
	}

	return cfg, nil
}
