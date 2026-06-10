// Part A — judge-facing "Simulate Service Failure" scenario catalog (frontend mirror).
//
// Single source of truth for the DemoControls dropdown (Part C). Every entry names a
// service that actually exists on the Windows demo device, so the simulation is honest.
// The backend owns the scenario-flavored event logs (backend/internal/demo/scenarios.go);
// the frontend only needs label + serviceName + mode for the dropdown. Keep the keys and
// serviceName values in sync with the backend catalog.

export type DemoScenarioMode = 'simulated' | 'real'

export type DemoScenario = {
  /** Stable key sent to POST /demo/incident and matched against the backend catalog. */
  key: string
  /** Human-facing dropdown text. */
  label: string
  /** Windows service SHORT name (Get-Service "Name"); confirm against the demo box. */
  serviceName: string
  /**
   * 'simulated' = never really restarted (e.g. Defender, blocked by tamper protection);
   * 'real' = a live agent could also remediate it (Spooler is the safe real-agent proof).
   */
  mode: DemoScenarioMode
}

/** Default selection: the strongest, Elastic-friendly endpoint-security story. */
export const DEFAULT_DEMO_SCENARIO_KEY = 'defender'

export const DEMO_SCENARIOS: DemoScenario[] = [
  { key: 'defender', label: 'Endpoint Security — Microsoft Defender', serviceName: 'WinDefend', mode: 'simulated' },
  { key: 'mysql', label: 'Database — MySQL', serviceName: 'MySQL80', mode: 'real' },
  { key: 'iis', label: 'Web Server — IIS', serviceName: 'W3SVC', mode: 'real' },
  { key: 'spooler', label: 'Print Spooler', serviceName: 'Spooler', mode: 'real' },
]
