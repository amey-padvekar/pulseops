import { useMemo } from 'react'

import type { Incident } from '../types/dashboard'

type ValidationPanelProps = {
  incident: Incident | null
}

type ValidationPhase = {
  key: 'idle' | 'executing' | 'validating' | 'resolved' | 'failed' | 'command_failed'
  label: string
  chipClass: string
  panelClass: string
}

// validationStarted reports whether recovery validation actually began for an incident.
// A command that failed before validation (executing -> failed) has no validation
// boundary or status, which is how we tell a command failure apart from a validation
// failure (validating -> failed) — the key operator distinction in Phase 10 step 4.9.
function validationStarted(incident: Incident): boolean {
  return Boolean(incident.validationBoundaryAt) || incident.validationStatus === 'failed' || incident.validationStatus === 'succeeded'
}

const IDLE_PHASE: ValidationPhase = {
  key: 'idle',
  label: 'no validation',
  chipClass: 'badge-placeholder',
  panelClass: 'validation-idle',
}

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return 'N/A'
  }
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString()
}

function validationStatusLabel(status: string | undefined, phaseKey: ValidationPhase['key']): string {
  switch (status) {
    case 'succeeded':
      return 'succeeded'
    case 'failed':
      return 'failed'
    case 'in_progress':
      return 'in progress'
    default:
      return phaseKey === 'executing' ? 'pending' : 'in progress'
  }
}

// derivePhase maps an incident's lifecycle state into a single recovery-validation phase
// so executing / validating / resolved / failed are each visually distinct (task 3).
function derivePhase(incident: Incident | null): ValidationPhase {
  if (!incident) {
    return IDLE_PHASE
  }
  switch (incident.state) {
    case 'resolved':
      return { key: 'resolved', label: 'recovery proven', chipClass: 'badge-running', panelClass: 'validation-resolved' }
    case 'failed':
      // A failure that never reached validation is a command failure, not a recovery
      // (validation) failure — surface them differently so operators know whether the
      // command ran at all.
      return validationStarted(incident)
        ? { key: 'failed', label: 'recovery not confirmed', chipClass: 'badge-stopped', panelClass: 'validation-failed' }
        : { key: 'command_failed', label: 'command failed', chipClass: 'badge-stopped', panelClass: 'validation-failed' }
    case 'validating':
      return { key: 'validating', label: 'validating recovery', chipClass: 'badge-degraded', panelClass: 'validation-validating' }
    case 'executing':
      return { key: 'executing', label: 'executing remediation', chipClass: 'badge-placeholder', panelClass: 'validation-executing' }
    default:
      return IDLE_PHASE
  }
}

// HealthyCycleProgress renders the consecutive-healthy-cycle counter as a row of dots so
// the "recovery is being proven, not assumed" idea reads at a glance. While validation is
// still in progress, the next dot to fill pulses so the panel reads as actively confirming
// recovery (Phase 12 step 4.6 task 1).
function HealthyCycleProgress({ count, required, active }: { count: number; required: number; active?: boolean }) {
  const total = Math.max(required, count, 1)
  const dots = Array.from({ length: total }, (_, index) => index)
  return (
    <div className="validation-cycles" aria-label={`${count} of ${required} healthy cycles`}>
      <div className="validation-cycle-dots">
        {dots.map((index) => {
          const filled = index < count
          const isNext = active && index === count
          const dotClass = filled ? 'cycle-filled' : isNext ? 'cycle-next' : 'cycle-empty'
          return <span key={index} className={`validation-cycle-dot ${dotClass}`} />
        })}
      </div>
      <span className="validation-cycle-text">
        {count} / {required} healthy cycles
      </span>
    </div>
  )
}

