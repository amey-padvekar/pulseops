# PulseOps AI Phase 10 Detailed Plan

Phase: 10 - Recovery validation and incident closure  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Prove that remediation actually restored endpoint health and only then close the incident.

Phase 9 established real remediation execution and execution-result reporting. Phase 10 adds the recovery proof layer: the backend watches fresh telemetry after remediation, evaluates explicit health criteria, and transitions the incident to `resolved` only after validation succeeds. If health does not recover, the incident remains unresolved and moves into a failure path.

At the end of Phase 10:
- incident enters a validation phase after remediation execution completes
- backend confirms recovery using fresh telemetry cycles rather than command success alone
- successful recovery transitions the incident to `resolved`
- failed recovery transitions the incident to `failed` or a clearly unhealthy terminal state
- dashboard shows the before/after outcome clearly enough for demo narration

---

## 2) Rule-aware constraints for Phase 10

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Functional agent requirement
- PulseOps must perform real operational work, not just advisory reasoning.
- Recovery must be demonstrated through observable system behavior.
- The incident flow shown in the dashboard must reflect actual backend decisions based on real telemetry.

2. Safety and correctness
- Do not mark incidents resolved because a command reported success.
- Validation criteria must be explicit, bounded, and tied to telemetry fields already collected by the agent.
- Failed remediation must remain visible so operators can act again or investigate further.

3. Stack compliance
- Gemini and Agent Builder remain upstream reasoning components; Phase 10 validation logic belongs in backend incident-management logic.
- Do not introduce competing monitoring or automation services for validation.

4. Demo readiness
- Validation criteria should be deterministic for the MVP incident type.
- Keep the validation window short enough for a live demo but long enough to avoid false positives.
- UI should clearly distinguish `executing`, `validating`, `resolved`, and `failed`.

5. Phase boundary
- Phase 10 proves operational recovery and closes the incident.
- Phase 11 will generate the polished final incident summary after the lifecycle is complete.
- Phase 10 should not add a full second investigation workflow unless needed as a documented future extension.

---

## 3) Phase 10 definition of done

Phase 10 is complete only when all are true:

1. Remediation completion can move an incident into `validating`.
2. Validation uses fresh telemetry received after remediation execution.
3. Validation requires 1-2 healthy telemetry cycles before resolution.
4. Successful validation transitions the incident to `resolved`.
5. Failed validation transitions the incident to `failed` or a clearly defined unhealthy state.
6. Backend stores validation evidence and timestamps on the incident.
7. Dashboard shows validation progress and final recovery outcome.
8. Resolution is not possible without post-remediation health evidence.
9. Phase 10 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Validation state transition rules

Goal: define the lifecycle boundary between execution, validation, and closure.

Tasks:
1. Define allowed transitions:
- `executing` -> `validating`
- `validating` -> `resolved`
- `validating` -> `failed`
2. Decide when to enter `validating`:
- immediately after successful remediation result ingestion
- or after backend marks execution complete regardless of success, with failure shortcut logic
3. Reject direct transition from `executing` to `resolved`.
4. Centralize transition rules in incident domain/service logic rather than scattering them across handlers.

Output:
- the incident lifecycle enforces recovery proof before closure.

---

### 4.2 Recovery validation criteria

Goal: define one deterministic health rule set for the MVP incident type.

Recommended baseline criteria for the service-failure demo:
- `heartbeat == true`
- `serviceStatus == running`
- `networkReachable == true` if that signal is reliable enough in the chosen demo environment

Tasks:
1. Define the exact set of fields required for validation.
2. Keep criteria grounded in telemetry already produced by the agent.
3. Decide whether all criteria are mandatory or whether connectivity is optional.
4. Record validation reasons so the UI can explain why the incident resolved or failed.

Output:
- backend has a deterministic validator for post-remediation health.

---

### 4.3 Fresh telemetry window and sequence tracking

Goal: ensure validation is based on new telemetry after remediation, not stale pre-remediation state.

