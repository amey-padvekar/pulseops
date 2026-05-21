# PulseOps AI Safety and Guardrails

Purpose: define non-negotiable execution boundaries for remediation actions.

## Core Principle

AI recommends actions. The system executes only approved, whitelisted action IDs.

## Allowed Action Pattern

- AI output must contain `actionId` references.
- Backend maps `actionId` to platform-specific command templates.
- Endpoint executes only mapped actions from a signed/validated command envelope.

## Prohibited Pattern

- No direct execution of raw model-generated shell commands.
- No arbitrary script download and execution.
- No command execution without explicit incident context and approval state.

## Minimum Whitelist (MVP)

- `restart_service`
- `flush_dns`
- `check_connectivity`

Each action must define:
- required parameters
- OS-specific implementation path
- expected output signals
- failure behavior

## Approval Policy

- Any remediation action requires explicit user approval in dashboard.
- Approval must record actor and timestamp.
- Approval scope is incident-bound and expires after a short TTL.

## Validation Policy

- Command success is not sufficient to resolve incident.
- Resolution requires fresh telemetry proving recovery.
- If validation fails, incident returns to non-resolved state.

## Auditability

- Log action plan, approver, execution start/end, output summary, and validation result.
- Preserve incident timeline for demo and post-incident analysis.

## Failsafe Behavior

- On endpoint command failure: mark incident execution failed, do not auto-retry unsafe actions.
- On telemetry uncertainty: keep incident in validating/unhealthy state.
- On integration outage: fall back to safe read-only recommendation mode.
