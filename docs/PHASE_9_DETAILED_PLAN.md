# PulseOps AI Phase 9 Detailed Plan

Phase: 9 - Endpoint remediation execution  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Execute approved remediation actions on the endpoint safely and report the outcome back to the backend.

Phase 8 established the human approval gate and produced a queued remediation command payload. Phase 9 turns that approved intent into real endpoint action: the backend dispatches a trusted remediation command, the Go agent executes only whitelisted actions, and execution results become visible to operators in the dashboard.

At the end of Phase 9:
- backend can dispatch an approved remediation command to the correct device
- agent can receive and validate the remediation command
- agent maps approved action IDs to platform-specific operations
- agent captures execution outcome, including logs and status
- backend stores and broadcasts execution results to the dashboard

---

## 2) Rule-aware constraints for Phase 9

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Stack compliance
- Gemini and Agent Builder remain responsible for diagnosis and recommendation, not direct endpoint execution.
- Endpoint execution stays inside the Go agent and backend control plane.
- Do not introduce competing orchestration or remote-execution services.

2. Safety constraints
- Only approved, whitelisted action IDs may execute.
- Raw shell commands from model output or dashboard input must never execute.
- The agent must map action IDs to known platform-specific implementations.
- Execution scope should remain intentionally small for the MVP.

3. Functional submission requirement
- The remediation path must perform a real task beyond answering questions.
- The flow shown in the dashboard must reflect actual backend and agent behavior.
- Execution result reporting must be reliable enough to demonstrate live in the web UI.

4. Demo readiness
- Choose deterministic remediation actions with low blast radius.
- Provide enough execution logging to explain what happened if the command fails.
- Keep transport and retry behavior simple and observable.

5. Phase boundary
- Phase 9 covers dispatch, execution, and execution-result reporting.
- Phase 10 will decide whether remediation actually restored health.
- Phase 9 should not mark incidents resolved based only on command success.

---

## 3) Phase 9 definition of done

Phase 9 is complete only when all are true:

1. Backend sends a remediation command only for an approved incident.
2. Agent receives remediation command intended for its device.
3. Agent rejects unknown or unapproved action IDs.
4. Agent executes only whitelisted actions.
5. Agent captures execution status and relevant stdout/stderr or structured result details.
6. Agent reports execution result back to backend.
7. Backend stores execution result on the incident.
8. Dashboard shows execution status and logs for the incident.
9. Phase 9 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Remediation command contract

Goal: define one durable payload format shared between backend and agent.

Suggested command shape:

```json
{
  "incidentId": "INC-1001",
  "deviceId": "DEV-AGENT-01",
  "approvedBy": "demo.operator",
  "approvedAt": "2026-05-23T22:10:00Z",
  "actions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService"
    }
  ],
  "dispatchedAt": "2026-05-23T22:10:05Z",
  "requestId": "rem-12345"
}
```

Tasks:
1. Define shared DTOs for remediation command and action items.
2. Carry approval metadata through the command for traceability.
3. Add correlation fields such as `requestId` for debugging.
4. Keep the payload bounded and action-ID-based only.

Output:
- backend and agent share a strict execution contract.

---

### 4.2 Execution result contract

Goal: define the result payload the agent returns after attempting remediation.

Suggested result shape:

```json
{
  "incidentId": "INC-1001",
  "deviceId": "DEV-AGENT-01",
  "requestId": "rem-12345",
  "status": "succeeded",
  "startedAt": "2026-05-23T22:10:06Z",
  "finishedAt": "2026-05-23T22:10:08Z",
  "results": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService",
      "status": "succeeded",
      "stdout": "Service restarted successfully",
      "stderr": ""
    }
  ]
}
```

Tasks:
1. Define top-level execution result DTO.
2. Define per-action result shape.
3. Normalize status values such as:
- `queued`
- `dispatched`
- `running`
- `succeeded`
- `failed`
- `rejected`
4. Keep timestamps in UTC and logs bounded in size.

Output:
- backend can store and render execution results consistently.

---

### 4.3 Backend dispatch mechanism

Goal: deliver approved remediation commands from backend to the correct agent.

