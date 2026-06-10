package agentbuilder

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds Agent Builder configuration from environment variables.
type Config struct {
	Endpoint    string
	ADKEndpoint string
	AuthToken   string

	// Transport selects the investigation transport: "agent_engine" | "adk" | "http".
	Transport string
	// AgentEngineResource is the Vertex AI Agent Engine reasoningEngine resource name.
	AgentEngineResource string
	GoogleProject       string
	GoogleLocation      string

	Timeout        time.Duration
	SummaryTimeout time.Duration
	Enabled        bool
	UseADK         bool
}

// NewConfig reads Agent Builder configuration from environment variables.
func NewConfig() (*Config, error) {
	cfg := &Config{
		Endpoint:            strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_BUILDER_ENDPOINT")), "/"),
		ADKEndpoint:         strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_BUILDER_ADK_ENDPOINT")), "/"),
		AuthToken:           strings.TrimSpace(os.Getenv("AGENT_BUILDER_AUTH")),
		AgentEngineResource: strings.TrimSpace(os.Getenv("AGENT_ENGINE_RESOURCE")),
		GoogleProject:       strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT")),
		GoogleLocation:      strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_LOCATION")),
	}

	// Resolve transport: explicit env wins, else infer from what is configured.
	transport := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BUILDER_TRANSPORT")))
	if transport == "" {
		switch {
		case cfg.AgentEngineResource != "":
			transport = "agent_engine"
		case cfg.ADKEndpoint != "":
			transport = "adk"
		default:
			transport = "http"
		}
	}
	cfg.Transport = transport
	cfg.UseADK = transport == "adk"

	// Fill project/location from the resource path when not set explicitly.
	if cfg.AgentEngineResource != "" {
		if cfg.GoogleProject == "" {
			cfg.GoogleProject = resourceSegment(cfg.AgentEngineResource, "projects")
		}
		if cfg.GoogleLocation == "" {
			cfg.GoogleLocation = resourceSegment(cfg.AgentEngineResource, "locations")
		}
	}

	if timeoutRaw := strings.TrimSpace(os.Getenv("AGENT_BUILDER_TIMEOUT_MS")); timeoutRaw != "" {
		parsed, err := strconv.Atoi(timeoutRaw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("AGENT_BUILDER_TIMEOUT_MS must be a positive integer")
		}
		cfg.Timeout = time.Duration(parsed) * time.Millisecond
	} else if transport == "agent_engine" {
		// Agent Engine + Gemini + MCP round-trips need a larger budget than HTTP.
		cfg.Timeout = 60 * time.Second
	} else {
		cfg.Timeout = 10 * time.Second
	}

	if summaryRaw := strings.TrimSpace(os.Getenv("AGENT_BUILDER_SUMMARY_TIMEOUT_MS")); summaryRaw != "" {
		parsed, err := strconv.Atoi(summaryRaw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("AGENT_BUILDER_SUMMARY_TIMEOUT_MS must be a positive integer")
		}
		cfg.SummaryTimeout = time.Duration(parsed) * time.Millisecond
	} else {
		cfg.SummaryTimeout = DefaultSummaryTimeout
	}

	enabledEnv := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BUILDER_ENABLED")))
	switch enabledEnv {
	case "true", "1", "yes":
		cfg.Enabled = true
	case "false", "0", "no":
		cfg.Enabled = false
	default:
		cfg.Enabled = cfg.Endpoint != "" || cfg.ADKEndpoint != "" || cfg.AgentEngineResource != ""
	}

	if cfg.Enabled {
		switch cfg.Transport {
		case "agent_engine":
			if cfg.AgentEngineResource == "" {
				return nil, fmt.Errorf("Agent Builder enabled with agent_engine transport but AGENT_ENGINE_RESOURCE is not set")
			}
			if cfg.GoogleLocation == "" {
				return nil, fmt.Errorf("agent_engine transport requires GOOGLE_CLOUD_LOCATION or a location segment in AGENT_ENGINE_RESOURCE")
			}
		case "adk":
			if !isHTTPURL(cfg.ADKEndpoint) {
				return nil, fmt.Errorf("AGENT_BUILDER_ADK_ENDPOINT must begin with http:// or https://")
			}
		default:
			if cfg.Endpoint == "" {
				return nil, fmt.Errorf("Agent Builder enabled but endpoint is not set")
			}
			if !isHTTPURL(cfg.Endpoint) {
				return nil, fmt.Errorf("AGENT_BUILDER_ENDPOINT must begin with http:// or https://")
			}
		}
	}

	return cfg, nil
}

// resourceSegment returns the value following the given key in a resource path
// like projects/<p>/locations/<l>/reasoningEngines/<id>.
func resourceSegment(resource, key string) string {
	parts := strings.Split(resource, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == key {
			return parts[i+1]
		}
	}
	return ""
}

func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}
