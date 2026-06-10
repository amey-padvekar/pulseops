package safety

import "testing"

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestCheckSecurityConfigProductionRequiresAuthToken(t *testing.T) {
	res := CheckSecurityConfig(envFromMap(map[string]string{
		"APP_ENV": "production",
	}))
	if res.OK() {
		t.Fatalf("expected fatal error when auth token missing in production")
	}
}

func TestCheckSecurityConfigDevMissingAuthIsWarningOnly(t *testing.T) {
	res := CheckSecurityConfig(envFromMap(map[string]string{
		"APP_ENV": "dev",
	}))
	if !res.OK() {
		t.Fatalf("missing auth token in dev should not be fatal: %+v", res.Errors)
	}
	if len(res.Warnings) == 0 {
		t.Fatalf("expected a warning for missing auth token in dev")
	}
}

func TestCheckSecurityConfigProductionWithAuthPasses(t *testing.T) {
	res := CheckSecurityConfig(envFromMap(map[string]string{
		"APP_ENV":            "production",
		"INBOUND_AUTH_TOKEN": "a-real-token",
	}))
	if !res.OK() {
		t.Fatalf("expected OK with auth token set: %+v", res.Errors)
	}
}

func TestCheckSecurityConfigWarnsOnEnabledElasticWithoutToken(t *testing.T) {
	res := CheckSecurityConfig(envFromMap(map[string]string{
		"APP_ENV":             "production",
		"INBOUND_AUTH_TOKEN":  "a-real-token",
		"ELASTIC_MCP_ENABLED": "true",
	}))
	if !res.OK() {
		t.Fatalf("elastic-without-token should be a warning, not fatal: %+v", res.Errors)
	}
	found := false
	for _, w := range res.Warnings {
		if w != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning about ELASTIC_MCP_TOKEN")
	}
}