Tasks:
1. Choose an MVP delivery mechanism consistent with the current architecture:
- polling endpoint for pending commands
- websocket stream to agent
- long-poll or simple command fetch endpoint
2. Prefer the simplest path that fits current backend/agent code and demo reliability.
3. Ensure backend dispatches only for incidents in `approved` state.
4. Mark command lifecycle progression in backend:
- `queued`
- `dispatched`
- `acknowledged` optional
5. Prevent repeated dispatch of the same command unless retry logic explicitly allows it.

Output:
- backend owns a minimal but reliable command delivery path.

---

### 4.4 Agent command retrieval loop

Goal: let the endpoint agent receive pending remediation commands as part of its normal runtime.

Tasks:
1. Add remediation polling or command subscription loop to the agent runtime.
2. Scope commands by `deviceId` so the agent ignores commands meant for other devices.
3. Add interval/backoff behavior suitable for the demo.
4. Avoid blocking the telemetry heartbeat loop while remediation runs.
5. Log command receipt with incident and request identifiers.

Output:
- the agent can discover pending remediation work without destabilizing telemetry collection.

---

### 4.5 Agent-side whitelist and action mapper

Goal: convert approved action IDs into explicit, bounded platform operations.

Tasks:
1. Define an action whitelist in `agent/internal/remediation`.
2. Implement action handlers for the MVP set, for example:
- `restart_service`
- `flush_dns`
- `reconnect_vpn` only if truly needed and reliable
3. Map each action ID to a concrete platform method rather than arbitrary shell text.
4. Reject unknown actions before execution.
5. Keep Windows-specific and Linux-specific behavior behind interfaces already established in agent platform abstractions.

Output:
- remediation execution is real, but still tightly bounded and auditable.

---

### 4.6 Command executor integration

Goal: reuse or extend the platform execution layer so remediation logic is testable and OS-aware.

Tasks:
1. Reuse `CommandExecutor` abstraction or extend it if Phase 2 introduced a minimal version.
2. Add remediation service methods such as:
- `RestartService(name string)`
- `FlushDNS()`
3. Keep command construction inside trusted agent code.
4. Capture exit code, stdout, stderr, and duration.
5. Normalize failures into structured remediation result data.

Output:
- remediation actions use one controlled execution abstraction instead of scattered OS calls.

---

### 4.7 Backend remediation result ingestion

Goal: receive execution outcomes from the agent and attach them to the incident lifecycle.

Tasks:
1. Add result ingestion endpoint such as:
- `POST /remediation/results`
2. Validate:
- request shape
- incident exists
- device matches incident
- requestId matches known command
3. Update incident state to `executing` when dispatch begins if not already done.
4. Persist execution result details and timestamps on the incident.
5. Decide the post-result state boundary for Phase 9:
- stay in `executing`
- or move to `validating` once command completes successfully
6. Broadcast updated incident state to frontend.

Output:
- backend becomes the source of truth for remediation execution progress and outcome.

---

### 4.8 Incident execution fields and timeline

Goal: store enough execution detail for operators and later summary generation.

Tasks:
1. Extend incident model with execution metadata such as:
- `executionStatus`
- `executionStartedAt`
- `executionFinishedAt`
- `executionResults`
- `executionRequestId`
2. Add timeline/event entries for:
- command queued
- command dispatched
- command started
- command finished
3. Keep approval metadata and execution metadata separate.

Output:
- incident record reflects the execution phase clearly and supports later validation and summary work.

---

### 4.9 Dashboard execution panel

Goal: show remediation progress and outcome clearly in the UI.

Tasks:
1. Add execution status section to the dashboard incident view.
2. Show at minimum:
- current execution state
- approved action list
- start and finish timestamps
- per-action result status
- stdout/stderr snippets or summarized logs
3. Use visual states to distinguish:
- queued
- running
- succeeded
- failed
4. Keep the panel compact enough for live demo narration.

Output:
- judges and operators can see that real remediation executed and what happened.

---

### 4.10 Retry and duplicate-execution safeguards

Goal: avoid accidental repeated execution while still allowing controlled recovery from transient delivery issues.

Tasks:
1. Mark each remediation command with a stable request ID.
2. Ensure the agent ignores a command already completed for the same request ID.
3. Decide whether backend retry is allowed for:
- not yet acknowledged commands
- agent timeout during retrieval
4. Do not retry automatically after a confirmed failed action unless explicitly designed.
5. Log duplicate detection and retry decisions.

Output:
- the execution path is safe from naive duplicate runs.

---

### 4.11 Tests for execution workflow

