import { useState } from 'react'

import { DEFAULT_DEMO_SCENARIO_KEY, DEMO_SCENARIOS } from '../data/demoScenarios'
import { useDemoControls } from '../hooks/useDemoControls'
import { useDemoMode } from '../hooks/useDemoMode'

type DemoControlsProps = {
  /** The dashboard's currently-selected device — the incident is generated here. */
  selectedDeviceId: string
  /** Called after a successful reset so the page can clear the device's incidents locally. */
  onAfterReset?: () => void
}

/**
 * DemoControls is the judge-facing "Simulate Service Failure" panel (DEMO_MODE only). It
 * generates a real incident on the selected device from the Part A scenario catalog and lets
 * the judge watch it diagnose (real Gemini + Agent Builder + Elastic MCP) and resolve live.
 * Renders nothing unless VITE_DEMO_MODE=true.
 */
export function DemoControls({ selectedDeviceId, onAfterReset }: DemoControlsProps) {
  const demoMode = useDemoMode()
  const { status, error, response, submitting, generate, reset } = useDemoControls()
  const [scenarioKey, setScenarioKey] = useState(DEFAULT_DEMO_SCENARIO_KEY)
  const [autoApprove, setAutoApprove] = useState(false)

  // Hooks run unconditionally above; only the render is gated.
  if (!demoMode) {
    return null
  }

  const scenario = DEMO_SCENARIOS.find((item) => item.key === scenarioKey) ?? DEMO_SCENARIOS[0]
  const deviceReady = selectedDeviceId.trim().length > 0
  const busy = submitting || !deviceReady

  const handleGenerate = async () => {
    if (busy) {
      return
    }
    await generate(selectedDeviceId, scenario, autoApprove)
  }

  const handleReset = async () => {
    if (busy) {
      return
    }
    if (await reset(selectedDeviceId)) {
      onAfterReset?.()
    }
  }

  return (
    <section className="status-card demo-controls-panel" aria-label="Demo controls">
      <div className="card-chip badge-demo">Demo · Simulation</div>
      <h2>Simulate Service Failure</h2>
      <p className="demo-controls-sub">
        Generate a real incident on the selected device and watch the AI diagnose it via
        Gemini + Agent Builder + Elastic MCP, then resolve it live.
      </p>

      <div className="demo-controls-grid">
        <div className="control-group">
          <label>Active device</label>
          <span className="demo-device-value">{deviceReady ? selectedDeviceId : '— none selected —'}</span>
        </div>

        <div className="control-group">
          <label htmlFor="demo-scenario">Scenario</label>
          <select
            id="demo-scenario"
            className="agent-select"
            value={scenarioKey}
            disabled={submitting}
            onChange={(event) => setScenarioKey(event.target.value)}
          >
            {DEMO_SCENARIOS.map((item) => (
              <option key={item.key} value={item.key}>
                {item.label} ({item.serviceName})
              </option>
            ))}
          </select>
        </div>
      </div>

      <label className="demo-auto-approve">
        <input
          type="checkbox"
          checked={autoApprove}
          disabled={submitting}
          onChange={(event) => setAutoApprove(event.target.checked)}
        />
        <span>
          Auto-approve remediation{' '}
          <em>(off — judge clicks Approve, showing the human governance gate)</em>
        </span>
      </label>

      {scenario.mode === 'simulated' ? (
        <p className="demo-controls-note">
          <code>{scenario.serviceName}</code> uses the simulated execution path (Defender tamper
          protection can block a real restart). Use <code>Spooler</code> for the real-agent proof.
        </p>
      ) : null}

      <div className="demo-controls-actions">
        <button
          type="button"
          className="demo-button demo-button-primary"
          disabled={busy}
          onClick={() => void handleGenerate()}
        >
          {submitting ? 'Working…' : 'Simulate Service Failure'}
        </button>
        <button
          type="button"
          className="demo-button demo-button-secondary"
          disabled={busy}
          onClick={() => void handleReset()}
        >
          Reset
        </button>
      </div>

      {status === 'error' && error ? (
        <p className="demo-controls-status demo-controls-error" role="alert">
          {error}
        </p>
      ) : null}
      {status === 'success' && response ? (
        <p className="demo-controls-status demo-controls-success">
          Incident <code>{response.incidentId}</code> created on {response.serviceName} — watch it
          progress below.
        </p>
      ) : null}
    </section>
  )
}
