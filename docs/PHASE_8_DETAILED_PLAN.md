# PulseOps AI Phase 8 Detailed Plan

Phase: 8 - Human approval workflow  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Introduce the human approval control point between AI recommendation and endpoint remediation execution.

Phase 7 produces a structured investigation result with recommended action IDs. Phase 8 turns that recommendation into an operator-visible, explicitly approved remediation plan that can later be executed safely in Phase 9.

At the end of Phase 8:
- dashboard shows the active recommendation for the incident
- remediation cannot proceed without explicit human approval
- backend records approver identity and approval timestamp
- incident lifecycle transitions cleanly from `awaiting_approval` to `approved`
- backend prepares a queued remediation command using approved action IDs, not raw commands

---

## 2) Rule-aware constraints for Phase 8

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Stack compliance
- Gemini remains responsible for recommendation generation, not direct command execution.
- Google Cloud Agent Builder remains the orchestration layer that produced the recommendation in Phase 7.
- Elastic integration remains meaningful through the broader incident flow, but approval logic must stay in the backend/dashboard control plane.
- Do not introduce competing approval, workflow, or AI services.

2. Safety and governance
- Approval must be required before execution.
- AI may recommend only action IDs and targets, never shell commands.
- Backend must not auto-approve or auto-execute recommendations.
- Approval metadata must be durable enough for demo evidence and operator auditability.

3. Functional submission requirement
- The approval flow must work in the live web app.
- The state shown in the dashboard must reflect real backend incident state, not frontend-only local state.
- The queued remediation payload must be derived from the approved recommendation actually attached to the incident.

4. Demo readiness
- Operator identity capture must be simple and deterministic for the demo.
- The UI should make it visually obvious that execution is blocked until approval happens.
- Failed or duplicate approval attempts must return clear feedback.

5. Boundaries for this phase
- Phase 8 stops at approval and queueing intent.
- Actual endpoint delivery and execution belong to Phase 9.
- Recovery validation belongs to Phase 10.

---

## 3) Phase 8 definition of done

Phase 8 is complete only when all are true:

1. Recommendation appears in the dashboard for an active incident.
2. Incident cannot enter execution flow until approval is recorded.
3. Backend exposes an approval endpoint for the active incident.
4. Approval request records approver identity.
5. Approval request records approval timestamp.
6. Incident state transitions from `awaiting_approval` to `approved` only once per approval event.
7. Backend derives a remediation command payload from approved action IDs.
8. Duplicate or invalid approval attempts are rejected predictably.
9. Phase 8 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Incident model approval fields

Goal: make approval part of the incident source of truth.

Tasks:
1. Extend incident domain model to include:
- `approvalStatus` if needed, or rely on lifecycle state
- `approvedBy`
- `approvedAt`
- `approvalNote` optional for future use
- `approvedActions` or equivalent approved remediation snapshot
2. Preserve the original AI recommendation alongside approved action selection.
3. Add timestamps/update semantics so approval updates `UpdatedAt`.
4. Keep approval data separate from execution result data that will arrive in Phase 9.

Output:
- incident record can represent recommendation, approval, and later execution as distinct lifecycle milestones.

---

### 4.2 Approval state transition rules

Goal: make the approval gate explicit and impossible to bypass accidentally.

Tasks:
1. Define allowed transition:
- `awaiting_approval` -> `approved`
2. Reject approval for incidents already in:
- `approved`
- `executing`
- `validating`
- `resolved`
- `failed`
3. Decide behavior for incidents still in `investigating`:
- either reject approval until recommendation exists
- or require investigation completion before the button is enabled
4. Centralize transition validation in incident service/store logic rather than only in HTTP handlers.

Output:
- backend owns one enforceable rule set for approval eligibility.

---

### 4.3 Approval request contract

Goal: define the exact API payload used by dashboard and backend.

Suggested request shape:

```json
{
  "approvedBy": "demo.operator",
  "selectedActionIds": ["restart_service"],
  "note": "Approved after reviewing recommendation"
}
```

