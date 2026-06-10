// Package demo holds the additive, DEMO_MODE-gated "Simulate Service Failure" building
// blocks for judge-facing demos. This file is the Part A scenario catalog: the single
// source of truth for which REAL Windows services the demo can simulate, plus the
// scenario-flavored Windows event-log lines the /demo/incident handler (Part B) echoes
// into the synthetic telemetry. It changes no detection or remediation code.
//
// Honesty principle: every scenario names a service that actually exists on the Windows
// demo device, so the synthetic telemetry is truthful and a real `Restart-Service` would
// succeed. Confirm each ServiceName against the box with `Get-Service`.
package demo

import (
	"fmt"
	"time"
)

// Execution mode for a scenario.
const (
	// ModeSimulated marks a scenario whose service must not be really restarted on the
	// box (e.g. Microsoft Defender, blocked by tamper protection). The demo always drives
	// these through the simulated execution path.
	ModeSimulated = "simulated"
	// ModeReal marks a scenario whose service a real agent could safely restart. The demo
	// still simulates execution by default; these are eligible for the real-agent proof.
	ModeReal = "real"
)

// DefaultScenarioKey is the scenario pre-selected in the panel — the strongest,
// Elastic-friendly endpoint-security story, rendered honestly via the real WinDefend service.
const DefaultScenarioKey = "defender"

// LogTemplate is one synthetic Windows event. It mirrors the pipe-delimited shape the real
// WindowsEventLogCollector emits — "<timestamp>|<provider>|<eventId>|<level>|<message>"
// (agent/internal/platform/log_collector.go) — so synthetic and real telemetry are
// indistinguishable to everything downstream, including the Gemini + Elastic MCP investigation.
type LogTemplate struct {
	Provider string
	EventID  int
	Level    string
	Message  string
}

// Scenario is one selectable "service failure" a judge can launch.
type Scenario struct {
	// Key is the stable identifier the frontend sends to POST /demo/incident and the
	// value used for catalog lookup.
	Key string
	// Label is the human-facing dropdown text.
	Label string
	// ServiceName is the Windows service SHORT name (Get-Service "Name" column) reported
	// in the synthetic stopped telemetry and used as the restart_service target.
	ServiceName string
	// Mode is ModeSimulated or ModeReal.
	Mode string
	// Logs are the scenario-flavored events; RecentLogs renders them with a timestamp.
	Logs []LogTemplate
}

// RecentLogs renders the scenario's log templates into the pipe-delimited lines used by
// store.DeviceState.RecentLogs, stamped with at (coerced to UTC).
func (s Scenario) RecentLogs(at time.Time) []string {
	ts := at.UTC().Format(time.RFC3339Nano)
	out := make([]string, 0, len(s.Logs))
	for _, l := range s.Logs {
		out = append(out, fmt.Sprintf("%s|%s|%d|%s|%s", ts, l.Provider, l.EventID, l.Level, l.Message))
	}
	return out
}

// catalog is the authoritative, ordered scenario list. Keep each ServiceName in sync with
// the actual short name on the demo VM (confirm with `Get-Service`): e.g. MySQL 8 installers
// commonly register the service as "MySQL80". If you change a ServiceName, update the
// matching log Message text below so the evidence stays coherent. Drop any scenario whose
// service is not installed on your box.
var catalog = []Scenario{
	{
		Key:         "defender",
		Label:       "Endpoint Security — Microsoft Defender",
		ServiceName: "WinDefend",
		Mode:        ModeSimulated,
		Logs: []LogTemplate{
			{Provider: "Service Control Manager", EventID: 7036, Level: "Information", Message: "The Microsoft Defender Antivirus Service service entered the stopped state."},
			{Provider: "Microsoft-Windows-Windows Defender", EventID: 5001, Level: "Warning", Message: "Microsoft Defender Antivirus Real-time Protection has been disabled."},
			{Provider: "Microsoft-Windows-Windows Defender", EventID: 5010, Level: "Warning", Message: "Scanning for malware and other potentially unwanted software is disabled."},
			{Provider: "Microsoft-Windows-Security-SPP", EventID: 16384, Level: "Information", Message: "Last successful endpoint heartbeat recorded 11 seconds before the service stopped."},
		},
	},
	{
		Key:         "mysql",
		Label:       "Database — MySQL",
		ServiceName: "MySQL80",
		Mode:        ModeReal,
		Logs: []LogTemplate{
			{Provider: "Service Control Manager", EventID: 7034, Level: "Error", Message: "The MySQL80 service terminated unexpectedly. It has done this 1 time(s)."},
			{Provider: "MySQL", EventID: 100, Level: "Error", Message: "mysqld: Got signal 15 (SIGTERM). Shutting down immediately."},
			{Provider: "Application", EventID: 2003, Level: "Error", Message: "Can't connect to MySQL server on '127.0.0.1:3306' (10061 connection refused)."},
		},
	},
	{
		Key:         "iis",
		Label:       "Web Server — IIS",
		ServiceName: "W3SVC",
		Mode:        ModeReal,
		Logs: []LogTemplate{
			{Provider: "Service Control Manager", EventID: 7036, Level: "Information", Message: "The World Wide Web Publishing Service service entered the stopped state."},
			{Provider: "Microsoft-Windows-WAS", EventID: 5074, Level: "Warning", Message: "Application pool 'DefaultAppPool' has been disabled. Worker processes are stopping."},
			{Provider: "Microsoft-Windows-IIS-W3SVC-WP", EventID: 2276, Level: "Error", Message: "Requests are returning HTTP 503 Service Unavailable to clients."},
		},
	},
	{
		Key:         "spooler",
		Label:       "Print Spooler",
		ServiceName: "Spooler",
		Mode:        ModeReal,
		Logs: []LogTemplate{
			{Provider: "Service Control Manager", EventID: 7034, Level: "Error", Message: "The Print Spooler service terminated unexpectedly. It has done this 1 time(s)."},
			{Provider: "Application Error", EventID: 1000, Level: "Error", Message: "Faulting application name: spoolsv.exe, faulting module name: localspl.dll."},
			{Provider: "Microsoft-Windows-PrintService", EventID: 808, Level: "Error", Message: "The print spooler failed to load a plug-in module."},
		},
	},
}

// Catalog returns a copy of the scenario catalog in display order. The returned slice is
// safe for the caller to reorder; the Scenario values (including Logs) are read-only.
func Catalog() []Scenario {
	out := make([]Scenario, len(catalog))
	copy(out, catalog)
	return out
}

// ByKey returns the scenario with the given key. The second result is false when no
// scenario matches.
func ByKey(key string) (Scenario, bool) {
	for _, s := range catalog {
		if s.Key == key {
			return s, true
		}
	}
	return Scenario{}, false
}

// Default returns the pre-selected scenario (DefaultScenarioKey).
func Default() Scenario {
	s, _ := ByKey(DefaultScenarioKey)
	return s
}
