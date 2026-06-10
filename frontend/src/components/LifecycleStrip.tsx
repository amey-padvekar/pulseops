import type { Incident, IncidentState } from '../types/dashboard'

// LifecycleStrip renders the incident's journey through the system as a compact
// horizontal strip: detected -> investigating -> awaiting approval -> approved ->
// executing -> validating -> resolved (or failed). Completed steps, the current
// step, and pending steps are visually distinct so the dashboard communicates
// process progression without verbal explanation (Phase 12 step 4.3).
// 'root_cause_identified' is a display-only node (no backend lifecycle state) inserted to give
// the AI's diagnosis its own beat — judges remember "the AI figured out WHY." Its status is
// derived from the investigation result, not from incident.state.
type LifecycleStepKey = IncidentState | 'root_cause_identified'

type LifecycleStep = {
  key: LifecycleStepKey
  label: string
  // getTs returns the timestamp at which the incident reached this step, when the
  // backend records one. Steps without a dedicated timestamp simply omit it.
  getTs: (incident: Incident) => string | undefined
}

const STEPS: LifecycleStep[] = [
  { key: 'detected', label: 'Detected', getTs: (i) => i.detectedAt },
  { key: 'investigating', label: 'Investigating', getTs: (i) => i.investigatedAt },
  { key: 'root_cause_identified', label: 'Root Cause', getTs: (i) => i.investigatedAt },
  { key: 'awaiting_approval', label: 'Awaiting Approval', getTs: () => undefined },
  { key: 'approved', label: 'Approved', getTs: (i) => i.approvedAt },
  { key: 'executing', label: 'Executing', getTs: (i) => i.remediationStartedAt },
  { key: 'validating', label: 'Validating', getTs: (i) => i.validationBoundaryAt },
  { key: 'resolved', label: 'Resolved', getTs: (i) => i.validatedAt ?? i.summaryGeneratedAt },
]

const TERMINAL_INDEX = STEPS.length - 1

function formatTimeUTC(timestamp: string | undefined): string | undefined {
  if (!timestamp) {
    return undefined
  }
  const parsed = new Date(timestamp)
  if (Number.isNaN(parsed.getTime())) {
    return undefined
  }
  const hh = String(parsed.getUTCHours()).padStart(2, '0')
  const mm = String(parsed.getUTCMinutes()).padStart(2, '0')
  const ss = String(parsed.getUTCSeconds()).padStart(2, '0')
  return `${hh}:${mm}:${ss}`
}

// currentIndex maps the incident's lifecycle state onto the strip. 'failed' is a
// terminal outcome that can occur from several points; it occupies the final node
// so the strip ends on an unmistakable failure marker.
function currentIndex(state: IncidentState): number {
  const idx = STEPS.findIndex((step) => step.key === state)
  if (idx >= 0) {
    return idx
  }
  if (state === 'failed') {
    return TERMINAL_INDEX
  }
  // 'healthy' or anything unexpected: treat as just-detected baseline.
  return 0
}

type StepStatus = 'complete' | 'current' | 'pending' | 'failed'

export function LifecycleStrip({ incident }: { incident: Incident | null }) {
  const failed = incident?.state === 'failed'
  const activeIndex = incident ? currentIndex(incident.state) : -1

  return (
    <ol className="lifecycle-strip" aria-label="Incident lifecycle">
      {STEPS.map((step, idx) => {
        const isTerminal = idx === TERMINAL_INDEX

        let status: StepStatus
        if (activeIndex < 0 || idx > activeIndex) {
          status = 'pending'
        } else if (idx < activeIndex) {
          status = 'complete'
        } else {
          status = failed && isTerminal ? 'failed' : 'current'
        }

        // The synthetic "Root Cause" node is derived from the AI diagnosis rather than a real
        // lifecycle state, so it lights up the moment Gemini's result lands (and stays lit),
        // and pulses while the AI is still investigating.
        if (step.key === 'root_cause_identified') {
          if (incident && (incident.probableCause || incident.investigationStatus === 'completed')) {
            status = 'complete'
          } else if (incident?.state === 'investigating') {
            status = 'current'
          } else {
            status = 'pending'
          }
        }

        const label = isTerminal && failed ? 'Failed' : step.label
        const time = incident ? formatTimeUTC(step.getTs(incident)) : undefined

        const marker = status === 'complete' ? '✓' : status === 'failed' ? '✕' : String(idx + 1)

        return (
          <li
            key={step.key}
            className={`lifecycle-step lifecycle-${status}`}
            aria-current={status === 'current' ? 'step' : undefined}
          >
            <span className="lifecycle-node" aria-hidden="true">
              {marker}
            </span>
            <span className="lifecycle-label">{label}</span>
            {time ? <span className="lifecycle-time">{time} UTC</span> : null}
          </li>
        )
      })}
    </ol>
  )
}
