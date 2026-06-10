import { useMemo } from 'react'

import type { Incident, RemediationActionResult, TimelineEvent } from '../types/dashboard'

type RemediationExecutionPanelProps = {
  incident: Incident | null
}

type ExecutionPhase = {
  key: 'idle' | 'locked' | 'queued' | 'running' | 'succeeded' | 'failed'
  label: string
  chipClass: string
  panelClass: string
}

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return 'N/A'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

function formatDuration(startedAt?: string, finishedAt?: string): string {
  if (!startedAt || !finishedAt) {
    return 'N/A'
  }
  const start = new Date(startedAt).getTime()
  const finish = new Date(finishedAt).getTime()
  if (Number.isNaN(start) || Number.isNaN(finish) || finish < start) {
    return 'N/A'
  }
  const ms = finish - start
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}

// snippet keeps command output compact on the main dashboard — full stdout/stderr
// is preserved in the backend incident record and agent logs, not the UI (Phase 12
// step 4.7 task 4). truncated() reports whether anything was trimmed so the UI can
// signal that more detail exists elsewhere.
function snippet(text: string | undefined, max = 160): string {
  if (!text) {
    return ''
  }
  return text.length > max ? `${text.slice(0, max)}…` : text
}

function truncated(text: string | undefined, max = 160): boolean {
  return Boolean(text && text.length > max)
}

// derivePhase maps an incident's execution state into a single, demo-legible phase.
function derivePhase(incident: Incident): ExecutionPhase {
  const status = incident.remediationStatus
  if (status === 'succeeded') {
    return { key: 'succeeded', label: 'succeeded', chipClass: 'badge-running', panelClass: 'exec-succeeded' }
  }
  if (status === 'failed' || status === 'rejected' || incident.state === 'failed') {
    return { key: 'failed', label: 'failed', chipClass: 'badge-stopped', panelClass: 'exec-failed' }
  }
  if (incident.state === 'executing') {
    return { key: 'running', label: 'running', chipClass: 'badge-degraded', panelClass: 'exec-running' }
  }
  if (incident.state === 'approved') {
    return { key: 'queued', label: 'queued', chipClass: 'badge-placeholder', panelClass: 'exec-queued' }
  }
  // An investigated incident that still needs sign-off makes the human gate
  // explicit: execution is locked until an operator approves (Phase 12 step 4.5).
  if (
    incident.state === 'awaiting_approval' ||
    incident.state === 'detected' ||
    incident.state === 'investigating'
  ) {
    return { key: 'locked', label: 'awaiting approval', chipClass: 'badge-placeholder', panelClass: 'exec-locked' }
  }
  return { key: 'idle', label: 'no remediation', chipClass: 'badge-placeholder', panelClass: 'exec-idle' }
}

function actionStatusClass(status: string): string {
  switch (status) {
    case 'succeeded':
      return 'badge-running'
    case 'failed':
    case 'rejected':
      return 'badge-stopped'
    case 'running':
      return 'badge-degraded'
    default:
      return 'badge-placeholder'
  }
}

const TIMELINE_LABELS: Record<TimelineEvent['type'], string> = {
  command_queued: 'Queued',
  command_dispatched: 'Dispatched',
  command_started: 'Started',
  command_finished: 'Finished',
}

function ActionResultRow({ result }: { result: RemediationActionResult }) {
  const stdout = snippet(result.stdout)
  const stderr = snippet(result.stderr)
  const wasTruncated = truncated(result.stdout) || truncated(result.stderr)
  return (
    <li className="exec-action">
      <div className="exec-action-head">
        <span>
          <code>{result.actionId}</code>
          {result.target ? ` on ${result.target}` : ''}
        </span>
        <span className={`card-chip ${actionStatusClass(result.status)}`}>{result.status}</span>
      </div>
      <div className="exec-action-meta">
        {typeof result.exitCode === 'number' ? <span>exit {result.exitCode}</span> : null}
        <span>{result.durationMs} ms</span>
      </div>
      {stdout ? (
        <pre className="exec-log exec-stdout" aria-label="stdout">
          {stdout}
        </pre>
      ) : null}
      {stderr ? (
        <pre className="exec-log exec-stderr" aria-label="stderr">
          {stderr}
        </pre>
      ) : null}
      {wasTruncated ? <p className="exec-log-truncated">output truncated</p> : null}
    </li>
  )
}

