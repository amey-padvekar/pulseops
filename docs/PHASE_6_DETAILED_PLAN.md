# PulseOps AI Phase 6 Detailed Plan

Phase: 6 - Agent Builder operational context handoff  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Package incident context into a structured backend request and hand it off to Google Cloud Agent Builder so later phases can perform investigation using Elastic-backed operational data.

Phase 5 made telemetry, incident history, and log context queryable in Elastic. Phase 6 is the bridge between backend operations and the Agent Builder orchestration layer.

At the end of Phase 6:
- backend can build a complete `AgentBuilderRequest` for an active incident
- the request includes telemetry, incident, device, and log context needed for investigation
- backend calls an Agent Builder adaptor/client rather than doing RCA locally
- request and response IDs are traceable in logs
- timeout, retry, and failure behavior are explicit and demo-safe
- a request payload can be logged or inspected for debugging without exposing secrets

---

## 2) Rule-aware constraints for Phase 6

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Stack compliance
- Agent Builder must be the orchestration layer.
- Gemini remains the reasoning engine in later phases, but Phase 6 is the backend handoff into that workflow.
- Do not introduce competing AI/orchestration services.

2. Elastic track meaning
- The Agent Builder handoff must materially include Elastic-backed operational context.
- Do not send empty or cosmetic context just to claim integration.

3. Functional submission requirement
- The handoff path must be runnable and testable, even if the actual cloud workflow is stubbed or fallback-wrapped during local development.
- Request building must be deterministic and inspectable.

4. Demo readiness
- Keep payload size bounded and intentional.
- Use trace IDs and structured logs so failures are explainable live.

5. Safety and architecture rule
- Backend does not perform final RCA reasoning directly.
- Backend prepares context, calls Agent Builder, and records the result.

---

## 3) Phase 6 definition of done

Phase 6 is complete only when all are true:

1. Backend can build a structured incident investigation request from current backend state.
2. Request includes current telemetry snapshot, incident metadata, device identity, affected service, and recent log summary.
3. Request includes enough identifiers to correlate backend logs, Elastic data, and Agent Builder activity.
4. Backend calls an Agent Builder adaptor/client instead of embedding reasoning logic locally.
5. Timeout and retry behavior are defined and implemented.
6. Request/response IDs are logged for debugging.
7. Sensitive config stays in environment variables, not committed files.
8. At least one local debug path proves request generation and handoff invocation.
9. Phase 6 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Agent Builder request contract

Goal: define the exact structured payload the backend sends into the Agent Builder workflow.

Tasks:
1. Create `backend/internal/agentbuilder/model.go`.
2. Define `AgentBuilderRequest` with fields such as:
- `requestId`
- `incidentId`
- `deviceId`
- `serviceName`
- `incidentState`
- `severity`
- `requestedAt`
- `timeWindow`
- `telemetrySnapshot`
- `recentLogs`
- `incidentSummary`
- `availableActions`
- `elasticContextHints`
3. Define nested DTOs where needed:
- `TimeWindow`
- `TelemetrySnapshot`
- `ElasticContextHints`
- `ActionOption`
4. Keep the shape explicit and JSON-serializable.
5. Add schema version field if useful for later compatibility.

Output:
- stable request model for investigation handoff.

---

### 4.2 Agent Builder response envelope (Phase 6 baseline)

Goal: define the minimum response shape needed before full RCA logic is implemented in Phase 7.

Tasks:
1. In `backend/internal/agentbuilder/model.go`, define `AgentBuilderResponse` with at least:
- `requestId`
- `traceId`
- `status`
- `receivedAt`
- optional `rawPayload`
2. Keep the model broad enough to absorb Phase 7 outputs later without breaking the contract.
3. Distinguish between:
- transport success/failure
- workflow accepted/running/completed states

Output:
- backend can parse and log a structured response even before full AI reasoning lands.

---

### 4.3 Context packer in backend

Goal: build the Agent Builder request from existing backend state and Elastic-ready context.

Tasks:
1. Create `backend/internal/agentbuilder/packer.go`.
2. Implement `BuildRequest(...) (AgentBuilderRequest, error)`.
3. Pull source data from:
- incident store
- device state store
- recent indexed/logged context if available
4. Include these context elements:
- current telemetry snapshot
- device identifier
- incident metadata
- affected service
- recent logs summary
- incident time window
- remediation catalog / action options
5. Bound payload size:
- logs limited to last N entries
- recent telemetry summarized rather than full event flood
6. Provide a concise incident summary string for debugging and human visibility.

