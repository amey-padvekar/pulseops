# PulseOps AI Phase 7 Detailed Plan

Phase: 7 - Elastic MCP-backed investigation + Gemini reasoning  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Produce a structured AI investigation result for an active incident by combining Agent Builder orchestration, Elastic MCP-backed operational retrieval, and Gemini reasoning.

Phase 6 established the handoff contract from backend to Agent Builder. Phase 7 turns that handoff into a useful investigation result: probable cause, recommended safe remediation actions, validation steps, and a concise operator-facing summary.

At the end of Phase 7:
- Agent Builder uses Elastic MCP to retrieve meaningful telemetry, incident, and log context
- Gemini-compatible reasoning produces a structured investigation result
- backend parses and stores the result on the incident
- dashboard can show probable cause, confidence, recommended actions, and validation steps
- timeout and fallback behavior are predictable enough for a live demo

---

## 2) Rule-aware constraints for Phase 7

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Stack compliance
- Gemini must remain the reasoning engine.
- Google Cloud Agent Builder must remain the orchestration layer.
- Elastic MCP must be used meaningfully for context retrieval, not as a cosmetic dependency.
- Do not introduce competing AI APIs or orchestration tools.

2. Safety constraints
- Recommended actions must be action IDs, not raw shell commands.
- Reasoning output must stay bounded, structured, and easy to validate.
- Backend must not execute recommendations automatically in this phase.

3. Functional submission requirement
- Investigation output must be reproducible from a real incident.
- The result shown in UI must come from actual workflow output or a clearly defined local stub/fallback path.

4. Demo readiness
- Keep prompt/context compact enough to return quickly.
- Favor deterministic formatting over creative variability.
- Provide fallback behavior when cloud/MCP latency or errors occur.

5. Elastic track meaning
- The investigation must clearly depend on operational context fetched via Elastic MCP.
- The system should be able to explain which data was considered when forming the recommendation.

---

## 3) Phase 7 definition of done

Phase 7 is complete only when all are true:

1. Investigation returns a structured result with probable cause.
2. Structured result contains recommended action IDs, not raw shell commands.
3. Structured result contains validation steps.
4. Backend parses investigation output successfully.
5. Parsed investigation result is stored on the incident.
6. Dashboard shows the AI diagnosis and recommended remediation.
7. Timeout and fallback behavior are defined and implemented.
8. At least one active incident can be investigated end-to-end.
9. Phase 7 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Investigation output schema

Goal: define one strict output shape that Agent Builder/Gemini must return.

Tasks:
1. Create `backend/internal/agentbuilder/investigation_model.go`.
2. Define `InvestigationResult` with fields:
- `probableCause`
- `confidence`
- `recommendedActions`
- `validationSteps`
- `summary`
3. Define `RecommendedAction` with fields:
- `actionId`
- `target`
- `reason`
4. Constrain `actionId` values to approved IDs only, for example:
- `restart_service`
- `flush_dns`
- `reconnect_vpn`
5. Keep `confidence` numeric in the range `0.0` to `1.0`.

Output:
- a strict reasoning result contract that backend and frontend can depend on.

---

### 4.2 Agent Builder workflow prompt and contract design

Goal: make the workflow reliably request the right Elastic MCP context and return structured output.

Tasks:
1. Define the investigation task contract inside `backend/internal/agentbuilder` docs or config.
2. Instruct the workflow to:
- query Elastic MCP for relevant telemetry, incident history, and endpoint logs
- reason only from provided/queried evidence
- return JSON matching `InvestigationResult`
3. Include explicit rules in the prompt/spec:
- do not return shell commands
- do not recommend actions outside the provided catalog
- produce concise, operator-facing language
4. Include decision heuristics for the MVP incident type:
- stopped monitored service while heartbeat is present

Output:
- a stable prompt/workflow contract that reduces output drift.

---

### 4.3 Elastic MCP retrieval plan

Goal: specify exactly what operational context Agent Builder should retrieve from Elastic.

