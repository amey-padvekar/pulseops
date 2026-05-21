package config

import (
	"testing"
	"time"
)

func TestLoadUsesLocalDefaults(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("AGENT_DEVICE_ID", "")
	t.Setenv("AGENT_HEARTBEAT_INTERVAL_SEC", "")
	t.Setenv("MONITORED_SERVICE_NAME", "")
	t.Setenv("BACKEND_BASE_URL", "")
	t.Setenv("AGENT_REQUEST_TIMEOUT_MS", "")
	t.Setenv("ENABLE_SIMULATED_LOGS", "")
	t.Setenv("NETWORK_CHECK_HOST", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.AppEnv != defaultAppEnv {
		t.Fatalf("AppEnv = %q, want %q", cfg.AppEnv, defaultAppEnv)
	}
	if cfg.DeviceID != defaultDeviceID {
		t.Fatalf("DeviceID = %q, want %q", cfg.DeviceID, defaultDeviceID)
	}
	if cfg.HeartbeatIntervalSec != defaultHeartbeatIntervalSec {
		t.Fatalf("HeartbeatIntervalSec = %d, want %d", cfg.HeartbeatIntervalSec, defaultHeartbeatIntervalSec)
	}
	if cfg.HeartbeatInterval != 10*time.Second {
		t.Fatalf("HeartbeatInterval = %s, want %s", cfg.HeartbeatInterval, 10*time.Second)
	}
	if cfg.MonitoredServiceName != defaultMonitoredServiceName {
		t.Fatalf("MonitoredServiceName = %q, want %q", cfg.MonitoredServiceName, defaultMonitoredServiceName)
	}
	if cfg.BackendBaseURL != defaultBackendBaseURL {
		t.Fatalf("BackendBaseURL = %q, want %q", cfg.BackendBaseURL, defaultBackendBaseURL)
	}
	if cfg.RequestTimeoutMS != defaultRequestTimeoutMS {
		t.Fatalf("RequestTimeoutMS = %d, want %d", cfg.RequestTimeoutMS, defaultRequestTimeoutMS)
	}
	if cfg.RequestTimeout != 5*time.Second {
		t.Fatalf("RequestTimeout = %s, want %s", cfg.RequestTimeout, 5*time.Second)
	}
	if cfg.EnableSimulatedLogs != defaultEnableSimulatedLogs {
		t.Fatalf("EnableSimulatedLogs = %t, want %t", cfg.EnableSimulatedLogs, defaultEnableSimulatedLogs)
	}
	if cfg.NetworkCheckHost != defaultNetworkCheckHost {
		t.Fatalf("NetworkCheckHost = %q, want %q", cfg.NetworkCheckHost, defaultNetworkCheckHost)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	t.Setenv("APP_ENV", "qa")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid APP_ENV")
	}

	t.Setenv("APP_ENV", defaultAppEnv)
	t.Setenv("AGENT_HEARTBEAT_INTERVAL_SEC", "0")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for non-positive AGENT_HEARTBEAT_INTERVAL_SEC")
	}

	t.Setenv("AGENT_HEARTBEAT_INTERVAL_SEC", "10")
	t.Setenv("BACKEND_BASE_URL", "localhost:8080")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error for invalid BACKEND_BASE_URL")
	}
}
