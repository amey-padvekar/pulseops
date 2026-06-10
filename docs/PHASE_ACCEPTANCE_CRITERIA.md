# PulseOps AI Phase Acceptance Criteria

Use this file as the pass/fail gate before moving to the next phase.

## Phase 0: Scope and Contracts

Pass when:
- [ ] One primary incident type is locked.
- [ ] Telemetry, incident, remediation, summary payload contracts are documented.
- [ ] Remediation whitelist is documented.
- [ ] Incident lifecycle states are defined.

## Phase 1: Project Setup

Pass when:
- [ ] Agent, backend, and frontend each run locally.
- [ ] Shared schemas are accessible to all components.
- [ ] Environment variables are documented.

## Phase 2: Endpoint Telemetry

Pass when:
- [ ] Agent sends heartbeat every configured interval.
- [ ] Service health and key telemetry fields arrive at backend.
- [ ] Endpoint remains reachable during simulated service failure.

## Phase 3: Backend Ingestion and UI State

Pass when:
- [ ] Backend stores latest endpoint state.
- [ ] Dashboard updates live without manual refresh.
- [ ] Healthy baseline is visually stable for at least 60 seconds.

## Phase 4: Incident Detection

Pass when:
- [ ] Service stop event creates incident automatically.
- [ ] Duplicate events do not create duplicate active incidents.
- [ ] Incident state transitions to investigating path.

## Phase 5: Elastic Integration

Pass when:
- [ ] Telemetry and incident events are queryable in Elastic.
- [ ] Required fields (device, service, timestamp, severity) are indexed.
- [ ] At least one incident context query returns expected records.

## Phase 6: Agent Builder Handoff

Pass when:
- [ ] Backend sends structured incident context request.
- [ ] Request/response IDs are traceable for debugging.
- [ ] Retries and timeout behavior are documented.

## Phase 7: AI Investigation Output

Pass when:
- [ ] Probable cause is produced in structured response.
- [ ] Recommended action IDs (not raw shell commands) are returned.
- [ ] Validation steps are returned.

## Phase 8: Human Approval

Pass when:
- [ ] Recommendation appears in dashboard.
- [ ] Approval is required before execution.
- [ ] Approver identity and timestamp are recorded.

## Phase 9: Remediation Execution

Pass when:
- [ ] Only whitelisted actions execute.
- [ ] Endpoint reports action result to backend.
- [ ] Execution logs are visible in dashboard.

## Phase 10: Recovery Validation

Pass when:
- [x] Health is confirmed by fresh telemetry cycles.
- [x] Incident transitions to resolved only after validation passes.
- [x] Failure to recover transitions to failed/unhealthy state.

Verified by `backend/cmd/server/validation_acceptance_test.go` and the
`internal/incidents` validation/state-machine tests; frontend per
[PHASE_10_FRONTEND_CHECKLIST.md](PHASE_10_FRONTEND_CHECKLIST.md).

## Phase 11: Incident Summary

Pass when:
- [ ] Final summary includes root cause, evidence, actions, and result.
- [ ] Summary is displayed in dashboard.

## Phase 12: Demo Readiness

Pass when:
- [ ] Full flow completes under 3 minutes in rehearsal.
- [ ] One fallback scenario is tested end-to-end.
- [ ] Submission artifacts are complete and linked.
- [ ] Dashboard UI quality gate passes — every item in
      [docs/PHASE_12_UI_VERIFICATION.md](PHASE_12_UI_VERIFICATION.md) is checked at the
      capture viewport.