Tasks:
1. Reuse `ElasticContextHints` from Phase 6.
2. Retrieve at least:
- telemetry events for the device during the incident window
- incident events for the current incident id
- endpoint log snippets for the device/service during the incident window
3. Keep query scope bounded:
- last N minutes around incident detection
- specific device and service filters
4. Summarize the retrieved evidence into a compact investigation context before passing to reasoning.

Output:
- Agent Builder uses Elastic MCP for concrete, bounded context retrieval.

---

### 4.4 Backend parsing and validation of AI result

Goal: safely parse and validate structured output before storing or showing it.

Tasks:
1. Create `backend/internal/agentbuilder/parser.go`.
2. Implement `ParseInvestigationResult(raw []byte) (InvestigationResult, error)`.
3. Validate:
- `probableCause` is non-empty
- `confidence` is within bounds
- every `recommendedAction.actionId` is in whitelist
- `validationSteps` is non-empty
4. Reject malformed or unsafe outputs.
5. Preserve raw payload for debugging if parsing fails.

Output:
- backend only accepts structured, safe investigation output.

---

### 4.5 Incident enrichment with AI result

Goal: attach the investigation result to the active incident without changing the incident lifecycle incorrectly.

Tasks:
1. Extend incident model/storage to hold AI investigation fields:
- `probableCause`
- `confidence`
- `recommendedActions`
- `validationSteps`
- `summary`
- `investigatedAt`
2. Add incident store update method for investigation results.
3. Update `UpdatedAt` when the result is saved.
4. Keep lifecycle state separate from content storage.

Output:
- incident becomes the source of truth for AI diagnosis and recommendation content.

---

### 4.6 Backend orchestration flow: handoff -> retrieve -> reason -> store

Goal: wire the complete investigation path into incident handling.

Tasks:
1. Decide the trigger for investigation:
- when incident enters `investigating`
- or when operator explicitly asks for investigation in MVP debug mode
2. Call Agent Builder client with structured request.
3. Receive workflow result.
4. Parse and validate structured response.
5. Store validated result on the incident.
6. Broadcast incident update to frontend.

Output:
- active incident can accumulate AI diagnosis and remediation recommendation automatically.

---

### 4.7 Timeout and fallback behavior

Goal: keep the demo stable when cloud calls or MCP retrieval are slow or unavailable.

Tasks:
1. Define investigation timeout budget.
2. If investigation times out:
- store failure metadata or fallback status
- keep incident actionable
- show understandable UI state such as `investigation pending` or `investigation unavailable`
3. Provide local stub/fallback result path for development/demo backup.
4. Log timeout cause and request identifiers.

Output:
- investigation failures are visible and survivable, not catastrophic.

---

### 4.8 Request/response traceability and evidence logging

Goal: make AI investigation behavior debuggable under hackathon conditions.

Tasks:
1. Log at minimum:
- `request_id`
- `incident_id`
- `device_id`
- `agent_builder_trace_id`
- `investigation_status`
2. Log a redacted summary of:
- retrieved evidence counts
- resulting confidence
- selected action IDs
3. Keep enough detail to explain the output during demo rehearsal.

Output:
- investigation path is observable from backend logs and artifacts.

---

### 4.9 Frontend AI investigation panel baseline

Goal: show the investigation result clearly without waiting for later polish phases.

Tasks:
1. Extend frontend incident types to include AI result fields.
2. Update investigation card/panel in `DashboardPage.tsx` or new component.
3. Render:
- probable cause
- confidence
- recommended actions
- validation steps
- concise summary
4. Show loading/fallback states when investigation is pending or failed.
5. Keep the display compact and demo-friendly.

Output:
- dashboard visibly shows the AI diagnosis and recommendation.

---

### 4.10 Safety validation for recommended actions

Goal: ensure later remediation phases receive only approved action IDs.

Tasks:
1. Create backend-side whitelist check reused during parsing.
2. Reject any output containing:
- raw shell commands
- unexpected action IDs
- unsupported targets if validation rules require them
3. Keep reasons/explanations free-form but actions constrained.

