import { useEffect, useMemo, useState } from 'react'
import type { ReactNode } from 'react'

import { AiInvestigationPanel } from '../components/AiInvestigationPanel'
import { DemoControls } from '../components/DemoControls'
import { EndpointHero } from '../components/EndpointHero'
import { LifecycleStrip } from '../components/LifecycleStrip'
import { FinalSummaryPanel } from '../components/FinalSummaryPanel'
import { RemediationApprovalCard } from '../components/RemediationApprovalCard'
import { RemediationExecutionPanel } from '../components/RemediationExecutionPanel'
import { ValidationPanel } from '../components/ValidationPanel'
import { useAgentStats } from '../hooks/useAgentStats'
import { useDeviceState } from '../hooks/useDeviceState'
import { useFlashOnChange } from '../hooks/useFlashOnChange'
import { useIncidents } from '../hooks/useIncidents'
import type { Incident } from '../types/dashboard'

// DashboardMood is the page-level macro-state that lets a viewer read the whole
// story at a glance: a calm baseline, an urgent active incident, an in-progress
// recovery, a distinct recovered end state, or a failure needing attention
// (Phase 12 step 4.10). It is intentionally separate from the endpoint hero so the
// page communicates the incident narrative, not just live device health.
type DashboardMood = 'baseline' | 'incident' | 'recovering' | 'recovered' | 'failed'

const MOOD_COPY: Record<DashboardMood, { title: string; sub: string; pulse: boolean }> = {
  baseline: { title: 'All systems healthy', sub: 'Monitoring — no active incidents', pulse: false },
  incident: { title: 'Active incident', sub: 'Service disruption detected', pulse: true },
  recovering: { title: 'Remediation in progress', sub: 'Restoring and verifying service', pulse: true },
  recovered: { title: 'Incident resolved', sub: 'Service recovered and verified', pulse: false },
  failed: { title: 'Remediation failed', sub: 'Needs operator attention', pulse: false },
}

function deriveMood(active: Incident | null, display: Incident | null): DashboardMood {
  if (active) {
    if (active.state === 'failed') {
      return 'failed'
    }
    if (active.state === 'approved' || active.state === 'executing' || active.state === 'validating') {
      return 'recovering'
    }
    // detected / investigating / awaiting_approval
    return 'incident'
  }
  // No active incident: reflect the persisted outcome so the narrative survives
  // after recovery (task 4) rather than snapping back to a clean baseline.
  if (display?.state === 'resolved') {
    return 'recovered'
  }
  if (display?.state === 'failed') {
    return 'failed'
  }
  return 'baseline'
}

// The incident chip reflects the current situation: a live incident is urgent,
// a just-resolved/failed incident keeps its outcome visible, and an endpoint with
// no incident history reads as calm.
function incidentChipText(active: Incident | null, display: Incident | null): string {
  if (active) {
    return 'active incident'
  }
  if (display) {
    return display.state
  }
  return 'no active incident'
}

function incidentChipClass(active: Incident | null, display: Incident | null): string {
  if (active) {
    return 'badge-stopped'
  }
  if (display?.state === 'resolved') {
    return 'badge-running'
  }
  if (display?.state === 'failed') {
    return 'badge-stopped'
  }
  if (display) {
    return 'badge-degraded'
  }
  return 'badge-placeholder'
}

