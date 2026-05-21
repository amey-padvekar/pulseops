# PulseOps AI Phase 2 Detailed Plan

Phase: 2 - Endpoint agent: heartbeat and telemetry collection  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Build the lightweight Go endpoint agent that reports device health, service status, and telemetry to the backend on a predictable interval.

Phase 2 is about proving that the endpoint is observable and that the backend can receive a structured heartbeat stream. This is the foundation for incident detection in Phase 4 and the later Agent Builder + Elastic workflow.

At the end of Phase 2, the agent should:
- load its config cleanly
- start a heartbeat loop
- gather service/process status
- gather basic system telemetry
- gather recent logs or a simulated log feed
- POST telemetry to the backend at a steady interval
- remain reachable while the monitored service is intentionally failing

---

## 2) Rule-aware constraints for Phase 2

These constraints from docs/rules.md must shape the implementation:

1. Required stack alignment
- Keep the implementation compatible with Gemini, Google Cloud Agent Builder, and Elastic MCP later.
- Do not introduce prohibited competing AI/cloud services.

2. Functional submission requirement
- The agent must be runnable locally and produce visible telemetry.
- Phase 2 should end with a reproducible startup and verification path.

3. New project requirement
- Keep all code original to this repository.
- Avoid importing prior work or hidden dependencies.

4. License and repository readiness
- Preserve the ability to publish the repo publicly with an OSI-approved license.
- Keep docs and runtime steps judge-readable.

5. Demo readiness
- The agent must support a deterministic failure scenario so later demo phases are repeatable.

---

## 3) Phase 2 definition of done

Phase 2 is complete only when all are true:

1. Agent config loads from environment variables without panic.
2. Agent sends heartbeat telemetry at the configured interval.
3. Telemetry includes the required minimum fields.
4. Service/process health is measured and reflected in telemetry.
5. Network reachability is checked and reflected in telemetry.
6. Recent logs or a simulated log feed are included.
7. Backend receives and logs telemetry successfully.
8. Endpoint stays reachable while the monitored service is stopped.
9. A local smoke-check path proves build and run behavior.
10. The acceptance criteria for Phase 2 are satisfied in docs/PHASE_ACCEPTANCE_CRITERIA.md.

---

## 4) Work breakdown structure

### 4.1 Config loader and runtime contract

Goal: make the agent start from environment variables with predictable defaults.

Tasks:
1. Define the agent runtime config model.
2. Read and validate these minimum variables:
- `APP_ENV`
- `AGENT_DEVICE_ID`
- `AGENT_HEARTBEAT_INTERVAL_SEC`
- `MONITORED_SERVICE_NAME`
- `BACKEND_BASE_URL`
- `AGENT_REQUEST_TIMEOUT_MS`
- `ENABLE_SIMULATED_LOGS`
- `NETWORK_CHECK_HOST`
3. Apply safe defaults for local development where appropriate.
4. Fail fast only for truly required values.
5. Keep secrets out of the agent config path unless absolutely necessary later.

Output:
- a repeatable runtime contract that supports local development and future deployment.

### 4.2 Heartbeat loop and telemetry emission

Goal: send a stable heartbeat payload on a fixed interval.

Tasks:
1. Create a ticker-driven loop that runs every 5 to 10 seconds.
2. Build a telemetry payload on each tick.
3. Include the heartbeat flag and current timestamp.
4. Send telemetry to the backend over HTTP POST or an agreed stream transport.
5. Add retry behavior with simple backoff for transient failures.
6. Log delivery success and failures clearly.

Output:
- a steady telemetry stream that the backend can ingest and use later for incident detection.

### 4.3 Service/process health and platform abstraction

Goal: inspect the monitored service without hard-wiring the code to a single OS path.

Tasks:
1. Define interfaces for platform-specific behaviors.
- `ServiceChecker`
- `LogCollector`
- `CommandExecutor`
2. Implement a simple service status check for the demo OS.
3. Return `running`, `stopped`, `degraded`, or `unknown` as appropriate.
4. Keep the interface boundaries clean so Linux support can be added later.
5. Record the service state in every telemetry payload.

Output:
- platform-aware health inspection that supports the demo OS and future expansion.

### 4.4 Network reachability and basic system telemetry

Goal: capture a minimal but useful operational snapshot.

Tasks:
1. Implement a lightweight network reachability check.
2. Capture basic CPU and memory usage.
3. Keep the measurements coarse and inexpensive.
4. Ensure the checks do not block the heartbeat loop excessively.
5. Reflect the health values in telemetry fields.

Output:
- a small, reliable telemetry slice that provides enough context for downstream incident logic.

### 4.5 Recent logs or simulated log feed

Goal: include a short operational context trail in the telemetry stream.

Tasks:
1. Implement recent log collection if available on the demo platform.
2. Otherwise generate a deterministic simulated log feed for the MVP.
3. Keep the log buffer short and bounded.
4. Ensure log collection failures do not break heartbeat delivery.
5. Normalize log strings so they can later be indexed in Elastic.