Suggested response shape:

```json
{
  "incidentId": "INC-1001",
  "state": "approved",
  "approvedBy": "demo.operator",
  "approvedAt": "2026-05-23T22:10:00Z",
  "queuedActions": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService"
    }
  ]
}
```

Tasks:
1. Create backend DTOs for approval request/response.
2. Require `approvedBy` in Phase 8 even if real authentication is deferred.
3. Validate `selectedActionIds` against the recommendation attached to the incident.
4. Keep `note` optional and bounded in length.

Output:
- frontend and backend share a clear approval contract.

---

### 4.4 Backend approval endpoint

Goal: add the write path that records approval and prepares downstream remediation.

Tasks:
1. Add endpoint such as:
- `POST /incidents/{incidentId}/approve`
2. Validate:
- method
- incident exists
- incident is in approvable state
- investigation result exists
- `approvedBy` is present
- selected actions are valid action IDs from recommendation
3. Record `approvedBy` and `approvedAt` using UTC.
4. Persist state change to `approved`.
5. Build queued remediation payload from the approved action set.
6. Return the updated incident or approval response DTO.

Output:
- one deterministic backend entry point for human approval.

---

### 4.5 Queued remediation command model

Goal: prepare the execution payload without executing it yet.

Tasks:
1. Define a remediation command model in backend remediation package or shared contract area.
2. Include at minimum:
- `incidentId`
- `deviceId`
- `approvedBy`
- `approvedAt`
- `actions`
3. Populate command from approved recommendation snapshot, not from freeform client input.
4. Store it in the incident record, in-memory queue, or remediation queue abstraction that Phase 9 will consume.
5. Mark clearly whether the payload is `queued`, `pending_dispatch`, or similar.

Output:
- Phase 9 can pick up a trustworthy queued command instead of reconstructing intent later.

---

### 4.6 Frontend approval panel

Goal: make the recommendation review and approval action visible and understandable in the dashboard.

Tasks:
1. Add or extend a remediation recommendation card in the dashboard.
2. Show:
- probable cause
- confidence
- recommended actions
- validation steps
- current approval state
3. Add approval controls:
- approver identity input for MVP
- approve button
- optional note field if low effort
4. Disable or hide approval action when:
- no recommendation exists
- incident is not `awaiting_approval`
- submission is in progress
5. Show success/error feedback after submission.

Output:
- operator can review the AI recommendation and explicitly approve from the UI.

---

### 4.7 Frontend/backend live state sync

Goal: ensure approval state changes propagate without manual refresh.

Tasks:
1. Extend incident DTOs sent to frontend with:
- `approvedBy`
- `approvedAt`
- queued remediation metadata if needed
2. Broadcast incident update after approval using the existing websocket/event path.
3. Update frontend incident state hook/store to merge approval fields.
4. Ensure a hard refresh still reproduces the approved state through REST fetch.

Output:
- approval is immediately visible and durable in the dashboard.

---

### 4.8 Audit trail and operator evidence

Goal: make approval demonstrable during rehearsal and submission prep.

Tasks:
1. Log approval events with at minimum:
- `incident_id`
- `device_id`
- `approved_by`
- `approved_at`
- approved action IDs
2. Add incident timeline/event entry for approval if the incident model already supports event history.
3. Keep timestamps in ISO 8601 UTC.
4. Ensure approval metadata can be shown in UI and in logs/artifacts.

Output:
- the system can prove who approved what and when.

---

### 4.9 Safety validation for approved actions

Goal: keep the approval path compatible with the action-ID-only safety model.

Tasks:
1. Never accept shell commands, scripts, or arbitrary parameters from the dashboard.
2. Validate selected actions against the recommendation already stored on the incident.
3. Validate each action ID against backend whitelist/remediation catalog.
4. Preserve target data from trusted backend recommendation content where possible.
5. Reject any mismatch between incident recommendation and approval request.

Output:
- approval confirms safe action IDs only; it does not create new executable instructions.

---

### 4.10 Tests for approval workflow

