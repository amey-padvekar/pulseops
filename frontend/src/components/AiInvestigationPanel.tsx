import type { DeviceState, Incident } from '../types/dashboard'

// AiInvestigationPanel presents the AI contribution (Gemini reasoning via Agent
// Builder) in a compact, credible form: probable cause, a confidence meter,
// recommended actions, validation steps, and a little supporting evidence. The
// recommendations are explicitly framed as AI proposals awaiting operator
// approval, so this panel reads as analysis — distinct from the human approval
// and system execution stages that follow (Phase 12 step 4.4).
type AiInvestigationPanelProps = {
  incident: Incident | null
  deviceState?: DeviceState
}

type ConfidenceLevel = 'high' | 'medium' | 'low' | 'na'

function clampConfidence(value: number | undefined): number | null {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return null
  }
  return Math.max(0, Math.min(1, value))
}

function formatConfidence(value: number | undefined): string {
  const clamped = clampConfidence(value)
  if (clamped === null) {
    return 'N/A'
  }
  return `${(clamped * 100).toFixed(0)}%`
}

function confidenceLevel(value: number | undefined): ConfidenceLevel {
  const clamped = clampConfidence(value)
  if (clamped === null) {
    return 'na'
  }
  if (clamped >= 0.75) {
    return 'high'
  }
  if (clamped >= 0.4) {
    return 'medium'
  }
  return 'low'
}

function investigationLabel(status: string | undefined): string {
  if (!status) {
    return 'not started'
  }
  if (status === 'completed') {
    return 'completed'
  }
  if (status === 'pending' || status === 'investigating') {
    return 'in progress'
  }
  if (status === 'fallback') {
    return 'fallback'
  }
  if (status === 'timeout') {
    return 'timed out'
  }
  return status
}

function investigationClassName(status: string | undefined): string {
  if (!status) {
    return 'investigation-idle'
  }
  if (status === 'completed') {
    return 'investigation-completed'
  }
  if (status === 'pending' || status === 'investigating') {
    return 'investigation-pending'
  }
  return 'investigation-failed'
}

function truncate(entry: string, max = 120): string {
  const trimmed = entry.trim()
  if (trimmed.length <= max) {
    return trimmed
  }
  return `${trimmed.slice(0, max - 3)}...`
}

export function AiInvestigationPanel({ incident, deviceState }: AiInvestigationPanelProps) {
  const status = incident?.investigationStatus
  const stateClass = investigationClassName(status)

  // A couple of recent log lines plus the detection trigger give judges a sense
  // of what the model reasoned over, without flooding the panel (step 4.7 owns
  // the broader evidence strategy).
  const keyLogs = (deviceState?.recentLogs ?? []).slice(-2).reverse().map((l) => truncate(l))
  const trigger = incident?.reason

  const level = confidenceLevel(incident?.confidence)
  const confidencePct = clampConfidence(incident?.confidence)

  return (
    <article className={`status-card investigation-card ${stateClass}`} aria-label="AI Investigation">
      <div className="investigation-head">
        <div className="card-chip badge-ai">Gemini &middot; Agent Builder &middot; Elastic MCP</div>
        <span className={`investigation-status-chip ${stateClass}`}>{investigationLabel(status)}</span>
      </div>
      <h2>AI Investigation</h2>

      {!incident ? (
        <p className="empty-log">Investigation not started.</p>
      ) : status === 'completed' ? (
        <div className="investigation-result">
          <div className="ai-headline">
            <div className="ai-cause">
              <span className="ai-field-label">Probable cause</span>
              <p className="ai-cause-text">{incident.probableCause ?? 'N/A'}</p>
            </div>
            <div className="ai-confidence">
              <span className="ai-field-label">Confidence</span>
              <div className="confidence-meter" role="img" aria-label={`Confidence ${formatConfidence(incident.confidence)}`}>
                <span
                  className={`confidence-fill confidence-${level}`}
                  style={{ width: confidencePct === null ? '0%' : `${confidencePct * 100}%` }}
                />
              </div>
              <span className={`confidence-value confidence-text-${level}`}>
                {formatConfidence(incident.confidence)}
              </span>
            </div>
          </div>

          {incident.summary ? <p className="ai-summary">{incident.summary}</p> : null}

          {incident.recommendedActions && incident.recommendedActions.length > 0 ? (
            <div className="ai-section ai-recommended">
              <div className="ai-section-head">
                <strong>Recommended Actions</strong>
                <span className="ai-proposed-tag">AI proposed &middot; awaiting approval</span>
              </div>
              <ul className="ai-action-list">
                {incident.recommendedActions.map((a) => (
                  <li key={a.actionId} className="ai-action">
                    <span className="ai-action-id">{a.actionId}</span>
                    {a.target ? <span className="ai-action-target">on {a.target}</span> : null}
                    {a.description ? <span className="ai-action-desc">{a.description}</span> : null}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {incident.validationSteps && incident.validationSteps.length > 0 ? (
            <div className="ai-section">
              <strong>Validation Steps</strong>
              <ol className="ai-validation-list">
                {incident.validationSteps.map((s, idx) => (
                  <li key={idx}>{s}</li>
                ))}
              </ol>
            </div>
          ) : null}

          {trigger || keyLogs.length > 0 ? (
            <div className="ai-section ai-evidence">
              <strong>Supporting Evidence</strong>
              {trigger ? (
                <p className="ai-evidence-line">
                  <span className="ai-evidence-tag">trigger</span>
                  {truncate(trigger, 140)}
                </p>
              ) : null}
              {keyLogs.map((log, idx) => (
                <p key={idx} className="ai-evidence-line">
                  <span className="ai-evidence-tag">log</span>
                  {log}
                </p>
              ))}
            </div>
          ) : null}
        </div>
      ) : status && status !== 'pending' && status !== 'investigating' ? (
        <div className="investigation-failed">
          <p><strong>Investigation status:</strong> {status}</p>
          {incident.investigationError ? (
            <p className="error">{incident.investigationError}</p>
          ) : (
            <p>Investigation unavailable.</p>
          )}
          {incident.summary ? <p><strong>Summary:</strong> {incident.summary}</p> : null}
        </div>
      ) : (
        // No terminal status yet → the AI is investigating. Surface the orchestration layer
        // here (investigationStatus is only set on completion) so judges see the tools in use.
        <div className="investigation-progress">
          <p className="empty-log investigation-loading">Investigation in progress…</p>
          <div className="ai-orchestration" aria-label="Investigating via">
            <span className="ai-orchestration-label">Investigating via</span>
            <span className="card-chip badge-ai">Gemini</span>
            <span className="card-chip badge-ai">Agent Builder</span>
            <span className="card-chip badge-ai">Elastic MCP</span>
          </div>
        </div>
      )}
    </article>
  )
}
