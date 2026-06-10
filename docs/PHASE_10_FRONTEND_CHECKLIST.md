# Phase 10 Frontend Manual Verification Checklist

The frontend has no automated test runner configured (no Vitest/Jest/Testing Library in
`frontend/package.json`). Per Phase 10 task 4.10.3, frontend coverage is satisfied by
**build verification plus this manual checklist**.

## Build verification

- [ ] `cd frontend && npm run build` completes with no TypeScript or Vite errors.
- [ ] `npm run lint` introduces no new errors in the Phase 10 files
      (`ValidationPanel.tsx`, `DashboardPage.tsx`, `types/dashboard.ts`,
      `hooks/useIncidents.ts`). Two pre-existing errors in `useDeviceState.ts` and the
      `DashboardPage` device-select effect are unrelated to Phase 10.

## Manual validation-panel checklist

Run the backend and frontend locally and drive an incident through the full lifecycle
(detect -> investigate -> approve -> execute -> validate). The agent must keep sending
telemetry so post-remediation cycles are observed.

### Validation display (4.8.1, 4.8.2)
- [ ] The Recovery Validation panel is idle/empty before remediation runs.
- [ ] While `executing`, the panel shows the executing phase.
- [ ] On entering `validating`, the panel shows lifecycle state, validation status,
      and the validation start timestamp.
- [ ] Healthy cycle progress dots fill as consecutive healthy telemetry arrives
      (e.g. `1 / 2`, then `2 / 2`).
- [ ] Validation criteria list shows each check (heartbeat, serviceStatus,
      networkReachable) with pass/fail and the observed detail.
- [ ] "Last checked" line reflects the most recent telemetry's service status/heartbeat.

### Visual state distinction (4.8.3)
- [ ] `executing` renders with the slate/neutral accent.
- [ ] `validating` renders with the amber accent and a pulsing chip.
- [ ] `resolved` renders with the green accent and a "recovery proven" outcome banner.
- [ ] `failed` renders with the red accent and a failure outcome banner.

### Closure UX (4.8.2, 4.8.4)
- [ ] On resolution, the outcome banner reads "Recovery proven by N healthy telemetry
      cycles" and the end timestamp is populated.
- [ ] The endpoint Health card returns to the normal green/healthy state once the
      service reports `running` again.
- [ ] The validation outcome remains visible after the incident deactivates (it does
      not vanish the instant `active` flips false).

### Failure visibility (4.9)
- [ ] A validation timeout shows `failed` with the recorded failure reason
      (e.g. service still stopped / no fresh telemetry).
- [ ] A command failure (remediation itself fails) shows the distinct "command failed"
      message pointing to the Remediation Execution panel — not a validation failure.
- [ ] Execution results (per-action stdout/stderr) remain visible alongside the failed
      validation evidence.
- [ ] The AI recommendation remains visible (read-only) on a failed incident for manual
      re-run / further investigation.

## Backend acceptance reference

Telemetry-driven closure rules are locked by automated backend tests:
- `backend/cmd/server/validation_acceptance_test.go` (end-to-end HTTP pipeline)
- `backend/internal/incidents/validation_test.go`,
  `validation_timeout_test.go`, `validation_evidence_test.go`, `freshness_test.go`
- `backend/internal/incidents/state_machine_test.go` (valid lifecycle transitions)