Goal: lock the approval gate before Phase 9 adds execution complexity.

Tasks:
1. Backend tests for:
- approve success path
- incident not found
- wrong method
- missing approver
- no recommendation present
- invalid state transition
- invalid action ID selection
- duplicate approval rejection
2. Store/service tests for approval metadata persistence.
3. Frontend tests if present in repo conventions, otherwise keep to build verification plus manual checklist.

Output:
- the approval contract is stable before endpoint execution is introduced.

---

### 4.11 Manual demo path and smoke coverage

Goal: make Phase 8 observable during local rehearsal.

Tasks:
1. Extend smoke or rehearsal checklist to include:
- active incident with recommendation visible
- approval submitted through API or UI
- state changes to `approved`
- approver/timestamp visible in response or dashboard
2. Capture example artifact output under `artifacts/` if useful.
3. Keep the flow short enough for later 3-minute demo packaging.

Output:
- Phase 8 has a repeatable proof path, not just code changes.

---

## 5) Recommended implementation order

Implement Phase 8 in this order:

1. Extend incident model with approval metadata and approved remediation snapshot.
2. Implement approval transition rules in incident service/store layer.
3. Add approval request/response DTOs and backend endpoint.
4. Add queued remediation command model.
5. Extend REST/websocket incident payloads.
6. Build dashboard remediation approval panel.
7. Add backend tests for success and rejection cases.
8. Run end-to-end manual verification with a real incident generated from earlier phases.

This order keeps state correctness ahead of UI work and avoids building approval controls on an unstable contract.

---

## 6) File-by-file implementation map

Expected backend touch points:
- `backend/internal/incidents/` for incident model and state transitions
- `backend/internal/remediation/` for queued command model or approval service
- `backend/internal/api/` for approval handler and incident response DTO updates
- `backend/cmd/server/main.go` for route wiring
- `backend/internal/store/` if incident persistence currently lives there

Expected frontend touch points:
- `frontend/src/types/` for incident/remediation approval types
- `frontend/src/pages/DashboardPage.tsx` for rendering approval controls
- `frontend/src/components/` for remediation/approval card extraction if needed
- `frontend/src/App.css` for approval-state styling

Expected docs/scripts touch points:
- `docs/PHASE_ACCEPTANCE_CRITERIA.md` only if clarifying notes are needed
- `scripts/smoke-check.ps1` or runbook docs for approval proof steps if Phase 8 is implemented immediately after planning

---

## 7) Pitfalls to avoid

1. Do not let frontend-only state imply approval.
- Approval must be stored in backend incident state and survive refresh.

2. Do not allow approval before recommendation exists.
- The operator must approve a concrete recommendation, not an empty incident shell.

3. Do not let the client invent executable content.
- Client may identify approved action IDs, but command details must be derived from trusted backend recommendation data.

4. Do not couple approval to execution completion.
- Phase 8 ends when approval is recorded and remediation is queued, not when the endpoint acts.

5. Do not bury approver metadata.
- It needs to be visible enough for judges and for internal debugging.

6. Do not weaken the hackathon story.
- Keep the UX focused on human-in-the-loop governance over AI-generated remediation.

---

## 8) Acceptance gate for moving to Phase 9

Do not start Phase 9 until all checks below are true:

1. Recommendation is visible in the dashboard for an active incident.
2. Approval cannot be triggered successfully unless incident is awaiting approval.
3. Approval records `approvedBy` and `approvedAt` on the incident.
4. Approved incident state is visible after websocket update and page refresh.
5. Backend can produce a queued remediation command payload from approved action IDs.
6. Rejected approval cases are covered by tests.
7. The Phase 8 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are demonstrably satisfied.

---

## 9) Outcome of this phase

When Phase 8 is complete, PulseOps AI will have a credible human-in-the-loop control: Gemini and Agent Builder can recommend a remediation, but a human operator must explicitly approve it before the backend prepares endpoint action. That safety gate is essential both for the product story and for the hackathon demo narrative.