Output:
- backend can deterministically assemble the context package for a single incident.

---

### 4.4 Elastic context hints for later MCP usage

Goal: prepare the request so Agent Builder can efficiently ask Elastic MCP for the right data.

Tasks:
1. Add `ElasticContextHints` to the request model.
2. Include fields such as:
- `deviceId`
- `incidentId`
- `serviceName`
- `timeRangeStart`
- `timeRangeEnd`
- `indexPatterns`
- `recommendedQueries`
3. Recommended queries should be short, explainable hints, for example:
- telemetry for device in incident window
- incident events for current incident id
- endpoint logs for device/service in incident window
4. Do not execute MCP here; only pass the context needed for later retrieval.

Output:
- request is ready for credible Elastic MCP-driven retrieval in Phase 7.

---

### 4.5 Remediation catalog handoff

Goal: ensure Agent Builder sees allowed remediation options without permitting arbitrary command generation.

Tasks:
1. Include `availableActions` in the request payload.
2. For MVP, pass stable action IDs only, for example:
- `restart_service`
- `flush_dns`
- `reconnect_vpn`
3. Include optional metadata:
- `target`
- `description`
- `allowedPlatforms`
4. Keep raw shell commands out of the payload.

Output:
- Agent Builder receives the allowed action vocabulary that later recommendation steps must honor.

---

### 4.6 Agent Builder adaptor/client

Goal: isolate outbound Agent Builder communication behind a narrow backend abstraction.

Tasks:
1. Create `backend/internal/agentbuilder/client.go`.
2. Implement an interface such as:
- `SubmitInvestigation(ctx context.Context, req AgentBuilderRequest) (AgentBuilderResponse, error)`
3. Provide a concrete implementation that can:
- send HTTP request to configured endpoint
- add auth header or token if configured
- parse structured response
4. Keep the rest of the backend dependent on the interface, not the transport.
5. Provide a local stub/fake implementation for tests and offline development.

Output:
- backend can call Agent Builder through a clean abstraction.

---

### 4.7 Configuration and secrets handling

Goal: make the integration configurable without leaking secrets into source control.

Tasks:
1. Create `backend/internal/agentbuilder/config.go`.
2. Read environment variables such as:
- `AGENT_BUILDER_ENDPOINT`
- `AGENT_BUILDER_AUTH`
- `AGENT_BUILDER_TIMEOUT_MS`
- optional `AGENT_BUILDER_ENABLED`
3. Validate required fields when enabled.
4. Add defaults that support local testing where safe.
5. Update `backend/.env.example` with placeholders only.

Output:
- secure, explicit runtime configuration for Agent Builder handoff.

---

### 4.8 Retry, timeout, and failure handling

Goal: make handoff robust enough for demo conditions without hiding failures.

Tasks:
1. Add request timeout handling in the client.
2. Add limited retry behavior for transient failures only.
3. Keep retry count small and deterministic (for example 1-2 retries max).
4. Log failure reason and trace ID when available.
5. If handoff fails:
- backend should record failure
- incident should remain actionable later
- failure should not crash telemetry or incident processing loop
6. Do not silently swallow all failures.

Output:
- backend handoff is resilient and debuggable under transient issues.

---

### 4.9 Request tracing and structured logging

Goal: correlate incident, backend, Elastic, and Agent Builder activity during debugging and the demo.

Tasks:
1. Generate a `requestId` per Agent Builder invocation.
2. Log at minimum:
- `request_id`
- `incident_id`
- `device_id`
- `service_name`
- `agent_builder_endpoint`
- `status`
- `trace_id` if returned
3. Add request payload logging in a safe form:
- log a redacted or summarized version
- avoid dumping secrets or excessive raw context
4. Preserve logs in a format that is easy to read during smoke-checks.

Output:
- every handoff can be traced end-to-end during tests and demos.

---

### 4.10 Backend orchestration wiring

Goal: connect incident lifecycle to Agent Builder request generation without prematurely adding Phase 7 reasoning output logic.