function formatTimestamp(timestamp: string | undefined): string {
  if (!timestamp) {
    return 'N/A'
  }
  const parsed = new Date(timestamp)
  if (Number.isNaN(parsed.getTime())) {
    return 'N/A'
  }
  const hh = String(parsed.getUTCHours()).padStart(2, '0')
  const mm = String(parsed.getUTCMinutes()).padStart(2, '0')
  const ss = String(parsed.getUTCSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss} UTC`
}

// Summarizes an incident's investigation status for the list row, so a viewer can see
// at a glance whether Gemini + Elastic MCP has diagnosed it, is still working, or failed.
function incidentStatusLine(incident: Incident): string {
  if (incident.investigationStatus === 'failed' || incident.investigationError) {
    return 'Diagnosis failed'
  }
  if (incident.probableCause) {
    return incident.probableCause
  }
  if (incident.state === 'investigating') {
    return 'Diagnosing… (Gemini + Elastic MCP)'
  }
  return incident.reason || '—'
}

// Maps an incident state to one of the existing badge color classes.
function incidentStateBadge(incident: Incident): string {
  switch (incident.state) {
    case 'resolved':
      return 'badge-running'
    case 'failed':
    case 'detected':
    case 'investigating':
    case 'awaiting_approval':
      return 'badge-stopped'
    case 'approved':
    case 'executing':
    case 'validating':
      return 'badge-degraded'
    default:
      return 'badge-placeholder'
  }
}

// FlowStage wraps each panel in a numbered, labeled stage so the dashboard reads
// top-to-bottom / left-to-right as the demo narrative: detect -> investigate ->
// remediate -> validate -> summarize. The label sits above the panel and never
// duplicates the panel's own title; it gives narration context instead.
//
// `flashKey` is a signature of the stage's relevant state; when it changes (driven
// by a websocket update) the stage briefly highlights so viewers can follow the
// change in motion (Phase 12 step 4.8).
type FlowStageProps = {
  step: number
  label: string
  flow: string
  className?: string
  flashKey?: string
  children: ReactNode
}

function FlowStage({ step, label, flow, className, flashKey, children }: FlowStageProps) {
  const updated = useFlashOnChange(flashKey)
  return (
    <section
      className={`flow-stage ${className ?? ''} ${updated ? 'is-updated' : ''}`}
      aria-label={label}
    >
      <div className="flow-stage-label">
        <span className="flow-step" aria-hidden="true">
          {step}
        </span>
        <span className="flow-stage-name">{label}</span>
        <span className="flow-stage-flow">{flow}</span>
      </div>
      {children}
    </section>
  )
}

export function DashboardPage() {
  const { activeAgents, activeAgentDevices } = useAgentStats()
  // No hardcoded default device: the dropdown lists only devices actually reporting
  // telemetry (activeAgentDevices), since a configured default agent may not exist.
  // Start with nothing selected; the effect below auto-selects the first real device.
  const [selectedDeviceId, setSelectedDeviceId] = useState('')

  const selectableDevices = useMemo(() => {
    const unique = new Set<string>()
    for (const deviceId of activeAgentDevices) {
      unique.add(deviceId)
    }
    return Array.from(unique)
  }, [activeAgentDevices])

  useEffect(() => {
    // Follow the live device set: auto-select the first reporting device when nothing
    // valid is selected, and clear the selection if the chosen device stops reporting.
    if (!selectableDevices.includes(selectedDeviceId)) {
      setSelectedDeviceId(selectableDevices[0] ?? '')
    }
  }, [selectableDevices, selectedDeviceId])

  const { deviceState, connected } = useDeviceState(selectedDeviceId)
  const {
    incidents,
    activeIncident,
    latestIncident,
    dismissIncident,
    connected: incidentConnected,
  } = useIncidents(selectedDeviceId)

  // An operator can click an incident in the list to pin it into the flow view below;
  // otherwise the view auto-follows the active (or most recent) incident.
  const [selectedIncidentId, setSelectedIncidentId] = useState<string | null>(null)

  // Reset the manual selection when switching devices so the view follows the new device.
  useEffect(() => {
    setSelectedIncidentId(null)
  }, [selectedDeviceId])

  // Validation/closure UX follows the active incident, but keeps showing a just-resolved
  // or just-failed incident's recovery outcome after it deactivates (Phase 10 step 4.8).
  // The same persisted incident feeds the execution and approval panels so a failed
  // attempt's execution results, validation evidence, and recommendation stay visible
  // together for operator review and manual re-run (Phase 10 step 4.9). The incident
  // record itself is never deleted, so failures are not archived prematurely.
  const selectedIncident = selectedIncidentId
    ? incidents.find((incident) => incident.incidentId === selectedIncidentId) ?? null
    : null
  // A pinned (clicked) incident takes precedence; otherwise follow active, then latest.
  const displayIncident = selectedIncident ?? activeIncident ?? latestIncident

  // Per-stage flash signatures: each captures the slice of state that, when it
  // changes over websocket, should briefly highlight that stage (Phase 12 step 4.8).
  const mood = deriveMood(activeIncident, displayIncident)
  const moodCopy = MOOD_COPY[mood]

  const incidentId = displayIncident?.incidentId ?? 'none'
  const healthFlashKey = `${deviceState?.serviceStatus ?? 'none'}|${displayIncident?.state ?? 'none'}`
  const incidentFlashKey = `${incidentId}|${displayIncident?.state ?? 'none'}`
  const investigationFlashKey = `${incidentId}|${displayIncident?.investigationStatus ?? 'none'}`
  const remediationFlashKey = `${incidentId}|${displayIncident?.state ?? 'none'}|${displayIncident?.remediationStatus ?? 'none'}`
  const validationFlashKey = `${incidentId}|${displayIncident?.validationStatus ?? 'none'}|${displayIncident?.state ?? 'none'}`
  const summaryFlashKey = `${incidentId}|${displayIncident?.summaryStatus ?? 'none'}|${displayIncident?.state ?? 'none'}`

  return (
    <main className="dashboard-shell">
      <header className="shell-header">
        <p className="kicker">PulseOps AI</p>
        <h1>Autonomous Incident Operations</h1>
        <p className="subtitle">
          Detect &rarr; Investigate &rarr; Remediate &rarr; Validate &rarr; Resolve
        </p>
      </header>

      {/* Page-level mood: a single at-a-glance read of the before/failure/after
          progression. Calm baseline, urgent incident, in-progress recovery, and a
          distinct recovered/failed end state (Phase 12 step 4.10). */}
      <section className={`mood-banner mood-${mood}`} aria-label="Overall status" role="status">
        <span className={`mood-dot ${moodCopy.pulse ? 'mood-dot-pulse' : ''}`} aria-hidden="true" />
        <span className="mood-title">{moodCopy.title}</span>
        <span className="mood-sub">{moodCopy.sub}</span>
      </section>

      {/* Operator controls are demoted to a compact bar so the incident story stays
          the focus. Device selection, connectivity, and agent count live here. */}
      <section className="control-bar" aria-label="Operator controls">
        <div className="control-group">
          <label htmlFor="active-agent-select">Monitored Device</label>
          <select
            id="active-agent-select"
            className="agent-select"
            value={selectedDeviceId}
            onChange={(event) => setSelectedDeviceId(event.target.value)}
          >
            {selectedDeviceId === '' ? (
              <option value="" disabled>
                {selectableDevices.length === 0 ? 'No active devices' : 'Select a device…'}
              </option>
            ) : null}
            {selectableDevices.map((deviceId) => (
              <option key={deviceId} value={deviceId}>
                {deviceId}
              </option>
            ))}
          </select>
        </div>

        {/* Multi-agent quick-switch is only shown when more than one device is
            tracked; with a single demo device it would just duplicate the selector
            above, so it stays off the screen as noise (Phase 12 step 4.12). */}
        {activeAgentDevices.length > 1 ? (
          <div className="control-group control-devices">
            <label>Switch Device</label>
            <ul className="agent-list">
              {activeAgentDevices.map((deviceId) => (
                <li key={deviceId}>
                  <button
                    type="button"
                    className={`agent-pill ${selectedDeviceId === deviceId ? 'agent-pill-active' : ''}`}
                    onClick={() => setSelectedDeviceId(deviceId)}
                  >
                    {deviceId}
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}

        <div className="control-group control-status">
          <span className={`control-badge ${activeAgents > 0 ? 'badge-running' : 'badge-unknown'}`}>
            {activeAgents} {activeAgents === 1 ? 'agent' : 'agents'}
          </span>
          <span className={`control-badge ${connected ? 'badge-running' : 'badge-degraded'}`}>
            telemetry {connected ? 'live' : 'reconnecting'}
          </span>
          <span className={`control-badge ${incidentConnected ? 'badge-running' : 'badge-degraded'}`}>
            incidents {incidentConnected ? 'live' : 'reconnecting'}
          </span>
        </div>
      </section>

      {/* DEMO_MODE-only: judge generates an incident on the selected device and watches it
          resolve live through the panels below. Renders nothing unless VITE_DEMO_MODE=true. */}
      <DemoControls
        selectedDeviceId={selectedDeviceId}
        onAfterReset={() => {
          // /demo/reset deletes the device's incidents server-side but emits no removal event,
          // so clear them locally too; dismissIncident treats the resulting 404 as success.
          for (const incident of incidents) {
            if (selectedIncidentId === incident.incidentId) {
              setSelectedIncidentId(null)
            }
            void dismissIncident(incident.incidentId)
          }
        }}
      />

      {/* Incident list / history: every incident for this endpoint with its live
          diagnosis status. Click a row to pin it into the flow below; Clear removes a
          stale or failed-to-diagnose incident from the dashboard. */}
      <section className="incident-list-panel" aria-label="Incidents">
        <div className="incident-list-header">
          <h2>Incidents</h2>
          <span className="incident-list-count">{incidents.length}</span>
        </div>
        {incidents.length === 0 ? (
          <p className="incident-list-empty">No incidents recorded for this endpoint.</p>
        ) : (
          <ul className="incident-list">
            {incidents.map((incident) => {
              const isSelected = incident.incidentId === displayIncident?.incidentId
              return (
                <li
                  key={incident.incidentId}
                  className={`incident-row ${isSelected ? 'is-selected' : ''}`}
                >
                  <button
                    type="button"
                    className="incident-row-main"
                    onClick={() => setSelectedIncidentId(incident.incidentId)}
                    aria-pressed={isSelected}
                  >
                    <span className={`incident-row-state ${incidentStateBadge(incident)}`}>
                      {incident.state}
                    </span>
                    <span className="incident-row-service">{incident.serviceName}</span>
                    <span className="incident-row-status">{incidentStatusLine(incident)}</span>
                    <span className="incident-row-time">{formatTimestamp(incident.updatedAt)}</span>
                  </button>
                  <button
                    type="button"
                    className="incident-row-clear"
                    onClick={() => {
                      if (selectedIncidentId === incident.incidentId) {
                        setSelectedIncidentId(null)
                      }
                      void dismissIncident(incident.incidentId)
                    }}
                    aria-label={`Clear incident ${incident.incidentId}`}
                  >
                    Clear
                  </button>
                </li>
              )
            })}
          </ul>
        )}
      </section>

      {/* Narrative flow: each stage is ordered to match the demo story so a single
          screen presents detect -> investigate -> remediate -> validate -> summarize. */}
      <div className="dashboard-flow">
        <FlowStage step={1} label="Endpoint Health" flow="Monitor" className="flow-health" flashKey={healthFlashKey}>
          <EndpointHero
            deviceState={deviceState ?? undefined}
            incident={displayIncident}
            telemetryConnected={connected}
          />
        </FlowStage>

        <FlowStage step={2} label="Incident Timeline" flow="Detect" className="flow-incident" flashKey={incidentFlashKey}>
          <article
            className={`status-card incident-panel ${activeIncident ? 'incident-active' : 'incident-idle'}`}
            aria-label="Incident Timeline"
          >
            <div className={`card-chip ${incidentChipClass(activeIncident, displayIncident)}`}>
              {incidentChipText(activeIncident, displayIncident)}
            </div>
            <h2>Incident Timeline</h2>
            <p>
              {activeIncident
                ? 'Real-time incident state from backend detection.'
                : displayIncident
                  ? 'Most recent incident for this endpoint.'
                  : 'No active incident for this endpoint right now.'}
            </p>

            <div className="incident-lifecycle">
              <LifecycleStrip incident={displayIncident} />
            </div>

            <div className="incident-details">
              <dl className="metrics-grid">
                <div>
                  <dt>Incident ID</dt>
                  <dd>{displayIncident?.incidentId ?? 'N/A'}</dd>
                </div>
                <div>
                  <dt>State</dt>
                  <dd>{displayIncident?.state ?? 'healthy'}</dd>
                </div>
                <div>
                  <dt>Severity</dt>
                  <dd>{displayIncident?.severity ?? 'low'}</dd>
                </div>
                <div>
                  <dt>Detected</dt>
                  <dd>{displayIncident ? formatTimestamp(displayIncident.detectedAt) : 'N/A'}</dd>
                </div>
              </dl>
            </div>
          </article>
        </FlowStage>

        <FlowStage step={3} label="AI Investigation" flow="Investigate" className="flow-investigation" flashKey={investigationFlashKey}>
          <AiInvestigationPanel incident={displayIncident} deviceState={deviceState ?? undefined} />
        </FlowStage>

        {/* Approval and execution are presented as one governance -> action
            sequence: the operator approves, then the same authorization flows
            into real endpoint execution. The connector reinforces that order. */}
        <FlowStage step={4} label="Approval & Execution" flow="Approve → Act" className="flow-remediation" flashKey={remediationFlashKey}>
          <div className="remediation-sequence">
            <RemediationApprovalCard incident={displayIncident} />
            <div className="sequence-connector" aria-hidden="true">
              <span>→</span>
            </div>
            <RemediationExecutionPanel incident={displayIncident} />
          </div>
        </FlowStage>

        <FlowStage step={5} label="Recovery Validation" flow="Validate" className="flow-validation" flashKey={validationFlashKey}>
          <ValidationPanel incident={displayIncident} />
        </FlowStage>

        <FlowStage step={6} label="Final Summary" flow="Resolve" className="flow-summary" flashKey={summaryFlashKey}>
          <FinalSummaryPanel incident={displayIncident} />
        </FlowStage>
      </div>
    </main>
  )
}