Tasks:
1. Capture the execution completion boundary on the incident, for example:
- `executionFinishedAt`
- latest processed telemetry timestamp or sequence at execution completion
2. Evaluate only telemetry events newer than that boundary.
3. Ignore stale telemetry snapshots that arrived before or during remediation.
4. If current telemetry transport lacks sequence IDs, use timestamp comparison with UTC normalization.

Output:
- validation decisions rely only on post-remediation evidence.

---

### 4.4 Healthy-cycle counter

Goal: reduce false positives by requiring consecutive healthy observations.

Tasks:
1. Add a validation progress model such as:
- `healthyCycleCount`
- `requiredHealthyCycles`
- `lastValidationTelemetryAt`
2. Increment count when a fresh telemetry event passes validation criteria.
3. Reset or hold the count when a fresh telemetry event remains unhealthy.
4. Resolve the incident only when the required count is reached.
5. Start with `requiredHealthyCycles = 2` for realism, but allow temporary reduction to `1` if the live demo timing demands it.

Output:
- transient momentary health does not immediately close the incident.

---

### 4.5 Validation timeout and failure path

Goal: avoid incidents getting stuck in `validating` forever.

Tasks:
1. Define a validation timeout window appropriate for the telemetry interval.
2. If no healthy telemetry arrives before timeout:
- mark incident `failed`
- or mark a clearly named unhealthy terminal state if the model allows it
3. Record why validation failed, for example:
- service still stopped
- heartbeat missing
- connectivity still down
- no fresh telemetry received
4. Preserve enough detail for later summary generation and operator debugging.

Output:
- validation has a deterministic failure outcome instead of silent limbo.

---

### 4.6 Backend validation engine wiring

Goal: connect telemetry ingestion to incident validation evaluation.

Tasks:
1. Extend telemetry ingestion flow to check for incidents in `validating` state for the same device.
2. Evaluate fresh telemetry against validation criteria.
3. Update validation progress counters and reasons.
4. Transition incident state when success or failure threshold is met.
5. Broadcast incident changes to frontend immediately.

Output:
- validation becomes an automatic part of the existing telemetry pipeline.

---

### 4.7 Incident validation fields and evidence model

Goal: store enough information to explain how recovery was determined.

Tasks:
1. Extend incident model with validation metadata such as:
- `validationStartedAt`
- `validatedAt`
- `validationStatus`
- `healthyCycleCount`
- `requiredHealthyCycles`
- `validationFailures` or `validationReason`
- `lastValidatedTelemetrySnapshot` or a compact evidence summary
2. Keep validation evidence separate from execution logs and AI diagnosis.
3. Update `UpdatedAt` as validation progresses.

Output:
- incident record can justify why it resolved or failed.

---

### 4.8 Frontend validation and closure UX

Goal: make recovery progress visible and intuitive in the dashboard.

Tasks:
1. Add validation state display to the incident panel.
2. Show:
- current lifecycle state
- healthy cycle progress
- validation criteria summary
- validation start and end timestamps
- clear final outcome: resolved or failed
3. Visually distinguish:
- `executing`
- `validating`
- `resolved`
- `failed`
4. Ensure the healthy device card returns to the normal green state when the incident resolves.

Output:
- users can see that recovery was proven, not assumed.

---

### 4.9 Failure handling and operator visibility

Goal: keep failed remediation attempts actionable.

Tasks:
1. Surface validation failure reasons in the dashboard.
2. Preserve execution results and failed validation evidence together.
3. Decide whether a failed validation keeps the recommendation visible for retry/manual intervention.
4. Avoid hiding or archiving failed incidents prematurely.

Output:
- operators can distinguish command failure, validation failure, and eventual recovery.

---

### 4.10 Tests for recovery validation

Goal: prove that closure logic is driven by telemetry evidence.

