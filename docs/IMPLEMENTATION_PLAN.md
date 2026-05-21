**phased implementation plan** aligned to your updated architecture, with **Go-centric implementation**, **Agent Builder as orchestration**, and **Elastic MCP as the operational context layer**.

---

# Implementation Plan in Phases

## Overall objective

Build a hackathon MVP that demonstrates this flow:

1. Endpoint agent collects telemetry and logs
2. Backend receives and stores operational context
3. Data is available in Elastic
4. Agent Builder investigates through Elastic MCP
5. Gemini identifies probable cause and proposes remediation
6. Dashboard shows recommendation
7. Human approves
8. Backend sends remediation command to endpoint
9. Endpoint executes fix
10. Recovery is validated
11. Incident summary is generated

---

# Phase 0 — Finalize MVP scope and contracts

## Goal
Lock the project into one excellent workflow before coding.

## Decisions to make
Choose exactly:
- primary incident type
- monitored service/process
- demo OS
- remediation actions
- approval path
- Elastic data schema
- backend-to-dashboard event model

## Recommended choices
- **Primary demo:** stopped service / VPN-like service failure
- **Implementation platform:** Go
- **Demo platform:** implement fully on **Windows or Linux**, not both
- **Fallback if VPN is flaky:** use a monitored dummy service/process
- **Approval:** manual button in dashboard
- **Remediation:** restart service + optional connectivity recheck

## Deliverables
- final architecture diagram
- JSON contracts for telemetry, incident, remediation, summary
- remediation action whitelist
- state machine definition

## Output
You should end Phase 0 with a clear answer to:
- what fails
- how it is detected
- what the AI sees
- what is approved
- what is executed
- how recovery is proven

Detailed Phase 0 spec source of truth: `../specs/phase-0/PHASE_0_DETAILED_SPEC.md`

---

# Phase 1 — Repository and project setup

## Goal
Create the project structure and working developer setup.

## Recommended repo structure
```text
/agent
/backend
/frontend
/shared
/docs
/scripts
```

## Suggested substructure
```text
/agent
  /cmd/agent
  /internal/telemetry
  /internal/health
  /internal/logs
  /internal/remediation
  /internal/platform

/backend
  /cmd/server
  /internal/api
  /internal/ws
  /internal/incidents
  /internal/telemetry
  /internal/remediation
  /internal/elastic
  /internal/agentbuilder
  /internal/store

/frontend
  /src/components
  /src/pages
  /src/hooks
  /src/types

/shared
  telemetry.schema.json
  incident.schema.json
  remediation.schema.json
```

## Tasks
- initialize Go modules for `agent` and `backend`
- create React app
- define shared DTOs/schemas
- define env config structure
- set up local run scripts/docker compose if needed

## Deliverables
- runnable backend skeleton
- runnable frontend skeleton
- runnable Go agent skeleton

Detailed Phase 1 source of truth: `./PHASE_1_DETAILED_PLAN.md`

---

# Phase 2 — Endpoint agent: heartbeat and telemetry collection

## Goal
Build the lightweight Go agent that reports endpoint health.

## Responsibilities
The agent should:
- send periodic heartbeat
- collect service/process status
- collect basic system telemetry
- package recent logs or simulated logs
- send JSON telemetry to backend

## Minimum telemetry fields
```json
{
  "deviceId": "LAPTOP-22",
  "timestamp": "2026-05-19T10:30:00Z",
  "heartbeat": true,
  "serviceName": "OpenVPNService",
  "serviceStatus": "running",
  "networkReachable": true,
  "cpuUsage": 12,
  "memoryUsage": 48,
  "recentLogs": []
}
```

## Tasks
- implement config loader
- implement heartbeat loop
- implement service/process health checker
- implement simple network reachability check
- implement recent log capture or simulated log feed
- POST or stream telemetry to backend

## Design note
Use interfaces so the agent can later support both Windows and Linux:
- `ServiceChecker`
- `LogCollector`
- `CommandExecutor`

## Deliverables
- Go agent that sends telemetry every 5–10 seconds
- telemetry visible in backend logs

Detailed Phase 2 source of truth: `./PHASE_2_DETAILED_PLAN.md`

---

# Phase 3 — Backend telemetry ingestion and live device state

## Goal
Receive agent telemetry and maintain current endpoint state.

## Responsibilities
Backend should:
- expose telemetry ingestion endpoint or WebSocket
- parse/store latest state per device
- expose current health to dashboard
- broadcast updates to frontend

## Tasks
- create telemetry API endpoint
- validate and parse telemetry payloads
- maintain in-memory or Redis-backed latest device state
- implement WebSocket updates to UI
- add API to fetch current device state and latest telemetry

## Suggested first storage strategy
Use in-memory store first for speed:
- `map[deviceId]DeviceState`
- `map[incidentId]Incident`

Add Redis only if necessary.

