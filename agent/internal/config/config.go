package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAppEnv               = "development"
	defaultDeviceID             = "DEV-AGENT-01"
	defaultHeartbeatIntervalSec = 10
	defaultMonitoredServiceName = "OpenVPNService"
	defaultBackendBaseURL       = "http://localhost:8080"
	defaultRequestTimeoutMS     = 5000
	defaultEnableSimulatedLogs  = true
	defaultNetworkCheckHost     = "8.8.8.8"
)

var allowedAppEnvironments = map[string]struct{}{
	"development": {},
	"staging":     {},
	"production":  {},
}

// RuntimeConfig defines the Phase 2 agent startup contract.
type RuntimeConfig struct {
	AppEnv               string
	DeviceID             string
	HeartbeatInterval    time.Duration
	HeartbeatIntervalSec int
	MonitoredServiceName string
	BackendBaseURL       string
	RequestTimeout       time.Duration
	RequestTimeoutMS     int
	EnableSimulatedLogs  bool
	NetworkCheckHost     string
}

// Load builds the runtime config from environment variables with safe local defaults.
func Load() (RuntimeConfig, error) {
	appEnv := strings.ToLower(getString("APP_ENV", defaultAppEnv))
	if _, ok := allowedAppEnvironments[appEnv]; !ok {
		return RuntimeConfig{}, fmt.Errorf("APP_ENV must be one of development, staging, production")
	}

	heartbeatIntervalSec, err := getPositiveInt("AGENT_HEARTBEAT_INTERVAL_SEC", defaultHeartbeatIntervalSec)
	if err != nil {
		return RuntimeConfig{}, err
	}

	requestTimeoutMS, err := getPositiveInt("AGENT_REQUEST_TIMEOUT_MS", defaultRequestTimeoutMS)
	if err != nil {
		return RuntimeConfig{}, err
	}

	backendBaseURL := getString("BACKEND_BASE_URL", defaultBackendBaseURL)
	if err := validateHTTPURL("BACKEND_BASE_URL", backendBaseURL); err != nil {
		return RuntimeConfig{}, err
	}

	return RuntimeConfig{
		AppEnv:               appEnv,
		DeviceID:             getString("AGENT_DEVICE_ID", defaultDeviceID),
		HeartbeatInterval:    time.Duration(heartbeatIntervalSec) * time.Second,
		HeartbeatIntervalSec: heartbeatIntervalSec,
		MonitoredServiceName: getString("MONITORED_SERVICE_NAME", defaultMonitoredServiceName),
		BackendBaseURL:       backendBaseURL,
		RequestTimeout:       time.Duration(requestTimeoutMS) * time.Millisecond,
		RequestTimeoutMS:     requestTimeoutMS,
		EnableSimulatedLogs:  getBool("ENABLE_SIMULATED_LOGS", defaultEnableSimulatedLogs),
		NetworkCheckHost:     getString("NETWORK_CHECK_HOST", defaultNetworkCheckHost),
	}, nil
}

func getString(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getPositiveInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be greater than 0", key)
	}

	return parsed, nil
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func validateHTTPURL(key string, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", key, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https", key)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include a host", key)
	}
	return nil
}
