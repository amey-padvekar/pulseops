# Phase 0 Detailed Spec - MVP Scope and Contracts

## Purpose
Lock one deterministic MVP workflow before implementation so all later phases build on fixed contracts and safety boundaries.

## Scope Lock

### Chosen platform
- Implementation language: Go
- Demo OS: Windows only
- Deployment mode for MVP: single endpoint + single backend + single dashboard

### Chosen incident story
- Primary incident type: monitored service unexpectedly stops
- Primary monitored service: OpenVPNService (if present)
- Fallback monitored target: PulseOpsDummyService (if OpenVPNService is unavailable)

### Chosen approval and execution model
- Approval path: manual dashboard approval only
- Remediation model: action IDs only (no raw command text from AI)
- Initial remediation set:
  - restart_service
  - connectivity_recheck (optional post-restart validation)

## Canonical End-to-End Flow
1. Agent emits heartbeat and service telemetry every 5-10 seconds.
2. Backend receives telemetry and updates latest device state.
3. Backend creates incident if detection rule is met.
4. Backend sends investigation context package to Agent Builder.
5. Agent Builder returns probable cause + recommended action IDs.
6. Dashboard displays recommendation and evidence.
7. Human approves remediation.
8. Backend sends whitelisted remediation command to agent.
9. Agent executes mapped OS action and returns result.
10. Backend validates recovery from fresh telemetry.
11. Backend stores/generates incident summary.

## Detection and Incident Rules

### Detection rule (MVP)
Create an incident when all conditions are true:
- heartbeat is present
- serviceName matches monitored target
- serviceStatus == stopped
- condition persists for 2 consecutive telemetry cycles

### Debounce and deduplication
- Do not open multiple active incidents for the same device + service.
- Reuse active incident if failure condition continues.
- Close detection loop only after incident reaches resolved or failed.

### Severity mapping
- stopped monitored service + heartbeat alive -> high
- unknown state or partial telemetry -> medium

## State Machine Definition
Allowed states:
- healthy
- detected
- investigating
- awaiting_approval
- approved
- executing
- validating
- resolved
- failed

### Required transitions
- healthy -> detected
- detected -> investigating
- investigating -> awaiting_approval
- awaiting_approval -> approved
- approved -> executing
- executing -> validating
- validating -> resolved
- validating -> failed

### Transition guards
- awaiting_approval -> approved requires approver identity and timestamp.
- approved -> executing requires action IDs to be in whitelist.
- validating -> resolved requires recovery criteria satisfied for 2 consecutive cycles.

## AI Context Contract
Backend sends a structured context package containing:
- incident metadata
- latest telemetry snapshot
- recent telemetry window
- recent log snippets
- monitored service metadata
- remediation catalog (allowed action IDs)

### Non-goal
- Backend does not perform final root-cause reasoning.
- AI does not produce executable shell commands.

## JSON Contracts (MVP)

### telemetry.event
```json
{
  "deviceId": "LAPTOP-22",
  "timestamp": "2026-05-20T10:30:00Z",
  "heartbeat": true,
  "serviceName": "OpenVPNService",
  "serviceStatus": "running",
  "networkReachable": true,
  "cpuUsage": 12,
  "memoryUsage": 48,
  "recentLogs": [
    "Service heartbeat OK"
  ]
}
```

### incident.record
```json
{
  "incidentId": "INC-1001",
  "deviceId": "LAPTOP-22",
  "serviceName": "OpenVPNService",
  "state": "detected",
  "severity": "high",
  "detectedAt": "2026-05-20T10:31:05Z",
  "lastUpdatedAt": "2026-05-20T10:31:10Z",
  "evidence": {
    "heartbeat": true,
    "serviceStatus": "stopped"
  }
}
```

### ai.recommendation
```json
{
  "incidentId": "INC-1001",
  "probableCause": "OpenVPN service unexpectedly stopped while endpoint remained reachable.",
  "confidence": 0.92,
  "recommendedActions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService",
      "reason": "Service is stopped and restart is lowest-risk recovery action."
    }
  ],
  "validationSteps": [
    "Confirm serviceStatus is running",
    "Confirm heartbeat remains true"
  ],
  "summary": "Service interruption likely caused VPN unavailability; restart is recommended."
}
```

### remediation.command
```json
{
  "incidentId": "INC-1001",
  "deviceId": "LAPTOP-22",
  "approvedBy": "admin@pulseops.local",
  "approvedAt": "2026-05-20T10:32:00Z",
  "actions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService"
    }
  ]
}
```

### remediation.result
```json
{
  "incidentId": "INC-1001",
  "deviceId": "LAPTOP-22",
  "actionId": "restart_service",
  "status": "success",
  "startedAt": "2026-05-20T10:32:05Z",
  "endedAt": "2026-05-20T10:32:09Z",
  "stdout": "Service restarted",
  "stderr": ""
}
```

