package obs

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestLoggerRedactsConfiguredSecretInValues(t *testing.T) {
	secret := "super-secret-inbound-token"
	t.Setenv("INBOUND_AUTH_TOKEN", secret)

	var buf bytes.Buffer
	logger := NewLoggerTo(&buf)
	// Simulate an error string that accidentally embeds the secret.
	logger.Error("adk investigate failed", "error", errors.New("dial failed for Bearer "+secret))

	out := buf.String()
	if strings.Contains(out, secret) {
		t.Fatalf("log output leaked secret value:\n%s", out)
	}
	if !strings.Contains(out, redactedPlaceholder) {
		t.Fatalf("expected redaction placeholder in output:\n%s", out)
	}
}

func TestLoggerRedactsSensitiveKeys(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerTo(&buf)
	logger.Info("inbound", "authorization", "Bearer abc.def.ghi", "request_id", "req-1")

	out := buf.String()
	if strings.Contains(out, "abc.def.ghi") {
		t.Fatalf("authorization value should be redacted:\n%s", out)
	}
	if !strings.Contains(out, "req-1") {
		t.Fatalf("non-sensitive field should survive:\n%s", out)
	}
}

func TestLoggerEmitsCloudLoggingSeverity(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLoggerTo(&buf)
	logger.Warn("heads up")

	out := buf.String()
	if !strings.Contains(out, `"severity":"WARNING"`) {
		t.Fatalf("expected cloud logging severity remap:\n%s", out)
	}
	if !strings.Contains(out, `"message":"heads up"`) {
		t.Fatalf("expected message key remap:\n%s", out)
	}
}
