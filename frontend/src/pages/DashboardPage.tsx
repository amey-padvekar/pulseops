import { StatusCard } from '../components/StatusCard'
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
  const agentDeviceId = import.meta.env.VITE_AGENT_DEVICE_ID || 'LAPTOP-22'
  const { deviceState, connected } = useDeviceState(agentDeviceId)
  const { activeIncident, connected: incidentConnected } = useIncidents(agentDeviceId)

  const endpointCard: DashboardCard = {
    title: 'Endpoint Health',
    status: endpointCardStatus(deviceState?.serviceStatus),
    description: deviceState
      ? `Live status for ${deviceState.deviceId}.`
      : 'Waiting for device telemetry...',
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