export function RemediationExecutionPanel({ incident }: RemediationExecutionPanelProps) {
  const phase = useMemo(() => (incident ? derivePhase(incident) : null), [incident])

  const approvedActions = incident?.approvedActions ?? []
  const recommendedByID = useMemo(() => {
    const map = new Map<string, string | undefined>()
    for (const action of incident?.recommendedActions ?? []) {
      map.set(action.actionId, action.target)
    }
    return map
  }, [incident?.recommendedActions])

  const results = incident?.remediationResults ?? []
  const timeline = incident?.timeline ?? []

  const isLocked = phase?.key === 'locked'
  // The execution body only renders once remediation has actually been authorized
  // and is queued/running/finished — never before approval.
  const showPanel = phase !== null && phase.key !== 'idle' && phase.key !== 'locked'

  return (
    <article
      className={`status-card exec-panel ${phase && phase.key !== 'idle' ? phase.panelClass : 'exec-idle'}`}
      aria-label="Remediation Execution"
    >
      <div className={`card-chip ${phase && phase.key !== 'idle' ? phase.chipClass : 'badge-placeholder'}`}>
        {phase && phase.key !== 'idle' ? phase.label : 'no remediation'}
      </div>
      <h2>Remediation Execution</h2>
      <p className="exec-gate-note">Endpoint actions run only after operator approval.</p>

      {isLocked ? (
        <div className="exec-gate" role="status">
          <span className="exec-gate-icon" aria-hidden="true">🔒</span>
          <p>
            Locked &mdash; waiting for an operator to approve remediation. Execution
            begins automatically once approval is granted.
          </p>
        </div>
      ) : !showPanel ? (
        <p className="empty-log">
          No remediation executed yet. This panel activates once approved actions are
          queued and dispatched to the agent.
        </p>
      ) : (
        <div className="exec-body">
          <dl className="metrics-grid">
            <div>
              <dt>Execution state</dt>
              <dd>{incident!.state}</dd>
            </div>
            <div>
              <dt>Result</dt>
              <dd>{incident!.remediationStatus ?? 'pending'}</dd>
            </div>
            <div>
              <dt>Started</dt>
              <dd>{formatTimestamp(incident!.remediationStartedAt)}</dd>
            </div>
            <div>
              <dt>Finished</dt>
              <dd>{formatTimestamp(incident!.remediationFinishedAt)}</dd>
            </div>
            <div>
              <dt>Duration</dt>
              <dd>{formatDuration(incident!.remediationStartedAt, incident!.remediationFinishedAt)}</dd>
            </div>
            <div>
              <dt>Request ID</dt>
              <dd>{incident!.remediationRequestId ?? 'N/A'}</dd>
            </div>
          </dl>

          <div className="approval-section">
            <strong>Approved Actions</strong>
            {approvedActions.length === 0 ? (
              <p className="empty-log">No approved actions recorded.</p>
            ) : (
              <ul className="exec-approved-list">
                {approvedActions.map((id) => {
                  const target = recommendedByID.get(id)
                  return (
                    <li key={id}>
                      <code>{id}</code>
                      {target ? ` on ${target}` : ''}
                    </li>
                  )
                })}
              </ul>
            )}
          </div>

          {results.length > 0 ? (
            <div className="approval-section">
              <strong>Per-action Results</strong>
              <ul className="exec-action-list">
                {results.map((result, index) => (
                  <ActionResultRow key={`${result.actionId}-${index}`} result={result} />
                ))}
              </ul>
              <p className="exec-evidence-note">
                Compact view — full command output is retained in the backend incident
                record and agent logs.
              </p>
            </div>
          ) : phase!.key === 'running' ? (
            <p className="empty-log">Execution in progress…</p>
          ) : null}

          {timeline.length > 0 ? (
            <div className="approval-section">
              <strong>Timeline</strong>
              <ol className="exec-timeline">
                {timeline.map((event, index) => (
                  <li key={`${event.type}-${index}`}>
                    <span className="exec-timeline-label">{TIMELINE_LABELS[event.type] ?? event.type}</span>
                    <span className="exec-timeline-time">{formatTimestamp(event.at)}</span>
                  </li>
                ))}
              </ol>
            </div>
          ) : null}
        </div>
      )}
    </article>
  )
}
