package obs

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

const redactedPlaceholder = "***REDACTED***"

// secretEnvKeys are environment variables whose values must never appear in logs.
var secretEnvKeys = []string{"INBOUND_AUTH_TOKEN", "ELASTIC_MCP_TOKEN", "AGENT_BUILDER_AUTH"}

// sensitiveLogKeys are attribute keys whose values are always redacted regardless
// of content, as defense-in-depth against accidentally logging credentials.
var sensitiveLogKeys = []string{"authorization", "token", "secret", "password", "api_key", "apikey"}

// NewLogger returns a structured JSON logger whose output is compatible with
// Google Cloud Logging's structured-log ingestion and which redacts known
// secrets. Secret values are captured once at construction (Cloud Run injects
// secrets before process start), so the redactor is allocation-free per call.
func NewLogger() *slog.Logger {
	return NewLoggerTo(os.Stdout)
}

// NewLoggerTo is NewLogger with an explicit writer, for tests.
func NewLoggerTo(w io.Writer) *slog.Logger {
	secrets := collectSecrets()
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:       slog.LevelInfo,
		ReplaceAttr: redactingAttrs(secrets),
	})
	return slog.New(handler)
}

func collectSecrets() []string {
	var out []string
	for _, key := range secretEnvKeys {
		value := strings.TrimSpace(os.Getenv(key))
		if len(value) < 4 { // ignore empty / trivially short values
			continue
		}
		out = append(out, value)
		// AGENT_BUILDER_AUTH is typically "Bearer <token>"; redact the bare token too.
		if after, ok := strings.CutPrefix(value, "Bearer "); ok {
			if token := strings.TrimSpace(after); len(token) >= 4 {
				out = append(out, token)
			}
		}
	}
	return out
}

func redactingAttrs(secrets []string) func([]string, slog.Attr) slog.Attr {
	return func(groups []string, a slog.Attr) slog.Attr {
		a = cloudLoggingAttrs(groups, a)

		if isSensitiveKey(a.Key) {
			return slog.String(a.Key, redactedPlaceholder)
		}

		switch a.Value.Kind() {
		case slog.KindString:
			if redacted := redactSecrets(a.Value.String(), secrets); redacted != a.Value.String() {
				return slog.String(a.Key, redacted)
			}
		case slog.KindAny:
			if err, ok := a.Value.Any().(error); ok {
				return slog.String(a.Key, redactSecrets(err.Error(), secrets))
			}
		}
		return a
	}
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveLogKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func redactSecrets(value string, secrets []string) string {
	for _, secret := range secrets {
		if secret != "" && strings.Contains(value, secret) {
			value = strings.ReplaceAll(value, secret, redactedPlaceholder)
		}
	}
	return value
}

func cloudLoggingAttrs(_ []string, a slog.Attr) slog.Attr {
	switch a.Key {
	case slog.LevelKey:
		// Cloud Logging reads "severity" rather than slog's "level".
		level, ok := a.Value.Any().(slog.Level)
		if !ok {
			return slog.Attr{Key: "severity", Value: a.Value}
		}
		return slog.Attr{Key: "severity", Value: slog.StringValue(cloudSeverity(level))}
	case slog.MessageKey:
		return slog.Attr{Key: "message", Value: a.Value}
	case slog.TimeKey:
		return slog.Attr{Key: "timestamp", Value: a.Value}
	default:
		return a
	}
}

func cloudSeverity(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARNING"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
