import { useEffect, useMemo, useState } from 'react'

import { StatusCard } from '../components/StatusCard'
import { useAgentStats } from '../hooks/useAgentStats'
import { useDeviceState } from '../hooks/useDeviceState'
import { useIncidents } from '../hooks/useIncidents'
import type { DashboardCard } from '../types/dashboard'

type DashboardPageProps = {
  apiBaseUrl: string
}

const placeholderCards: DashboardCard[] = [
  {
    title: 'AI Investigation',
    status: 'placeholder',
    description: 'Agent Builder and Gemini probable-cause outputs will appear here in Phase 7.',
  },
  {
    title: 'Remediation Approval',
    status: 'placeholder',
    description: 'Human approval controls for recommended actions will be enabled in Phase 8.',
  },
]

function endpointCardStatus(serviceStatus: string | undefined): DashboardCard['status'] {
  if (serviceStatus === 'running') {
    return 'healthy'
  }
  if (serviceStatus === 'stopped') {
    return 'stopped'
  }
  if (serviceStatus === 'degraded') {
    return 'degraded'
  }
  if (serviceStatus === 'unknown') {
    return 'unknown'
  }
  return 'placeholder'
}

export function DashboardPage({ apiBaseUrl }: DashboardPageProps) {
  const defaultAgentDeviceId = import.meta.env.VITE_AGENT_DEVICE_ID || 'DEV-AGENT-01'
  const { activeAgents, activeAgentDevices } = useAgentStats()
  const [selectedDeviceId, setSelectedDeviceId] = useState(defaultAgentDeviceId)

  const selectableDevices = useMemo(() => {
    const unique = new Set<string>()
    unique.add(defaultAgentDeviceId)
    for (const deviceId of activeAgentDevices) {
      unique.add(deviceId)
    }
    return Array.from(unique)
  }, [activeAgentDevices, defaultAgentDeviceId])

  useEffect(() => {
    if (selectableDevices.length === 0) {
      return
    }

    if (!selectableDevices.includes(selectedDeviceId)) {
      setSelectedDeviceId(selectableDevices[0])
    }
  }, [selectableDevices, selectedDeviceId])

  const { deviceState, connected } = useDeviceState(selectedDeviceId)
  const { activeIncident, connected: incidentConnected } = useIncidents(selectedDeviceId)

  const endpointCard: DashboardCard = {
    title: 'Endpoint Health',
    status: endpointCardStatus(deviceState?.serviceStatus),
    description: deviceState
      ? `Live status for ${deviceState.deviceId}.`
      : 'Waiting for device telemetry...',
  }

  const activeAgentsCard: DashboardCard = {
    title: 'Active Agents',
    status: activeAgents > 0 ? 'healthy' : 'unknown',
    description:
      activeAgents === 1
        ? '1 agent currently tracked by the backend.'
        : `${activeAgents} agents currently tracked by the backend.`,
  }

  return (
    <main className="dashboard-shell">
      <header className="shell-header">
        <p className="kicker">PulseOps AI</p>
        <h1>Operations Dashboard Shell</h1>
        <p className="subtitle">
          Frontend baseline for detect, investigate, remediate, validate workflow.
        </p>
        <p className="api-hint">
          API base URL: <code>{apiBaseUrl}</code>
        </p>
        <p className="connection-state">
          Connection:
          <span className={connected ? 'conn-connected' : 'conn-reconnecting'}>
            {connected ? ' connected' : ' reconnecting'}
          </span>
        </p>
      </header>

      <section className="card-grid">
        <article className={`status-card ${activeAgents > 0 ? 'status-running' : 'status-unknown'}`}>
          <div className={`card-chip ${activeAgents > 0 ? 'badge-running' : 'badge-unknown'}`}>
            {activeAgents > 0 ? 'active agents' : 'no agents'}
          </div>
          <h2>{activeAgentsCard.title}</h2>
          <p>{activeAgentsCard.description}</p>

          <div className="agent-list-wrap">
            <label htmlFor="active-agent-select">Selected Agent Device</label>
            <select
              id="active-agent-select"
              className="agent-select"
              value={selectedDeviceId}
              onChange={(event) => setSelectedDeviceId(event.target.value)}
            >
              {selectableDevices.map((deviceId) => (
                <option key={deviceId} value={deviceId}>
                  {deviceId}
                </option>
              ))}
            </select>

            <h3>Tracked Devices</h3>
            {activeAgentDevices.length === 0 ? (
              <p className="empty-log">No active agents reported yet.</p>
            ) : (
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
            )}
          </div>
        </article>

        <StatusCard card={endpointCard} deviceState={deviceState ?? undefined} />

        <article
          className={`status-card incident-panel ${activeIncident ? 'incident-active' : 'incident-idle'}`}
          aria-label="Incident Timeline"
        >
          <div className={`card-chip ${activeIncident ? 'badge-stopped' : 'badge-placeholder'}`}>
            {activeIncident ? 'active incident' : 'no active incident'}
          </div>
          <h2>Incident Timeline</h2>
          <p>
            {activeIncident
              ? 'Real-time incident state from backend detection.'
              : 'No active incident for this endpoint right now.'}
          </p>

          <div className="incident-details">
            <dl className="metrics-grid">
              <div>
                <dt>Incident ID</dt>
                <dd>{activeIncident?.incidentId ?? 'N/A'}</dd>
              </div>
              <div>
                <dt>State</dt>
                <dd>{activeIncident?.state ?? 'healthy'}</dd>
              </div>
              <div>
                <dt>Severity</dt>
                <dd>{activeIncident?.severity ?? 'low'}</dd>
              </div>
              <div>
                <dt>Incident WS</dt>
                <dd>{incidentConnected ? 'connected' : 'reconnecting'}</dd>
              </div>
            </dl>
          </div>
        </article>

        {placeholderCards.map((card) => (
          <StatusCard key={card.title} card={card} />
        ))}
      </section>
    </main>
  )
}