Tasks:
1. Decide invocation trigger for MVP Phase 6:
- on transition to `investigating`
- or on explicit internal call once incident is active
2. Update `backend/cmd/server/main.go` to initialize Agent Builder config and client.
3. On invocation trigger:
- build request
- submit to client
- log request/response
4. For Phase 6, it is acceptable to store only request/transport metadata and not final RCA output yet.

Output:
- active incident can be handed off into Agent Builder workflow in a structured way.

---

### 4.11 Tests and local verification path

Goal: ensure the contract and handoff behavior are stable before adding full AI output parsing in Phase 7.

Tasks:
1. Add tests for request packer:
- required fields present
- time window populated
- logs bounded
- action catalog included
2. Add tests for config validation.
3. Add tests for client timeout/retry behavior using a fake server or stub.
4. Add tests for redacted request logging if implemented.
5. Add a local verification path that prints or stores a sample request payload for an active incident.

Output:
- confidence that the handoff path is correct even before live cloud execution is fully exercised.

---

### 4.12 Phase 6 smoke-check path

Goal: produce repeatable evidence that backend can build and submit structured incident context into the Agent Builder layer.

Manual/Script sequence:
1. Start backend and agent.
2. Trigger a stopped-service incident.
3. Wait for incident creation.
4. Trigger the Agent Builder handoff path.
5. Verify:
- structured request payload exists
- request includes incident id, device id, service name, time window, telemetry snapshot, recent logs, and action catalog
- request/response IDs are logged
- retry/timeout behavior is visible if endpoint is unavailable
6. Capture evidence under `artifacts/phase6-smoke/<timestamp>/`:
- backend logs
- request payload snapshot (redacted)
- response snapshot or failure log

Output:
- repeatable proof that the backend can package and submit operational context into Agent Builder workflow.

---

## 5) Detailed implementation order

Build in this order to keep the backend runnable and testable:

1. `backend/internal/agentbuilder/model.go`
2. `backend/internal/agentbuilder/config.go`
3. `backend/internal/agentbuilder/packer.go`
4. `backend/internal/agentbuilder/client.go`
5. Add fake/stub client for tests
6. Add timeout and retry behavior
7. Wire client initialization in `backend/cmd/server/main.go`
8. Connect incident trigger to request build + submit path
9. Add structured logging and request correlation
10. Add tests for packer/config/client
11. Add Phase 6 smoke-check path and evidence capture

Why this order:
- request model first prevents drift across packer/client/logging
- packer before transport ensures the hard part is testable in isolation
- transport and retries come next once payloads are stable
- main wiring and smoke-check close the loop with minimal churn

---

## 6) Key implementation details and pitfalls

1. Backend is not the reasoner
- do not sneak RCA rules into the handoff layer.
- backend prepares context and delegates.

2. Bound context size
- uncontrolled payloads will hurt latency and demo reliability.
- summarize logs and keep telemetry snapshot compact.

3. Preserve traceability
- every request must tie back to one incident and one device.

4. Keep action vocabulary safe
- action IDs only, not arbitrary shell commands.

5. Make local development possible
- stubbed or fake Agent Builder client is acceptable for local verification as long as the real integration boundary is preserved.

6. Keep Elastic meaningful in the request
- include Elastic query hints/index patterns so the later MCP story is operationally credible.

---

## 7) Environment variables reference (Phase 6)

Backend variables for Agent Builder handoff:

- `AGENT_BUILDER_ENDPOINT`
- `AGENT_BUILDER_AUTH`
- `AGENT_BUILDER_TIMEOUT_MS`
- optional: `AGENT_BUILDER_ENABLED`

These must remain environment-based and out of committed secrets.

---

## 8) New files to create

- `backend/internal/agentbuilder/model.go`
- `backend/internal/agentbuilder/config.go`
- `backend/internal/agentbuilder/packer.go`
- `backend/internal/agentbuilder/client.go`
- `backend/internal/agentbuilder/*_test.go`

## 9) Files to modify

- `backend/cmd/server/main.go`
- `backend/.env.example`
- optionally `scripts/smoke-check.ps1` or a Phase 6 companion smoke script
- later incident storage files if request metadata is persisted

---

## 10) Phase 6 acceptance gate

Pass when all three are true (from [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md)):

- [ ] Backend sends structured incident context request.
- [ ] Request/response IDs are traceable for debugging.
- [ ] Retries and timeout behavior are documented.
