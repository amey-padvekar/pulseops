package agentbuilder

import "testing"

func TestNewConfig_DisabledByDefault(t *testing.T) {
	t.Setenv("AGENT_BUILDER_ENDPOINT", "")
	t.Setenv("AGENT_BUILDER_ENABLED", "")
	t.Setenv("AGENT_BUILDER_TIMEOUT_MS", "")

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig error: %v", err)
	}

	if cfg.Enabled {
		t.Fatal("expected Agent Builder to be disabled by default")
	}
	if cfg.Timeout.Milliseconds() != 10000 {
		t.Fatalf("timeout_ms = %d, want 10000", cfg.Timeout.Milliseconds())
	}
}

func TestNewConfig_EnabledRequiresEndpoint(t *testing.T) {
	t.Setenv("AGENT_BUILDER_ENABLED", "true")
	t.Setenv("AGENT_BUILDER_ENDPOINT", "")

	if _, err := NewConfig(); err == nil {
		t.Fatal("expected error when enabled without endpoint")
	}
}

func TestNewConfig_InvalidTimeout(t *testing.T) {
	t.Setenv("AGENT_BUILDER_ENDPOINT", "https://agent-builder.example.com")
	t.Setenv("AGENT_BUILDER_ENABLED", "true")
	t.Setenv("AGENT_BUILDER_TIMEOUT_MS", "-1")

	if _, err := NewConfig(); err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}
