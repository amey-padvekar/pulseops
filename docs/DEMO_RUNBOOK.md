# PulseOps AI Demo Runbook

Goal: deliver a deterministic, under-3-minute demo that proves agentic detect -> investigate -> remediate flow.

## Demo Preconditions

- One endpoint connected and reporting heartbeat.
- Backend and dashboard are running.
- Elastic ingestion is confirmed for recent telemetry/log entries.
- Agent Builder workflow endpoint is reachable.
- Monitored service name is known and validated on demo machine.

## Timing Plan (Target <= 2m45s)

| Segment | Target Time |
|---|---|
| Intro and healthy state | 0:20 |
| Trigger failure | 0:15 |
| Detection and AI investigation | 0:40 |
| Recommendation and approval | 0:30 |
| Remediation execution | 0:25 |
| Recovery + incident summary | 0:35 |

## Primary Script

1. Show healthy dashboard state.
2. Trigger service failure on endpoint.
3. Show incident detection transition.
4. Show AI investigation evidence (telemetry/log context + probable cause).
5. Show recommended actions and approval requirement.
6. Click `Approve Remediation`.
7. Show command execution status.
8. Show validated recovery and final summary.

## Operator Checklist During Demo

- [ ] Narration states Agent Builder is orchestration layer.
- [ ] Narration states Elastic MCP provides operational context.
- [ ] Narration states Gemini performs reasoning and summary generation.
- [ ] Human-in-the-loop approval is visible before execution.
- [ ] End state clearly returns to healthy.

## Fallback Paths

### Fallback A: AI latency or timeout

Use the latest successful cached recommendation and explicitly state:
"This recommendation is from the most recent completed analysis for the same incident signature."

### Fallback B: External integration instability

Switch to pre-seeded telemetry/log scenario while preserving the same workflow transitions in UI.

### Fallback C: Service name mismatch on demo host

Use pre-validated alternate monitored process/service listed in environment notes.

## Hard Stop Conditions

- If endpoint heartbeat drops entirely, restart from healthy baseline.
- If approval action is not visible, do not execute direct remediation.
- If recovery telemetry does not confirm health within defined window, call out validation failure handling and stop.

## Phase 2 Smoke and Failure Drill (4.7)

Use this before recording or rehearsal to prove heartbeat and failure observability.

1. Run smoke-check from repository root:
- `powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1`

Expected result:
- backend `/healthz` is healthy
- backend logs include `telemetry received`
- evidence logs are written under `artifacts\phase2-smoke\<timestamp>\`

2. Start backend and agent in separate terminals:
- `powershell -ExecutionPolicy Bypass -File .\scripts\run-backend.ps1`
- `powershell -ExecutionPolicy Bypass -File .\scripts\run-agent.ps1`

3. Trigger a deterministic service-stop failure (Windows demo path):
- Determine service from `MONITORED_SERVICE_NAME` in `agent\.env`.
- Stop it manually as administrator:
	- `Stop-Service -Name <MONITORED_SERVICE_NAME> -Force`
	- fallback command: `sc.exe stop <MONITORED_SERVICE_NAME>`

4. Verify expected behavior:
- agent process remains running
- backend continues receiving heartbeat telemetry
- service status transitions away from `running` (typically `stopped` or `unknown` depending on host permissions/service type)

5. Capture rehearsal evidence:
- screenshot or clip showing agent terminal still alive after service stop
- snippet from backend logs showing continuing `telemetry received` entries after service stop

## Phase 4 Incident Smoke Path

Use this to generate deterministic incident lifecycle evidence before demo day.

1. Run Phase 4 smoke-check from repository root:
- `powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1 -Phase phase4`

Optional fast local run (skip builds + no frontend process):
- `powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1 -Phase phase4 -SkipBuild -NoFrontend`

Expected result:
- `POST /telemetry` synthetic stopped-service heartbeats are accepted
- `GET /incidents?active=true` returns exactly one active incident
- incident state reaches `investigating`
- repeated failing heartbeat reuses the same active incident (no duplicate)
- evidence is written under `artifacts\phase4-smoke\<timestamp>\` including:
	- `baseline_incidents.json`
	- `incidents_after_first_failure.json`
	- `incidents_after_second_failure.json`
	- `incident_detail.json`
	- `incidents_snapshot.json`
	- backend/agent (and optional frontend) logs
