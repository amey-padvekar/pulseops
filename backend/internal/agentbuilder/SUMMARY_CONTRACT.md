# Agent Builder: Final Summary Task Contract (Phase 11)

Purpose
- Define the exact final-summary task contract that Agent Builder workflows (Gemini) must follow for Phase 11, so summary generation produces a consistent closing artifact instead of freeform prose drift.

Contract summary
- Task name: `phase11_summary`
- Input (from backend caller): a `summary_request` matching `FinalSummaryRequest` (see summary_request.go), plus a rendered `prompt` and trace `metadata`.
  - Incident metadata: `incidentId`, `deviceId`, `serviceName`, `severity`, `state`, `outcome`
  - `outcome` is explicit and terminal: `resolved` | `failed` | `incomplete`
  - Diagnosis (Phase 7): `probableCause`, `confidence`, `diagnosisSummary`
  - Recommendation/approval (Phases 7-8): `recommendedActions`, `approvedActions`, `approvedBy`, `approvalNote`
  - Execution (Phase 9): `remediationStatus`, `executionResults` (bounded; stdout/stderr summarized)
  - Validation (Phase 10): `validationStatus`, `validationReason`, `validationFailureReason`, healthy-cycle counters
  - `evidence`: a small, bounded set of high-value factual snippets (telemetry, logs, timeline)
  - `metadata` object for ADK traceability: `incident_id`, `device_id`, `request_id`, optional `idempotency_token`

- Output: JSON matching `IncidentSummary` (see summary_model.go)
  - `rootCause`: one short factual sentence
  - `evidence`: array of short factual strings (at least one)
  - `actionsTaken`: array of short factual strings; when no remediation executed, a single entry stating that
  - `result`: one short sentence; MUST match the stated `outcome`
  - `operatorSummary` (optional): one-sentence at-a-glance narrative

Rules and safety constraints (MUST enforce)
- Summarize ONLY from the provided evidence and structured record. Do not invent telemetry, logs, causes, or actions.
- Preserve the factual distinction between the diagnosis (what was believed), the actions taken (what was done), and the outcome (what actually happened). Never blur them.
- `result` and `operatorSummary` must reflect `outcome` exactly. If `outcome` is `failed`, do not claim recovery; if `resolved`, state recovery was confirmed.
- Keep wording concise and operator-facing: `rootCause`, `result`, and `operatorSummary` are one short sentence each.
- Avoid speculative language ("might", "possibly", "likely") beyond what the record supports.
- Output must be valid JSON strictly matching the `IncidentSummary` schema; emit JSON only, no surrounding prose.

Evidence discipline
- The backend pre-selects and compacts evidence (steps 4.2 and 4.7). Workflows must not request or assume full raw logs.
- Prefer the stored incident record first; Elastic-backed context is supplementary, used only to enrich evidence when needed.

Reuse and integration
- Reuses the Phase 6/7 ADK request pattern: a rendered prompt (`FinalSummaryPromptTemplate`) plus a structured payload (`ADKSummaryRequestPayload`) built by `BuildSummaryADKRequestPayload`.
- Parsing/validation of the response and fallback behavior are defined in step 4.5 (`ParseFinalSummary`).

Timeouts and fallback
- Workflows must respond within the configured summary timeout budget. On timeout or malformed output, the backend records a summary-generation failure (step 4.5) rather than storing partial prose.

Demo readiness
- Output must be compact enough to render on one dashboard screen and to narrate in a live demo within seconds.