export function ValidationPanel({ incident }: ValidationPanelProps) {
  const phase = useMemo(() => derivePhase(incident), [incident])
  const showPanel = phase.key !== 'idle'

  const snapshot = incident?.lastValidationSnapshot
  const healthyCount = incident?.healthyCycleCount ?? 0
  const requiredCycles = incident?.requiredHealthyCycles ?? 2

  return (
    <article
      className={`status-card validation-panel ${showPanel ? phase.panelClass : 'validation-idle'}`}
      aria-label="Recovery Validation"
    >
      <div className={`card-chip ${showPanel ? phase.chipClass : 'badge-placeholder'}`}>
        {showPanel ? phase.label : 'no validation'}
      </div>
      <h2>Recovery Validation</h2>

      {!showPanel ? (
        <p className="empty-log">
          Recovery validation activates after remediation runs. It confirms the endpoint
          actually returned to health before the incident is closed.
        </p>
      ) : phase.key === 'command_failed' ? (
        <div className="validation-body">
          <p className="validation-outcome validation-outcome-failed">
            ✕ Remediation command failed before recovery could be validated.
          </p>
          <p className="validation-command-failed-note">
            {incident!.reason ?? 'The command did not complete successfully.'} See the
            Remediation Execution panel for command output, then re-run remediation.
          </p>
        </div>
      ) : (
        <div className="validation-body">
          <dl className="metrics-grid">
            <div>
              <dt>Lifecycle state</dt>
              <dd>
                <span className={`validation-state-pill validation-state-${phase.key}`}>{incident!.state}</span>
              </dd>
            </div>
            <div>
              <dt>Validation</dt>
              <dd>{validationStatusLabel(incident!.validationStatus, phase.key)}</dd>
            </div>
            <div>
              <dt>Started</dt>
              <dd>{formatTimestamp(incident!.validationBoundaryAt)}</dd>
            </div>
            <div>
              <dt>Ended</dt>
              <dd>{formatTimestamp(incident!.validatedAt)}</dd>
            </div>
          </dl>

          {phase.key === 'validating' || phase.key === 'resolved' || phase.key === 'failed' ? (
            <div className="validation-section">
              <strong>Healthy cycle progress</strong>
              <HealthyCycleProgress
                count={healthyCount}
                required={requiredCycles}
                active={phase.key === 'validating'}
              />
              {phase.key === 'validating' ? (
                <p className="validation-progress-note">
                  Confirming the endpoint stays healthy across consecutive cycles…
                </p>
              ) : null}
            </div>
          ) : null}

          <div className="validation-section">
            <strong>Validation criteria</strong>
            {snapshot && snapshot.checks.length > 0 ? (
              <ul className="validation-criteria-list">
                {snapshot.checks.map((check) => (
                  <li key={check.name} className="validation-criterion">
                    <span className={`card-chip ${check.passed ? 'badge-running' : 'badge-stopped'}`}>
                      {check.passed ? 'pass' : 'fail'}
                    </span>
                    <span className="validation-criterion-text">
                      <code>{check.name}</code>
                      {check.required ? '' : ' (optional)'} — {check.detail}
                    </span>
                  </li>
                ))}
              </ul>
            ) : (
              <p className="empty-log">Awaiting first post-remediation telemetry…</p>
            )}
            {snapshot ? (
              <p className="validation-snapshot-meta">
                Last checked {formatTimestamp(snapshot.observedAt)} · service{' '}
                <strong>{snapshot.serviceStatus}</strong> · heartbeat{' '}
                <strong>{snapshot.heartbeat ? 'yes' : 'no'}</strong>
              </p>
            ) : null}
          </div>

          {phase.key === 'resolved' ? (
            <p className="validation-outcome validation-outcome-resolved">
              ✓ Recovery proven by {Math.max(healthyCount, requiredCycles)} healthy telemetry{' '}
              {Math.max(healthyCount, requiredCycles) === 1 ? 'cycle' : 'cycles'}. Incident resolved.
            </p>
          ) : phase.key === 'failed' ? (
            <>
              <p className="validation-outcome validation-outcome-failed">
                ✕ {incident!.validationFailureReason ?? incident!.reason ?? 'Recovery could not be confirmed.'}
              </p>
              <p className="validation-command-failed-note">
                The remediation command ran, but the endpoint did not return to health.
                The recommendation is retained for manual re-run or further investigation.
              </p>
            </>
          ) : null}
        </div>
      )}
    </article>
  )
}