Output:
- AI output remains compatible with safe approval/execution flow in later phases.

---

### 4.11 Tests for schema, parsing, fallback, and UI integration

Goal: lock behavior before approval/execution phases depend on it.

Tasks:
1. Add parser tests for valid output.
2. Add parser tests for invalid action IDs and malformed JSON.
3. Add storage tests verifying incident enrichment.
4. Add timeout/fallback tests using fake Agent Builder client.
5. Add frontend tests where practical or at minimum build validation for investigation rendering.

Output:
- confidence that AI investigation output remains parseable and safe.

---

### 4.12 Phase 7 smoke-check path

Goal: produce repeatable evidence that an active incident gets an AI diagnosis and remediation recommendation.

Manual/Script sequence:
1. Start backend, agent, frontend, and required Elastic/Agent Builder dependencies.
2. Trigger stopped-service incident.
3. Wait for incident to enter `investigating`.
4. Trigger or observe investigation workflow completion.
5. Verify:
- incident contains `probableCause`
- incident contains allowed `recommendedActions`
- incident contains `validationSteps`
- dashboard shows diagnosis and recommendation
6. Capture evidence under `artifacts/phase7-smoke/<timestamp>/`:
- backend logs
- investigation request/response snapshot (redacted)
- incident API snapshot
- optional dashboard screenshot

Output:
- repeatable proof that Elastic MCP-backed investigation produces structured AI guidance.

---

## 5) Detailed implementation order

Build in this order to keep behavior testable and low-risk:

1. `backend/internal/agentbuilder/investigation_model.go`
2. `backend/internal/agentbuilder/parser.go`
3. Add action whitelist validation helpers
4. Extend incident model/store to hold AI investigation fields
5. Wire backend orchestration for investigation result storage
6. Add timeout and fallback behavior
7. Update frontend incident/investigation rendering
8. Add parser/storage/fallback tests
9. Run smoke-check and collect evidence

Why this order:
- schema and parser first prevent unsafe output drift
- storage comes before UI so data flow is stable
- timeout/fallback handling is critical before demo-facing rendering
- frontend follows once backend contracts are settled

---

## 6) Key implementation details and pitfalls

1. Keep reasoning structured
- free-form text without strict JSON will slow progress and create parsing risk.

2. Use Elastic MCP meaningfully
- if the result does not clearly depend on indexed telemetry/logs/incidents, the track story weakens.

3. Do not permit arbitrary commands
- action IDs only, validated in backend.

4. Keep confidence interpretable
- numeric confidence is useful, but avoid pretending precision beyond the available evidence.

5. Separate workflow failure from incident failure
- if investigation fails, the incident still exists and can proceed with fallback/manual handling.

6. Bound payloads and results
- concise context and concise result improve latency and demo reliability.

---

## 7) Environment variables reference (Phase 7)

Phase 7 may reuse Phase 6 Agent Builder configuration.

Optional additions if needed:
- `AGENT_BUILDER_INVESTIGATION_TIMEOUT_MS`
- `AGENT_BUILDER_FALLBACK_MODE`

Prefer reusing existing config unless a separate timeout/fallback knob is truly needed.

---

## 8) New files to create

- `backend/internal/agentbuilder/investigation_model.go`
- `backend/internal/agentbuilder/parser.go`
- `backend/internal/agentbuilder/*_test.go`
- optional frontend investigation component files if UI is split

## 9) Files to modify

- `backend/internal/incidents/*`
- `backend/cmd/server/main.go`
- `frontend/src/types/dashboard.ts` or dedicated incident types file
- `frontend/src/pages/DashboardPage.tsx`
- `frontend/src/App.css`
- optionally `scripts/smoke-check.ps1` or a Phase 7 companion smoke script

---

## 10) Phase 7 acceptance gate

Pass when all three are true (from [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md)):

- [ ] Probable cause is produced in structured response.
- [ ] Recommended action IDs (not raw shell commands) are returned.
- [ ] Validation steps are returned.
