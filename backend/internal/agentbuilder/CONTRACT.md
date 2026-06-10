# Agent Builder: Investigation Task Contract (Phase 7)

Purpose
- Define the exact investigation task contract that Agent Builder workflows (Gemini) must follow for Phase 7.

Contract summary
- Input (from backend caller):
  - `incidentId` (string)
  - `deviceId` (string)
  - `requestId` (string)
  - `service` (optional string)
  - `timeWindow` (ISO timestamps or minutes before/after detection)
  - `evidence` (compact summary and/or structured list retrieved from Elastic MCP)
  - `metadata` object for ADK traceability:
    - `incident_id`
    - `device_id`
    - `request_id`
    - optional `idempotency_token` for retries

- Evidence must include (when available):
  1. Telemetry events for the device during the incident window
  2. Incident events for the current `incidentId`
  3. Endpoint log snippets for the device/service during the incident window

- Output: JSON matching `InvestigationResult` (see investigation_model.go)
  - `probableCause`: human-readable short cause
  - `confidence`: numeric 0.0-1.0
  - `recommendedActions`: array of `{actionId, target, reason}` where `actionId` is an approved action id
  - `validationSteps`: ordered list of steps an operator can run to verify the diagnosis
  - `summary`: concise operator-facing 1-3 sentence summary

Rules and safety constraints (MUST enforce)
- Reason only from the provided `evidence`. Do not hallucinate or invent data sources.
- Do not return shell commands or implementation-level steps. Only return `actionId` values from the approved catalog.
- Recommended action IDs must be one of the approved values (e.g. `restart_service`, `flush_dns`, `reconnect_vpn`).
- Keep language concise and operator-facing; avoid speculative prose.
- Keep output JSON strictly valid and parseable; follow the `InvestigationResult` schema.

Decision heuristics (MVP)
- If a monitored service is stopped while heartbeat telemetry from the same host is present, consider `probableCause` = "service stopped" (with confidence depending on log evidence), and recommend `restart_service` as an allowed remediation.

Traceability and evidence
- Include in the archived request/response the raw `evidence` that was used to reason. The backend will retain the raw payload for debugging.

Timeouts and fallback
- Workflows must respond within the configured investigation timeout budget. If timed out, return a well-formed JSON indicating `probableCause` empty and an `investigation_status` field in the metadata (backend will synthesize a failure result path).

Notes for implementers
- The backend will provide a compact `evidence` summary (not full logs) to keep prompts short for demo performance. When in doubt, prefer compact, high-value snippets (errors, recent telemetry spikes, missing heartbeats).