## Deliverables
- backend receives telemetry
- frontend shows live device state
- healthy state visible in dashboard

---

# Phase 4 — Incident detection and state machine

## Goal
Convert unhealthy telemetry into a managed incident lifecycle.

## Incident states
Use a simple state machine:
- `healthy`
- `detected`
- `investigating`
- `awaiting_approval`
- `approved`
- `executing`
- `validating`
- `resolved`
- `failed`

## Detection rule for MVP
If:
- `serviceStatus == stopped`
and
- endpoint heartbeat is still present

Then:
- create incident

## Tasks
- implement incident creation logic
- deduplicate repeated heartbeats into the same active incident
- assign severity
- track timestamps
- expose incident APIs for frontend
- broadcast incident state changes

## Deliverables
- stopping monitored service creates an incident
- dashboard shows red incident state

---

# Phase 5 — Elastic integration for logs and telemetry

## Goal
Make operational context available to the observability layer.

## Responsibilities
Send logs/telemetry into Elastic so the Agent Builder + Elastic MCP story is real.

## Tasks
- set up Elasticsearch index strategy
- add backend Elastic client
- index telemetry events
- index incident events
- optionally index endpoint log snippets
- create a few search-ready fields:
  - `deviceId`
  - `serviceName`
  - `serviceStatus`
  - `severity`
  - `incidentId`
  - `timestamp`

## Recommended index idea
- `telemetry-events-*`
- `incident-events-*`
- `endpoint-logs-*`

## Important hackathon shortcut
You do not need perfect observability architecture.
You just need enough data indexed so:
- queries return meaningful context
- MCP queries are believable and demonstrable

## Deliverables
- telemetry and incident documents visible in Elastic
- at least a few searchable logs for the failing device

---

# Phase 6 — Agent Builder operational context handoff

## Goal
Forward incident context into the Agent Builder workflow.

## Responsibilities
Backend should prepare the context package sent to Agent Builder.

## Backend should send
- current telemetry snapshot
- device identifier
- incident metadata
- affected service name
- recent logs summary
- incident time window
- remediation catalog / action options

## Tasks
- define `AgentBuilderRequest` model
- build context packer in backend
- add integration client/adaptor
- add retry/error handling
- support request tracing for demo/debugging

## Important architectural rule
Backend does **not** perform final RCA reasoning directly.
It packages operational context and calls the Agent Builder layer.

## Deliverables
- backend can send a structured incident investigation request
- request payload logged for debugging/demo visibility

---

# Phase 7 — Elastic MCP-backed investigation + Gemini reasoning

## Goal
Produce the AI diagnosis and remediation recommendation.

## Responsibilities
Agent Builder should:
- use Elastic MCP tools to fetch operational context
- reason over telemetry/logs/history
- produce probable cause
- recommend remediation
- define validation steps
- produce a concise explanation

## Expected structured output
```json
{
  "probableCause": "VPN connectivity failure likely caused by stopped OpenVPN service.",
  "confidence": 0.92,
  "recommendedActions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService",
      "reason": "Service is stopped while endpoint is reachable."
    },
    {
      "actionId": "flush_dns",
      "target": "device",
      "reason": "Connectivity remediation follow-up."
    }
  ],
  "validationSteps": [
    "Confirm service status is running",
    "Confirm vpnConnected is true"
  ],
  "summary": "The service stopped unexpectedly, causing VPN unavailability. Restarting the service is the safest first remediation."
}
```

## Tasks
- define exact output schema
- parse output in backend
- store AI result on incident
- add timeout/fallback behavior
- add simple confidence display

## Deliverables
- active incident gets AI diagnosis
- dashboard shows cause + recommended remediation

---

# Phase 8 — Human approval workflow

## Goal
Implement the governance/safety layer.

## Responsibilities
The remediation plan must be visible and require approval before execution.

## Tasks
- add dashboard remediation card
- add “Approve Remediation” endpoint
- record approver and approval time
- transition incident to `approved`
- queue remediation command for endpoint

## Important safety rule
AI should recommend **action IDs**, not raw shell commands.

Example:
- `restart_service`
- `flush_dns`
- `reconnect_vpn`

Then the backend/agent maps those to platform-specific commands.

## Deliverables
- user can approve a remediation plan from dashboard
- approval is recorded in incident lifecycle

---

# Phase 9 — Endpoint remediation execution

## Goal
Execute approved remediation safely on the endpoint.

## Responsibilities
The Go agent receives a remediation command and executes a whitelisted action.

## Command model
```json
{
  "incidentId": "INC-1001",
  "deviceId": "LAPTOP-22",
  "actions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService"
    }
  ]
}
```

## Tasks
- implement command channel from backend to agent
- agent receives remediation command
- map action IDs to real OS-specific execution
- collect stdout/stderr/result
- send remediation execution result back to backend

## Safety constraints
- no arbitrary shell execution from model output
- only execute whitelisted actions
- keep action list very small