Tasks:
1. Backend tests for:
- remediation success followed by healthy telemetry resolves incident
- only one healthy cycle does not resolve when two are required
- stale telemetry is ignored
- unhealthy telemetry during validation leads to failure or timeout path
- no fresh telemetry triggers failure after timeout
- command success without healthy telemetry does not resolve incident
2. Incident lifecycle tests for valid state transitions.
3. Frontend verification through build plus manual checklist unless UI test conventions already exist.

Output:
- validation rules are locked before Phase 11 summary logic depends on them.

---

### 4.11 Manual rehearsal and smoke proof

Goal: create a repeatable demonstration that recovery is visibly confirmed.

Tasks:
1. Extend local rehearsal steps to include:
- incident enters `executing`
- incident enters `validating`
- one or two fresh healthy telemetry cycles arrive
- incident resolves visibly in dashboard
2. Add a negative-path rehearsal:
- remediation executes
- health does not return
- incident becomes `failed` or unhealthy terminal state
3. Capture evidence artifacts if useful under `artifacts/`.

Output:
- Phase 10 is proven through observable state transitions, not just code inspection.

---

## 5) Recommended implementation order

Implement Phase 10 in this order:

1. Define validation criteria and lifecycle transitions.
2. Extend incident model with validation metadata.
3. Wire telemetry ingestion to validation evaluation using fresh post-execution telemetry.
4. Add healthy-cycle counting and timeout logic.
5. Broadcast and render validation state in the dashboard.
6. Add backend tests for success, stale-data rejection, and failure paths.
7. Run positive and negative manual validation rehearsals.

This order keeps the domain rules stable before UI work and prevents accidental coupling between command success and incident resolution.

---

## 6) File-by-file implementation map

Expected backend touch points:
- `backend/internal/incidents/` for lifecycle transition and validation logic
- `backend/internal/store/` if incident persistence currently lives there
- `backend/cmd/server/main.go` where telemetry ingestion currently drives incident updates
- `backend/internal/api/` if incident DTOs need validation metadata exposed to frontend

Expected frontend touch points:
- `frontend/src/types/` for validation status and evidence fields
- `frontend/src/pages/DashboardPage.tsx` for validation/closure rendering
- `frontend/src/components/` for incident timeline or validation card extraction if needed
- `frontend/src/App.css` for resolved/failed/validating styling

Expected docs/scripts touch points:
- `scripts/smoke-check.ps1` or a follow-up rehearsal script if you want automated proof for validation transitions
- runbook docs if the recovery proof path needs explicit operator steps

---

## 7) Pitfalls to avoid

1. Do not equate command success with service recovery.
- Restarting a service can succeed while the service immediately fails again or connectivity remains down.

2. Do not validate against stale telemetry.
- Without a fresh-telemetry boundary, the system can incorrectly resolve incidents using pre-remediation data.

3. Do not leave validation state ambiguous.
- Operators need a clear distinction between still validating, validated successfully, and failed to recover.

4. Do not let incidents stay in limbo.
- Add timeout or explicit failure rules so the demo remains deterministic.

5. Do not overcomplicate the MVP rule set.
- Use the minimum reliable criteria needed for the chosen incident type.

6. Do not hide failure evidence.
- Failed recovery is still valuable demo behavior and important for later AI summary generation.

---

## 8) Acceptance gate for moving to Phase 11

Do not start Phase 11 until all checks below are true:

1. Incident enters `validating` only after remediation execution completes.
2. Fresh telemetry cycles drive recovery decisions.
3. Incident resolves only after validation criteria pass.
4. Failed recovery transitions to `failed` or the agreed unhealthy terminal state.
5. Dashboard clearly shows validation progress and final closure outcome.
6. Validation evidence is stored on the incident.
7. The Phase 10 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are demonstrably satisfied.

---

## 9) Outcome of this phase

When Phase 10 is complete, PulseOps AI will be able to demonstrate a full operational loop: approve remediation, execute it on the endpoint, and prove from fresh telemetry whether the endpoint actually recovered. That recovery proof is what makes the system credible as an operational agent rather than a command runner, and it creates the clean incident endpoint that Phase 11 can summarize.