Output:
- recent log context that supports later incident investigation and Elastic ingestion.

### 4.6 Backend telemetry submission

Goal: deliver telemetry to the backend in a predictable, inspectable format.

Tasks:
1. POST JSON telemetry to the backend ingestion endpoint.
2. Include headers or metadata that help identify the device and request.
3. Handle non-200 responses with clear logs.
4. Add timeouts so the agent remains responsive.
5. Surface backend connectivity failures without terminating the process.

Output:
- backend-visible telemetry that can be verified from logs and later consumed by the UI.

### 4.7 Smoke test and demo failure trigger

Goal: make the agent easy to verify and easy to break on purpose.

Tasks:
1. Provide a local run command that starts the agent.
2. Provide a smoke-check path that confirms the agent can start and emit telemetry.
3. Document the manual service-stop action used in demos.
4. Ensure the agent keeps heartbeating when the monitored service is stopped.
5. Capture a repeatable log or screen state for demo rehearsal.

Output:
- a controlled failure scenario that proves the agent can observe the problem before the remediation phases begin.

---

## 5) Detailed implementation order

Recommended sequence:

1. Wire the config loader and runtime defaults.
2. Build the telemetry payload model.
3. Implement the heartbeat loop.
4. Add service/process status checks.
5. Add network reachability and system metrics.
6. Add recent log capture or simulation.
7. Implement backend POST submission.
8. Add logs, retries, and timeout handling.
9. Verify the local smoke path.
10. Rehearse the service-stop failure scenario.

Why this order:
- the agent needs a stable config base before any telemetry logic
- the heartbeat loop is the core control path
- the service check and logs make the payload useful
- backend submission and smoke testing prove the phase is actually runnable

---

## 6) Deliverables and evidence artifacts

Deliverables to produce in this phase:

1. Runnable agent binary
- the Go agent starts and remains active

2. Telemetry payloads
- structured JSON with the required minimum fields

3. Backend visibility
- telemetry appears in backend logs or ingestion output

4. Deterministic local verification
- smoke-check or runbook path that proves the heartbeat loop works

5. Demo scenario support
- manual service stop does not stop the agent itself

Evidence to capture for compliance matrix:
- terminal output showing agent startup
- terminal output showing telemetry delivery
- screenshot or log snippet of backend receipt
- proof that the monitored service can fail while the agent remains alive

---

## 7) Acceptance gates mapped to existing docs

Use these pass/fail gates before moving to Phase 3.

Gate A: Heartbeat delivery
- agent sends telemetry on the configured interval
- timestamps update as expected

Gate B: Payload completeness
- telemetry includes device ID, timestamp, heartbeat, service name, service status, network reachability, CPU, memory, and logs

Gate C: Backend ingestion
- backend receives and logs telemetry without manual intervention

Gate D: Failure resilience
- stopping the monitored service does not stop the agent process
- telemetry continues to flow after the service stop

Gate E: Platform consistency
- the same demo OS path used in documentation matches the tested agent behavior

These gates align with docs/PHASE_ACCEPTANCE_CRITERIA.md and docs/COMPLIANCE_EVIDENCE_MATRIX.md.

---

## 8) Risks and mitigations in Phase 2

Risk 1: Platform-specific service checks are brittle
- Mitigation: keep the OS-specific logic behind interfaces and start with one demo OS only.

Risk 2: Telemetry payload grows too large
- Mitigation: keep logs short and fields bounded.

Risk 3: Backend connectivity errors interrupt the loop
- Mitigation: use timeouts, retries, and non-fatal logging.

Risk 4: Network checks slow the heartbeat loop
- Mitigation: keep them lightweight and on a bounded timeout.

Risk 5: Demo failure is not repeatable
- Mitigation: document one deterministic service-stop path and rehearse it.

---

## 9) Suggested time-box plan (single day)

Block 1 (60 to 90 min)
- config loader, payload model, and telemetry types

Block 2 (60 to 90 min)
- heartbeat loop and backend POST path

Block 3 (45 to 60 min)
- service check, network reachability, and log capture/simulation

Block 4 (45 to 60 min)
- smoke test, demo failure rehearsal, docs update

Total expected Phase 2 effort:
- about 4 to 5 focused hours

---

## 10) Exit checklist

- [ ] Agent config loads from environment cleanly
- [ ] Heartbeat loop emits telemetry at the configured interval
- [ ] Minimum telemetry fields are present
- [ ] Service status is captured
- [ ] Network reachability is captured
- [ ] Logs or simulated logs are included
- [ ] Backend receives telemetry successfully
- [ ] Agent remains alive when monitored service stops
- [ ] Smoke-check or local verification path exists
- [ ] Phase 2 acceptance criteria are satisfied

---

## 11) Guardrail reminder for next phases

Phase 2 should stay focused on observation, not incident logic.

Do not add incident detection, AI reasoning, approval flows, or remediation execution yet. Those belong in later phases and will depend on the telemetry contract established here.
