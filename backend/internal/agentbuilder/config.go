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
	Endpoint  string
	AuthToken string
	Timeout   time.Duration
	Enabled   bool
}

// NewConfig reads Agent Builder configuration from environment variables.
func NewConfig() (*Config, error) {
	cfg := &Config{
		Endpoint:  strings.TrimRight(strings.TrimSpace(os.Getenv("AGENT_BUILDER_ENDPOINT")), "/"),
		AuthToken: strings.TrimSpace(os.Getenv("AGENT_BUILDER_AUTH")),
	}

	if timeoutRaw := strings.TrimSpace(os.Getenv("AGENT_BUILDER_TIMEOUT_MS")); timeoutRaw != "" {
		parsed, err := strconv.Atoi(timeoutRaw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("AGENT_BUILDER_TIMEOUT_MS must be a positive integer")
		}
		cfg.Timeout = time.Duration(parsed) * time.Millisecond
	} else {
		cfg.Timeout = 10 * time.Second
	}

	enabledEnv := strings.ToLower(strings.TrimSpace(os.Getenv("AGENT_BUILDER_ENABLED")))
	switch enabledEnv {
	case "true", "1", "yes":
		cfg.Enabled = true
	case "false", "0", "no":
		cfg.Enabled = false
	default:
		cfg.Enabled = cfg.Endpoint != ""
	}

	if cfg.Enabled {
		if cfg.Endpoint == "" {
			return nil, fmt.Errorf("Agent Builder enabled but AGENT_BUILDER_ENDPOINT is not set")
		}
		if !strings.HasPrefix(cfg.Endpoint, "http://") && !strings.HasPrefix(cfg.Endpoint, "https://") {
			return nil, fmt.Errorf("AGENT_BUILDER_ENDPOINT must begin with http:// or https://")
		}
	}

	return cfg, nil
}
