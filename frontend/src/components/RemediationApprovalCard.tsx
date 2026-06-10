import { useMemo, useState } from 'react'

import { useApproveIncident } from '../hooks/useApproveIncident'
import type { Incident } from '../types/dashboard'

type RemediationApprovalCardProps = {
  incident: Incident | null
}

function formatConfidence(value: number | undefined): string {
  if (typeof value !== 'number' || Number.isNaN(value)) {
    return 'N/A'
  }
  return `${(Math.max(0, Math.min(1, value)) * 100).toFixed(0)}%`
}

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return 'N/A'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

const DEFAULT_OPERATOR = 'demo.operator'

export function RemediationApprovalCard({ incident }: RemediationApprovalCardProps) {
  const { submitting, error, status, approve, reset } = useApproveIncident()

  const recommendedActions = useMemo(
    () => incident?.recommendedActions ?? [],
    [incident?.recommendedActions],
  )
  const hasRecommendation = recommendedActions.length > 0

  const [approvedBy, setApprovedBy] = useState(DEFAULT_OPERATOR)
  const [note, setNote] = useState('')
  const [selectedActionIds, setSelectedActionIds] = useState<string[]>([])

  // Reset the selection (and any prior feedback) whenever a different incident or a
  // new set of recommended actions arrives, defaulting to every action selected.
  // This adjusts state during render (the React-endorsed alternative to an effect)
  // and re-initializes when recommendations appear on an already-tracked incident.
  const selectionKey = `${incident?.incidentId ?? ''}::${recommendedActions.map((a) => a.actionId).join(',')}`
  const [trackedSelectionKey, setTrackedSelectionKey] = useState<string | null>(null)
  if (selectionKey !== trackedSelectionKey) {
    setTrackedSelectionKey(selectionKey)
    setSelectedActionIds(recommendedActions.map((action) => action.actionId))
    reset()
  }

  const toggleAction = (actionId: string) => {
    setSelectedActionIds((previous) =>
      previous.includes(actionId)
        ? previous.filter((id) => id !== actionId)
        : [...previous, actionId],
    )
  }

  const isApproved = incident?.state === 'approved'
  const isAwaitingApproval = incident?.state === 'awaiting_approval'
  const canApprove =
    isAwaitingApproval &&
    hasRecommendation &&
    selectedActionIds.length > 0 &&
    approvedBy.trim().length > 0 &&
    !submitting

  const handleApprove = async () => {
    if (!incident || !canApprove) {
      return
    }
    await approve(incident.incidentId, {
      approvedBy: approvedBy.trim(),
      selectedActionIds,
      note: note.trim() || undefined,
    })
  }

  const chipLabel = isApproved
    ? 'approved'
    : isAwaitingApproval
      ? 'awaiting approval'
      : hasRecommendation
        ? incident?.state ?? 'pending'
        : 'no recommendation'

  const chipClass = isApproved
    ? 'badge-running'
    : isAwaitingApproval
      ? 'badge-stopped'
      : 'badge-placeholder'

  return (
    <article
      className={`status-card approval-panel ${isApproved ? 'approval-approved' : isAwaitingApproval ? 'approval-awaiting' : 'approval-idle'}`}
      aria-label="Remediation Approval"
    >
      <div className={`card-chip ${chipClass}`}>{chipLabel}</div>
      <h2>Remediation Approval</h2>

      {!incident || !hasRecommendation ? (
        <p className="empty-log">
          No AI recommendation to review yet. Approval unlocks once an investigation
          completes with recommended actions.
        </p>
      ) : (
        <div className="approval-body">
          <dl className="metrics-grid">
            <div>
              <dt>Probable cause</dt>
              <dd>{incident.probableCause ?? 'N/A'}</dd>
            </div>
            <div>
              <dt>Confidence</dt>
              <dd>{formatConfidence(incident.confidence)}</dd>
            </div>
            <div>
              <dt>Approval state</dt>
              <dd>{incident.state}</dd>
            </div>
          </dl>

          <div className="approval-section">
            <strong>Recommended Actions</strong>
            <ul className="approval-action-list">
              {recommendedActions.map((action) => {
                const checked = selectedActionIds.includes(action.actionId)
                const isApprovedAction = incident.approvedActions?.includes(action.actionId)
                return (
                  <li key={action.actionId}>
                    {isAwaitingApproval ? (
                      <label className="approval-action-option">
                        <input
                          type="checkbox"
                          checked={checked}
                          disabled={submitting}
                          onChange={() => toggleAction(action.actionId)}
                        />
                        <span>
                          <code>{action.actionId}</code>
                          {action.target ? ` on ${action.target}` : ''}
                          {action.description ? ` — ${action.description}` : ''}
                        </span>
                      </label>
                    ) : (
                      <span className={isApprovedAction ? 'approval-action-approved' : ''}>
                        <code>{action.actionId}</code>
                        {action.target ? ` on ${action.target}` : ''}
                        {action.description ? ` — ${action.description}` : ''}
                        {isApprovedAction ? ' ✓' : ''}
                      </span>
                    )}
                  </li>
                )
              })}
            </ul>
          </div>

          {incident.validationSteps && incident.validationSteps.length > 0 ? (
            <div className="approval-section">
              <strong>Validation Steps</strong>
              <ol>
                {incident.validationSteps.map((step, index) => (
                  <li key={index}>{step}</li>
                ))}
              </ol>
            </div>
          ) : null}

          {isApproved ? (
            <div className="approval-result" role="status">
              <dl className="approval-meta">
                <div>
                  <dt>Approved by</dt>
                  <dd>{incident.approvedBy ?? 'unknown'}</dd>
                </div>
                <div>
                  <dt>Approved at</dt>
                  <dd>{formatTimestamp(incident.approvedAt)}</dd>
                </div>
              </dl>
              {incident.approvalNote ? (
                <p className="approval-note">&ldquo;{incident.approvalNote}&rdquo;</p>
              ) : null}
              <p className="approval-handoff">
                ✓ Approved actions handed off to execution &rarr;
              </p>
            </div>
          ) : isAwaitingApproval ? (
            <div className="approval-controls">
              <label htmlFor="approval-operator">Approver identity</label>
              <input
                id="approval-operator"
                type="text"
                className="approval-input"
                value={approvedBy}
                disabled={submitting}
                onChange={(event) => setApprovedBy(event.target.value)}
                placeholder="operator id"
              />

              <label htmlFor="approval-note">Note (optional)</label>
              <textarea
                id="approval-note"
                className="approval-input"
                value={note}
                disabled={submitting}
                maxLength={500}
                rows={2}
                onChange={(event) => setNote(event.target.value)}
                placeholder="Reason for approving these actions"
              />

              <button
                type="button"
                className="approval-button"
                disabled={!canApprove}
                onClick={handleApprove}
              >
                {submitting ? 'Approving…' : 'Approve remediation'}
              </button>

              {status === 'error' && error ? (
                <p className="approval-feedback approval-feedback-error" role="alert">
                  {error}
                </p>
              ) : null}
            </div>
          ) : (
            <p className="empty-log">
              Approval is locked while the incident is <code>{incident.state}</code>.
              Controls unlock when the incident is <code>awaiting_approval</code>.
            </p>
          )}
        </div>
      )}
    </article>
  )
}
