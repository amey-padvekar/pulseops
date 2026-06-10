package safety

import "strings"

// SecurityCheckResult reports the outcome of the startup security posture check.
// Errors are fatal in production; Warnings are advisory in any environment.
type SecurityCheckResult struct {
	Errors   []string
	Warnings []string
}

// OK reports whether the configuration has no fatal security errors.
func (r SecurityCheckResult) OK() bool {
	return len(r.Errors) == 0
}

// CheckSecurityConfig validates the runtime security posture from environment
// values. It is a deployment gate (Phase E3): in production a missing inbound
// auth token is fatal, so the service refuses to start unauthenticated.
//
// env is an indirection over os.Getenv so the check is unit-testable.
func CheckSecurityConfig(env func(string) string) SecurityCheckResult {
	var res SecurityCheckResult
	isProd := strings.EqualFold(strings.TrimSpace(env("APP_ENV")), "production")

	if strings.TrimSpace(env("INBOUND_AUTH_TOKEN")) == "" {
		msg := "INBOUND_AUTH_TOKEN is not set; inbound bearer auth is disabled"
		if isProd {
			res.Errors = append(res.Errors, msg)
		} else {
			res.Warnings = append(res.Warnings, msg)
		}
	}

	if strings.EqualFold(strings.TrimSpace(env("ELASTIC_MCP_ENABLED")), "true") &&
		strings.TrimSpace(env("ELASTIC_MCP_TOKEN")) == "" {
		res.Warnings = append(res.Warnings,
			"ELASTIC_MCP_ENABLED=true but ELASTIC_MCP_TOKEN is empty; enrichment calls would be unauthenticated")
	}

	return res
}
