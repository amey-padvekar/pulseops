import { StatusCard } from '../components/StatusCard'
import type { DashboardCard } from '../types/dashboard'

type DashboardPageProps = {
  apiBaseUrl: string
}

const cards: DashboardCard[] = [
  {
    title: 'Endpoint Health',
    status: 'placeholder',
    description: 'Live endpoint heartbeat and service status will render here in Phase 3.',
  },
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

export function DashboardPage({ apiBaseUrl }: DashboardPageProps) {
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
      </header>

      <section className="card-grid">
        {cards.map((card) => (
          <StatusCard key={card.title} card={card} />
        ))}
      </section>
    </main>
  )
}
