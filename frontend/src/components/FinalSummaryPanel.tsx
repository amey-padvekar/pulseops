import { useMemo, useState } from 'react'

import type { FinalSummary, Incident } from '../types/dashboard'

type FinalSummaryPanelProps = {
  incident: Incident | null
}

type SummaryPhase = {
  key: 'idle' | 'pending' | 'ready' | 'failed'
  label: string
  chipClass: string
  panelClass: string
}

const IDLE_PHASE: SummaryPhase = {
  key: 'idle',
  label: 'no summary yet',
  chipClass: 'badge-placeholder',
  panelClass: 'summary-idle',
}

// incidentClosed reports whether the lifecycle has reached a terminal state, the only
// point at which a final summary is meaningful (mirrors the backend trigger rule).
function incidentClosed(incident: Incident): boolean {
  return incident.state === 'resolved' || incident.state === 'failed'
}

// derivePhase maps the incident's summary-generation status into one display phase so the
// panel shows a clear loading / ready / fallback state (task 3).
function derivePhase(incident: Incident | null): SummaryPhase {
  if (!incident) {
    return IDLE_PHASE
  }

  // A generated summary always wins, even if the incident reopened, so the report stays
  // visible after refresh.
  if (incident.finalSummary && incident.summaryStatus !== 'pending') {
    const failedOutcome = incident.state === 'failed'
    return {
      key: 'ready',
      label: failedOutcome ? 'incident failed' : 'incident resolved',
      chipClass: failedOutcome ? 'badge-stopped' : 'badge-running',
      panelClass: failedOutcome ? 'summary-failed-outcome' : 'summary-ready',
    }
  }

  if (incident.summaryStatus === 'pending') {
    return { key: 'pending', label: 'generating…', chipClass: 'badge-degraded', panelClass: 'summary-pending' }
  }

  if (incident.summaryStatus === 'failed') {
    return { key: 'failed', label: 'summary unavailable', chipClass: 'badge-stopped', panelClass: 'summary-failed' }
  }

  // Terminal incident but no summary status yet: generation is imminent.
  if (incidentClosed(incident)) {
    return { key: 'pending', label: 'generating…', chipClass: 'badge-degraded', panelClass: 'summary-pending' }
  }

  return IDLE_PHASE
}

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return 'N/A'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

// formatSummaryText renders the summary as clean, deterministic plain text suitable for
// copy/paste into submission notes, demo narration, or a chat/issue description (step 4.9).
// Plain text (not markdown) keeps it readable everywhere it lands.
function formatSummaryText(incident: Incident, summary: FinalSummary): string {
  const lines: string[] = []
  lines.push(`Incident Summary — ${incident.incidentId}`)
  lines.push(`Device: ${incident.deviceId} · Service: ${incident.serviceName}`)
  lines.push(`Outcome: ${incident.state}`)
  lines.push('')
  if (summary.operatorSummary) {
    lines.push(summary.operatorSummary)
    lines.push('')
  }
  lines.push(`Root cause: ${summary.rootCause}`)
  lines.push('')
  lines.push('Evidence:')
  for (const item of summary.evidence) {
    lines.push(`  - ${item}`)
  }
  lines.push('')
  lines.push('Actions taken:')
  for (const item of summary.actionsTaken) {
    lines.push(`  - ${item}`)
  }
  lines.push('')
  lines.push(`Result: ${summary.result}`)
  if (incident.summaryGeneratedAt) {
    lines.push('')
    lines.push(`Generated: ${formatTimestamp(incident.summaryGeneratedAt)}`)
  }
  return lines.join('\n')
}

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // fall through to legacy path
  }
  try {
    const textarea = document.createElement('textarea')
    textarea.value = text
    textarea.style.position = 'fixed'
    textarea.style.opacity = '0'
    document.body.appendChild(textarea)
    textarea.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(textarea)
    return ok
  } catch {
    return false
  }
}

function CopySummaryButton({ incident, summary }: { incident: Incident; summary: FinalSummary }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    const ok = await copyText(formatSummaryText(incident, summary))
    if (ok) {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    }
  }

  return (
    <button
      type="button"
      className="summary-copy-button"
      onClick={handleCopy}
      aria-label="Copy incident summary to clipboard"
    >
      {copied ? '✓ Copied' : 'Copy'}
    </button>
  )
}

