import type { DeviceState, Incident, ServiceStatus } from '../types/dashboard'

// HeroState is the single, instantly-readable operational state shown at the top
// of the dashboard. It blends live device telemetry (running/degraded/stopped)
// with incident lifecycle framing (recovering/resolved/failed) so a viewer can
// tell at a glance whether the endpoint is healthy, in trouble, being fixed, or
// recovered — and compare before/after during demo narration.
type HeroState =
  | 'healthy'
  | 'degraded'
  | 'stopped'
  | 'recovering'
  | 'resolved'
  | 'failed'
  | 'unknown'
  | 'idle'

type EndpointHeroProps = {
  deviceState?: DeviceState
  // incident is the active-or-latest incident; it supplies the recovering/resolved
  // framing that raw service status cannot express.
  incident?: Incident | null
  telemetryConnected: boolean
}

const HERO_COPY: Record<HeroState, { word: string; sub: string }> = {
  healthy: { word: 'Healthy', sub: 'All systems operational' },
  degraded: { word: 'Degraded', sub: 'Service performance impaired' },
  stopped: { word: 'Stopped', sub: 'Monitored service is down' },
  recovering: { word: 'Recovering', sub: 'Remediation in progress' },
  resolved: { word: 'Resolved', sub: 'Service restored after incident' },
  failed: { word: 'Action Failed', sub: 'Remediation did not restore service' },
  unknown: { word: 'Unknown', sub: 'Awaiting telemetry' },
  idle: { word: 'Idle', sub: 'No device connected — run a simulation to begin' },
}

function formatTimeUTC(timestamp: string | undefined): string {
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

function deriveHeroState(deviceState: DeviceState | undefined, incident: Incident | null | undefined): HeroState {
  // Incident lifecycle takes precedence so the demo narrative (recovering ->
  // resolved / failed) is unmistakable even while raw telemetry catches up.
  const incidentState = incident?.state
  if (incidentState === 'resolved') {
    return 'resolved'
  }
  if (incidentState === 'failed') {
    return 'failed'
  }
  if (incidentState === 'approved' || incidentState === 'executing' || incidentState === 'validating') {
    return 'recovering'
  }

  // Detected / investigating / awaiting_approval fall through to the live service
  // status, which is what actually communicates "in trouble" at that point.
  const serviceStatus: ServiceStatus | undefined = deviceState?.serviceStatus
  switch (serviceStatus) {
    case 'running':
      return 'healthy'
    case 'degraded':
      return 'degraded'
    case 'stopped':
      return 'stopped'
    default:
      // No telemetry yet. With no incident either, this is a calm idle baseline
      // (e.g. no device connected), not a problem state — so it reads neutral, not red.
      return incident ? 'unknown' : 'idle'
  }
}

export function EndpointHero({ deviceState, incident, telemetryConnected }: EndpointHeroProps) {
  const state = deriveHeroState(deviceState, incident)
  const copy = HERO_COPY[state]

  const deviceId = deviceState?.deviceId ?? incident?.deviceId ?? 'awaiting device'
  const serviceName = deviceState?.serviceName ?? incident?.serviceName ?? 'N/A'
  const liveStatus: ServiceStatus | string = deviceState?.serviceStatus ?? 'unknown'
  const heartbeatOk = deviceState?.heartbeat ?? false
  const networkOk = deviceState?.networkReachable ?? false
  const lastSeen = formatTimeUTC(deviceState?.lastSeenAt || deviceState?.timestamp)

  // "recovered" is styled distinctly from a calm "healthy" baseline so the
  // before/after transition is obvious during narration.
  const recovered = state === 'resolved'

  // A compact latest-telemetry snapshot (CPU + memory) is the headline evidence
  // for the endpoint's health; thin inline bars keep it informative, not cluttered
  // (Phase 12 step 4.7). Fuller telemetry detail lives in the backend.
  const hasTelemetry = Boolean(deviceState)
  const cpu = deviceState ? Math.min(100, Math.max(0, deviceState.cpuUsage)) : 0
  const memory = deviceState ? Math.min(100, Math.max(0, deviceState.memoryUsage)) : 0

  return (
    <article className={`hero-panel hero-${state}`} aria-label="Endpoint Health">
      <div className="hero-banner">
        <span className="hero-dot" aria-hidden="true" />
        <div className="hero-headline">
          <p className="hero-state-word">{copy.word}</p>
          <p className="hero-state-sub">{copy.sub}</p>
        </div>
        <div className="hero-flags">
          {recovered ? <span className="hero-flag hero-flag-recovered">recovered</span> : null}
          <span className={`hero-flag ${telemetryConnected ? 'hero-flag-live' : 'hero-flag-stale'}`}>
            {telemetryConnected ? 'live' : 'reconnecting'}
          </span>
        </div>
      </div>

      <dl className="hero-facts">
        <div>
          <dt>Device</dt>
          <dd>{deviceId}</dd>
        </div>
        <div>
          <dt>Service</dt>
          <dd>{serviceName}</dd>
        </div>
        <div>
          <dt>Service Status</dt>
          <dd className={`hero-status-value status-text-${liveStatus}`}>{liveStatus}</dd>
        </div>
        <div>
          <dt>Connectivity</dt>
          <dd>
            {deviceState ? (
              <>
                <span className={heartbeatOk ? 'hero-ok' : 'hero-bad'}>
                  {heartbeatOk ? 'heartbeat' : 'no heartbeat'}
                </span>
                {' · '}
                <span className={networkOk ? 'hero-ok' : 'hero-bad'}>
                  {networkOk ? 'network up' : 'network down'}
                </span>
              </>
            ) : (
              <span className="hero-muted">awaiting telemetry</span>
            )}
          </dd>
        </div>
        <div>
          <dt>Last Telemetry</dt>
          <dd>{lastSeen}</dd>
        </div>
      </dl>

      {hasTelemetry ? (
        <div className="hero-telemetry" aria-label="Latest telemetry snapshot">
          <div className="hero-metric">
            <div className="hero-metric-head">
              <span className="hero-metric-label">CPU</span>
              <span className="hero-metric-value">{cpu.toFixed(0)}%</span>
            </div>
            <div className="hero-metric-bar" aria-hidden="true">
              <span className={`hero-metric-fill ${cpu >= 85 ? 'metric-hot' : ''}`} style={{ width: `${cpu}%` }} />
            </div>
          </div>
          <div className="hero-metric">
            <div className="hero-metric-head">
              <span className="hero-metric-label">Memory</span>
              <span className="hero-metric-value">{memory.toFixed(0)}%</span>
            </div>
            <div className="hero-metric-bar" aria-hidden="true">
              <span className={`hero-metric-fill ${memory >= 85 ? 'metric-hot' : ''}`} style={{ width: `${memory}%` }} />
            </div>
          </div>
        </div>
      ) : null}
    </article>
  )
}
