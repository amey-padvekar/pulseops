import { StatusCard } from '../components/StatusCard'
import { useDeviceState } from '../hooks/useDeviceState'
import type { DashboardCard } from '../types/dashboard'

type DashboardPageProps = {
  apiBaseUrl: string
}

const placeholderCards: DashboardCard[] = [
  {
    title: 'Incident Timeline',
    status: 'placeholder',
    description: 'Incident lifecycle events and transitions will be listed here in Phase 4.',
  },
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
        {placeholderCards.map((card) => (
          <StatusCard key={card.title} card={card} />
        ))}
      </section>
    </main>
  )
}
