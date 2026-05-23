import type { DashboardCard, DeviceState } from '../types/dashboard'

type StatusCardProps = {
  card: DashboardCard
  deviceState?: DeviceState
}

function formatTimeUTC(timestamp: string): string {
  const parsed = new Date(timestamp)
  if (Number.isNaN(parsed.getTime())) {
    return 'N/A'
  }

  const hh = String(parsed.getUTCHours()).padStart(2, '0')
  const mm = String(parsed.getUTCMinutes()).padStart(2, '0')
  const ss = String(parsed.getUTCSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss} UTC`
}

function truncateLog(entry: string): string {
  const trimmed = entry.trim()
  if (trimmed.length <= 96) {
    return trimmed
  }
  return `${trimmed.slice(0, 93)}...`
}

function statusClass(card: DashboardCard, deviceState?: DeviceState): string {
  if (deviceState) {
    return `status-${deviceState.serviceStatus}`
  }

  switch (card.status) {
    case 'healthy':
      return 'status-running'
    case 'stopped':
      return 'status-stopped'
    case 'degraded':
      return 'status-degraded'
    case 'unknown':
    case 'placeholder':
    default:
      return 'status-unknown'
  }
}

export function StatusCard({ card, deviceState }: StatusCardProps) {
  const rootStatusClass = statusClass(card, deviceState)
  const badgeText = deviceState ? deviceState.serviceStatus : card.status

  return (
    <article className={`status-card ${rootStatusClass}`} aria-label={card.title}>
      <div className={`card-chip badge-${badgeText}`}>{badgeText}</div>
      <h2>{card.title}</h2>
      <p>{card.description}</p>

      {deviceState && (
        <div className="device-metrics">
          <dl className="metrics-grid">
            <div>
              <dt>Service</dt>
              <dd>{deviceState.serviceName}</dd>
            </div>
            <div>
              <dt>Last Seen</dt>
              <dd>{formatTimeUTC(deviceState.lastSeenAt || deviceState.timestamp)}</dd>
            </div>
            <div>
              <dt>CPU</dt>
              <dd>{deviceState.cpuUsage.toFixed(1)}%</dd>
              <div className="metric-bar" aria-hidden="true">
                <span style={{ width: `${Math.min(100, Math.max(0, deviceState.cpuUsage))}%` }} />
              </div>
            </div>
            <div>
              <dt>Memory</dt>
              <dd>{deviceState.memoryUsage.toFixed(1)}%</dd>
              <div className="metric-bar" aria-hidden="true">
                <span style={{ width: `${Math.min(100, Math.max(0, deviceState.memoryUsage))}%` }} />
              </div>
            </div>
          </dl>

          <div className="recent-logs">
            <h3>Recent Logs</h3>
            {deviceState.recentLogs.length === 0 ? (
              <p className="empty-log">No logs received yet.</p>
            ) : (
              <ul>
                {deviceState.recentLogs
                  .slice(-3)
                  .reverse()
                  .map((entry, idx) => (
                    <li key={`${idx}-${entry.slice(0, 12)}`}>{truncateLog(entry)}</li>
                  ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </article>
  )
}