## Deliverables
- approved remediation restarts monitored service
- execution result shown in dashboard

---

# Phase 10 — Recovery validation and incident closure

## Goal
Prove that the remediation worked.

## Responsibilities
Recovery should be confirmed by new telemetry, not just by command success.

## Validation checks
For MVP:
- service status returns to `running`
- heartbeat remains alive
- optional connectivity check passes

## Tasks
- implement validation state
- require 1–2 healthy telemetry cycles before resolving
- if validation fails, mark incident `failed` or `still_unhealthy`
- optionally trigger second recommendation path later

## Deliverables
- incident transitions from `executing` → `validating` → `resolved`
- dashboard returns to green
- recovery is clearly visible

---

# Phase 11 — Incident summary generation

## Goal
Generate a polished AI-generated final report.

## Responsibilities
Once incident is resolved, Agent Builder/Gemini should produce:
- root cause
- evidence
- actions taken
- final outcome

## Example structure
```text
Incident Summary

Root Cause:
OpenVPN service unexpectedly stopped.

Evidence:
Telemetry showed serviceStatus=stopped while networkReachable=true.
Recent logs showed service termination errors.

Actions Taken:
- Restarted OpenVPN service
- Flushed DNS cache
- Revalidated connectivity

Result:
VPN connectivity restored and service health returned to normal.
```

## Tasks
- create final summary request payload
- store summary with incident
- render summary in dashboard
- optionally make it exportable/copyable

## Deliverables
- final incident report visible in UI
- good closing artifact for demo

---

# Phase 12 — Dashboard polish and demo readiness

## Goal
Turn the functional MVP into a compelling demo.

## Dashboard sections
- live endpoint health card
- telemetry panel
- incident timeline/status
- AI investigation panel
- remediation recommendation panel
- approval button
- remediation execution log
- final summary panel

## Tasks
- add color-coded states
- add real-time status transitions
- add confidence/rationale badge
- show brief logs/telemetry evidence
- make the before/after state visually clear
- reduce noisy data on screen

## Deliverables
- polished, demo-friendly dashboard
- all core flow visible on one screen if possible

---

# Phase 13 — Demo script, rehearsal, and failure-proofing

## Goal
Ensure the demo works reliably under time pressure.

## Demo sequence
1. show healthy endpoint
2. stop monitored service manually
3. show incident detection
4. show AI investigation output
5. show remediation recommendation
6. approve remediation
7. show agent execution
8. show recovery validation
9. show final incident summary

## Tasks
- create a deterministic failure trigger script
- create a deterministic recovery script fallback
- reduce AI payload size for lower latency
- rehearse with timers
- add fallback canned logs if live logs are unreliable
- add retry handling for transient failures

## Deliverables
- repeatable demo under 2–3 minutes
- backup demo path if one integration is flaky

---

# Recommended phase order by build priority

## Must-build first
1. Phase 0 — scope and contracts
2. Phase 1 — repo setup
3. Phase 2 — endpoint telemetry
4. Phase 3 — backend ingestion + live dashboard
5. Phase 4 — incident detection
6. Phase 8 — approval UI skeleton
7. Phase 9 — remediation execution
8. Phase 10 — validation

## Then add AI/observability depth
9. Phase 5 — Elastic integration
10. Phase 6 — Agent Builder handoff
11. Phase 7 — AI reasoning
12. Phase 11 — summary generation

## Final polish
13. Phase 12 — dashboard polish
14. Phase 13 — demo rehearsal

---

# Practical build schedule

## Day 1
- Phase 0
- Phase 1
- Phase 2
- Phase 3

**Goal:** live heartbeat and telemetry visible in dashboard

## Day 2
- Phase 4
- Phase 8
- Phase 9
- Phase 10

**Goal:** detect failure, approve remediation, execute fix, validate recovery

## Day 3
- Phase 5
- Phase 6
- Phase 7
- Phase 11
- Phase 12
- Phase 13

**Goal:** complete Agent Builder + Elastic MCP narrative and polish the demo

---

# MVP success criteria

You are done when this works end-to-end:

- endpoint healthy
- monitored service stopped
- incident auto-detected
- telemetry/logs available in Elastic
- Agent Builder investigates with MCP-backed context
- Gemini returns probable cause + remediation plan
- dashboard shows recommendation
- admin approves
- endpoint executes fix
- health returns
- summary generated

---

# Strong recommendation

Build this in **two layers**:

## Layer 1 — reliable operational loop
- detect
- approve
- execute
- validate

## Layer 2 — hackathon intelligence layer
- Elastic indexing
- MCP context retrieval
- Agent Builder reasoning
- Gemini summary

That way, even if one cloud integration is imperfect, your core demo still works.

If you want, I can next turn this into either:

1. a **task-by-task engineering backlog**
2. a **Go project folder structure with module names**
3. a **phase-wise checklist with exact deliverables**
4. or a **3-day hackathon execution board in Kanban format**