Goal: validate the control plane before Phase 10 adds health-based recovery rules.

Tasks:
1. Backend tests for:
- dispatch only after approval
- command retrieval scoped by device
- result ingestion success path
- invalid request ID rejection
- duplicate result handling
2. Agent tests for:
- whitelist enforcement
- unknown action rejection
- action mapper behavior
- structured result generation
3. Integration-style tests where feasible for command contract serialization.

Output:
- the execution slice is covered before recovery validation is layered on top.

---

### 4.12 Manual rehearsal and smoke evidence

Goal: prove that Phase 9 works as a demoable operational action, not just a documented design.

Tasks:
1. Extend rehearsal flow to include:
- approved incident exists
- backend dispatches command
- agent executes approved action
- backend receives result
- dashboard shows execution logs
2. Capture logs or artifact snapshots under `artifacts/`.
3. Keep the action list intentionally small so the demo path is reliable.

Output:
- Phase 9 has a repeatable proof path for rehearsal and submission evidence.

---

## 5) Recommended implementation order

Implement Phase 9 in this order:

1. Define remediation command and result DTOs.
2. Add backend queue/dispatch state and result-ingestion endpoints.
3. Build agent command retrieval loop.
4. Implement whitelist-based action mapper and executor integration.
5. Add incident execution metadata storage.
6. Render execution state in the dashboard.
7. Add tests for backend dispatch/result paths and agent whitelist enforcement.
8. Run manual remediation rehearsal with one approved incident.

This order stabilizes the protocol and safety boundaries before UI polish and avoids debugging transport and execution at the same time as data-model churn.

---

## 6) File-by-file implementation map

Expected agent touch points:
- `agent/internal/remediation/` for whitelist, action mapper, and execution orchestration
- `agent/internal/platform/` for OS-specific execution helpers and service control
- `agent/cmd/agent/main.go` for remediation polling/subscription loop wiring
- `agent/internal/config/` for remediation polling interval or backend endpoint config if needed

Expected backend touch points:
- `backend/internal/remediation/` for command queue/dispatch models and result ingestion logic
- `backend/internal/incidents/` or `backend/internal/store/` for execution metadata persistence
- `backend/internal/api/` for agent command fetch and remediation result endpoints
- `backend/cmd/server/main.go` for route wiring and orchestration hooks

Expected frontend touch points:
- `frontend/src/types/` for remediation execution DTOs
- `frontend/src/pages/DashboardPage.tsx` for execution panel rendering
- `frontend/src/components/` for execution log/status card extraction if helpful
- `frontend/src/App.css` for execution-state styling

Expected docs/scripts touch points:
- `scripts/smoke-check.ps1` or a follow-up demo script to exercise approval plus execution
- runbook docs if the proof path needs explicit rehearsal steps

---

## 7) Pitfalls to avoid

1. Do not let execution depend on raw AI output.
- Only use the approved, backend-derived remediation payload.

2. Do not mix execution success with recovery success.
- A service restart command can succeed even if the endpoint is still unhealthy; Phase 10 handles that distinction.

3. Do not overbuild the transport.
- Pick the simplest reliable mechanism that fits the current MVP rather than inventing a full job system.

4. Do not make the agent trust the backend blindly.
- The agent should still enforce the whitelist and reject unsafe actions.

5. Do not dump unbounded logs into the UI.
- Truncate or summarize stdout/stderr so the dashboard remains readable.

6. Do not ignore duplicate-delivery behavior.
- Without request IDs and idempotency rules, repeated commands can create confusing demo failures.

---

## 8) Acceptance gate for moving to Phase 10

Do not start Phase 10 until all checks below are true:

1. Only approved incidents can produce executable remediation commands.
2. Agent executes only whitelisted action IDs.
3. Agent reports remediation result back to backend with request correlation.
4. Backend stores execution logs/results on the incident.
5. Dashboard shows execution progress and final command outcome.
6. Duplicate or invalid execution paths are handled predictably.
7. The Phase 9 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are demonstrably satisfied.

---

## 9) Outcome of this phase

When Phase 9 is complete, PulseOps AI will no longer stop at recommendation and approval. It will perform a real, bounded endpoint remediation action through the Go agent, then report the execution outcome back into the incident flow. That is the key operational step that turns the system from an advisory assistant into a functional agent while still preserving the human approval boundary established in Phase 8.