// buildReportMarkdown assembles a self-contained Markdown incident report. It prefers the
// AI-written FinalSummary when one was generated, but always falls back to raw incident fields
// so the export never depends on a live agent (C7).
function buildReportMarkdown(incident: Incident): string {
  const summary =
    incident.finalSummary &&
    (incident.summaryStatus === 'generated' || incident.summaryStatus === 'fallback')
      ? incident.finalSummary
      : null

  const rootCause = summary
    ? summary.rootCause
    : incident.probableCause ?? incident.reason ?? 'Not recorded.'

  const evidence =
    summary && summary.evidence.length > 0
      ? summary.evidence
      : [
          incident.reason ? `Trigger: ${incident.reason}` : null,
          incident.summary ? `AI summary: ${incident.summary}` : null,
          incident.lastValidationSnapshot
            ? `Recovery check: ${incident.lastValidationSnapshot.reason}`
            : null,
        ].filter((line): line is string => line !== null)

  const actions =
    summary && summary.actionsTaken.length > 0
      ? summary.actionsTaken
      : (incident.remediationResults ?? []).map(
          (r) => `${r.actionId}${r.target ? ` on ${r.target}` : ''} — ${r.status}`,
        )

  const result = summary
    ? summary.result
    : incident.validationStatus === 'succeeded'
      ? 'Service recovered and verified.'
      : incident.state === 'failed'
        ? 'Remediation did not confirm recovery.'
        : `Final state: ${incident.state}.`

  const lines: string[] = [
    `# Incident Report — ${incident.incidentId}`,
    '',
    `- **Device:** ${incident.deviceId}`,
    `- **Service:** ${incident.serviceName}`,
    `- **Severity:** ${incident.severity}`,
    `- **Outcome:** ${incident.state}`,
    `- **Detected:** ${formatTimestamp(incident.detectedAt)}`,
  ]
  if (incident.validatedAt) {
    lines.push(`- **Resolved:** ${formatTimestamp(incident.validatedAt)}`)
  }
  lines.push('')

  if (summary?.operatorSummary) {
    lines.push(summary.operatorSummary, '')
  }

  lines.push('## Root Cause', rootCause, '')

  lines.push('## Evidence')
  lines.push(...(evidence.length > 0 ? evidence.map((e) => `- ${e}`) : ['- None recorded.']))
  lines.push('')

  lines.push('## Actions Taken')
  lines.push(
    ...(actions.length > 0
      ? actions.map((a) => `- ${a}`)
      : ['- No remediation actions were executed.']),
  )
  lines.push('')

  lines.push('## Result', result, '')

  lines.push('---')
  lines.push(
    `Generated by PulseOps${
      incident.summaryGeneratedAt ? ` · ${formatTimestamp(incident.summaryGeneratedAt)}` : ''
    } · Diagnosis: Gemini + Agent Builder + Elastic MCP`,
  )

  return lines.join('\n')
}

function downloadReport(incident: Incident): void {
  const blob = new Blob([buildReportMarkdown(incident)], { type: 'text/markdown;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `incident-${incident.incidentId}.md`
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  URL.revokeObjectURL(url)
}

function DownloadReportButton({ incident }: { incident: Incident }) {
  return (
    <button
      type="button"
      className="summary-copy-button summary-download-button"
      onClick={() => downloadReport(incident)}
      aria-label="Download incident report as Markdown"
    >
      Download Report
    </button>
  )
}

export function FinalSummaryPanel({ incident }: FinalSummaryPanelProps) {
  const phase = useMemo(() => derivePhase(incident), [incident])
  const summary = incident?.finalSummary

  return (
    <article
      className={`status-card summary-panel ${phase.panelClass}`}
      aria-label="Incident Summary"
    >
      <div className="summary-header">
        <div className={`card-chip ${phase.chipClass}`}>{phase.label}</div>
        {incident && (incidentClosed(incident) || incident.finalSummary) ? (
          <div className="summary-actions">
            {phase.key === 'ready' && summary ? (
              <CopySummaryButton incident={incident} summary={summary} />
            ) : null}
            <DownloadReportButton incident={incident} />
          </div>
        ) : null}
      </div>
      <h2>Incident Summary</h2>

      {phase.key === 'idle' ? (
        <p className="empty-log">
          A final report is generated once the incident is resolved or failed. It explains
          what happened, the evidence, the actions taken, and the outcome.
        </p>
      ) : phase.key === 'pending' ? (
        <p className="empty-log summary-loading">Generating final incident report…</p>
      ) : phase.key === 'failed' && !summary ? (
        <div className="summary-body">
          <p className="summary-result summary-result-failed">
            ✕ The final report could not be generated for this incident.
          </p>
          <p className="summary-fallback-note">
            The incident record remains the source of truth — see the Recovery Validation
            and Remediation Execution panels for the full outcome and evidence.
          </p>
        </div>
      ) : summary ? (
        <div className="summary-body">
          {incident?.summaryStatus === 'fallback' ? (
            <p className="summary-fallback-banner">
              Auto-generated from the incident record — the live AI narrative was
              unavailable.
            </p>
          ) : null}

          {summary.operatorSummary ? (
            <p className="summary-operator">{summary.operatorSummary}</p>
          ) : null}

          <div className="summary-section">
            <strong>Root cause</strong>
            <p className="summary-root-cause">{summary.rootCause}</p>
          </div>

          <div className="summary-section">
            <strong>Evidence</strong>
            {summary.evidence.length > 0 ? (
              <ul className="summary-list">
                {summary.evidence.map((item, idx) => (
                  <li key={idx}>{item}</li>
                ))}
              </ul>
            ) : (
              <p className="empty-log">No evidence recorded.</p>
            )}
          </div>

          <div className="summary-section">
            <strong>Actions taken</strong>
            {summary.actionsTaken.length > 0 ? (
              <ul className="summary-list">
                {summary.actionsTaken.map((item, idx) => (
                  <li key={idx}>{item}</li>
                ))}
              </ul>
            ) : (
              <p className="empty-log">No remediation actions were executed.</p>
            )}
          </div>

          <p
            className={`summary-result ${
              incident?.state === 'failed' ? 'summary-result-failed' : 'summary-result-resolved'
            }`}
          >
            {incident?.state === 'failed' ? '✕' : '✓'} {summary.result}
          </p>

          {incident?.summaryGeneratedAt ? (
            <p className="summary-meta">Report generated {formatTimestamp(incident.summaryGeneratedAt)}</p>
          ) : null}
        </div>
      ) : null}
    </article>
  )
}
