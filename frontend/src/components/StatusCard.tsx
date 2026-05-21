import type { DashboardCard } from '../types/dashboard'

type StatusCardProps = {
  card: DashboardCard
}

export function StatusCard({ card }: StatusCardProps) {
  return (
    <article className="status-card" aria-label={card.title}>
      <div className="card-chip">{card.status}</div>
      <h2>{card.title}</h2>
      <p>{card.description}</p>
    </article>
  )
}