### incident.summary
```json
{
  "incidentId": "INC-1001",
  "rootCause": "OpenVPN service stopped unexpectedly.",
  "evidence": [
    "Telemetry showed serviceStatus=stopped while heartbeat=true",
    "Post-remediation telemetry returned serviceStatus=running"
  ],
  "actionsTaken": [
    "restart_service"
  ],
  "outcome": "resolved",
  "confidence": 0.9,
  "closedAt": "2026-05-20T10:33:00Z"
}
```

## Remediation Whitelist (Phase 0)
Allowed action IDs and semantics:
- restart_service
  - input: target serviceName
  - behavior: restart service through platform adapter
  - safety: fail if target service is not monitored service
- connectivity_recheck
  - input: none
  - behavior: run agent connectivity probe only
  - safety: read-only, no system mutation except probe execution

Disallowed in MVP:
- arbitrary shell commands
- dynamic script execution from AI output
- any action ID not explicitly whitelisted

## Elastic Data Schema Baseline

### Index naming
- telemetry-events-*
- incident-events-*
- endpoint-logs-*

### Required indexed fields
- timestamp
- deviceId
- incidentId
- serviceName
- serviceStatus
- severity
- state
- actionId

### Event taxonomy
- telemetry_received
- incident_created
- incident_state_changed
- ai_recommendation_received
- remediation_approved
- remediation_executed
- remediation_failed
- recovery_validated
- incident_resolved

## Backend-to-Dashboard Event Model
Use server push (WebSocket or SSE) with typed events:
- device.health.updated
- incident.created
- incident.updated
- recommendation.ready
- remediation.execution.updated
- incident.resolved

Each event envelope should include:
```json
{
  "eventType": "incident.updated",
  "timestamp": "2026-05-20T10:32:10Z",
  "deviceId": "LAPTOP-22",
  "incidentId": "INC-1001",
  "payload": {}
}
```

## Recovery Validation Criteria
An incident can transition to resolved only if:
- serviceStatus == running for 2 consecutive telemetry cycles
- heartbeat == true for same cycles
- optional connectivity_recheck passes (if executed)

Otherwise:
- remain in validating until timeout, then transition to failed

## Phase 0 Acceptance Criteria
Phase 0 is complete when all are true:
- one demo OS is selected and documented
- one primary incident trigger is selected and documented
- remediation whitelist is frozen
- state machine and transition guards are frozen
- JSON contracts are documented and versioned
- Elastic field baseline is documented
- backend-dashboard event envelope is documented
- recovery proof criteria are explicit and testable

## Open Questions to Resolve Immediately
- Final monitored service name on demo machine (OpenVPNService or PulseOpsDummyService)
- Exact telemetry interval (5s or 10s)
- Validation timeout threshold before marking failed
- Identity field format for approver (email, username, or ID)

## Versioning
- Spec version: v1.0
- Status: locked for MVP build
- Change policy: only critical fixes allowed after Phase 1 starts

## Winning Edge Additions (Hackathon)

### Must-have differentiators
- Deterministic outage simulator with one-click trigger and one-click fallback recovery.
- Live incident timeline that shows each state transition with timestamps.
- Evidence-first AI panel that shows telemetry and log lines used for recommendation.
- Safety policy gate with explicit allow/deny reason before remediation execution.
- Before/after metrics bar showing detection latency, approval latency, MTTR, and success rate.

### Trust and governance enhancements
- Require action IDs only; reject free-form command text at API boundary.
- Add dry-run mode for remediation preview before real execution.
- Capture approver identity, reason, and approval timestamp in immutable incident history.
- Emit audit events for recommendation generated, approval granted, command sent, and validation result.

### Reliability and demo resilience
- Provide backup canned telemetry/log bundles to demo full flow if integrations are unstable.
- Add retry policy with capped attempts for Agent Builder and Elastic query calls.
- Add timeout-based fallback summary when AI response is delayed.
- Keep the operational loop functional even if AI path is partially unavailable.

### Product/storytelling polish
- Single-screen command center layout for healthy -> failure -> recovery progression.
- Label each step in plain language: what failed, why, what action, proof of recovery.
- Add final exportable incident summary for judges to copy/download.
- Include confidence + rationale badge next to recommended action.

### Judge scoring alignment
- Innovation: MCP-backed contextual investigation with explainable recommendation.
- Technical depth: end-to-end closed-loop automation with human approval gate.
- Impact: measurable MTTR reduction and controlled remediation safety.
- Completeness: deterministic trigger, validation proof, and final report artifact.
- Presentation: clear timeline narrative and visible risk controls.

### Fast-track 1-day add-ons
- Add outage simulator script and trigger endpoint.
- Add recommendation evidence card sourced from context payload.
- Add metrics banner with detection and resolution timers.
- Add policy check result object to remediation API responses.
- Add incident summary export button in dashboard.
