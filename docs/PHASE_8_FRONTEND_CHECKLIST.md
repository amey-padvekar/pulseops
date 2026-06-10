# Phase 8 Frontend Manual Verification Checklist

The frontend has no automated test runner configured (no Vitest/Jest/Testing Library
in `frontend/package.json`). Per Phase 8 task 4.10.3, frontend coverage is satisfied by
**build verification plus this manual checklist**.

## Build verification

- [ ] `cd frontend && npm run build` completes with no TypeScript or Vite errors.
- [ ] `npx tsc --noEmit` is clean for the approval files
      (`RemediationApprovalCard.tsx`, `useApproveIncident.ts`, `types/dashboard.ts`).

## Manual approval-panel checklist

Run the backend and frontend locally, then drive an incident to `awaiting_approval`
(detection -> investigation completes with a non-empty recommendation).

### Recommendation review (4.6.2)
- [ ] Remediation Approval card shows probable cause and confidence.
- [ ] Recommended actions are listed with their target.
- [ ] Validation steps are listed when present.
- [ ] Current approval state is shown.

### Gating (4.6.4)
- [ ] With no recommendation, the card shows the empty/locked message and no approve control.
- [ ] While the incident is `investigating` (not yet promoted), approval controls are not active.
- [ ] The Approve button is disabled when the approver field is empty.
- [ ] The Approve button is disabled while a submission is in progress.

### Approval action (4.6.3, 4.6.5)
- [ ] Approver identity input is present (defaults to `demo.operator`).
- [ ] Action checkboxes default to all recommended actions selected; deselecting works.
- [ ] Optional note accepts text up to 500 characters.
- [ ] Submitting a valid approval flips the card to the green "approved" view showing
      approver and timestamp without a manual refresh (websocket update).
- [ ] A rejected approval (e.g. backend returns 4xx) shows an inline error and leaves
      the controls usable.

### Live sync and durability (4.7.4)
- [ ] After approval, a hard browser refresh still shows the approved state and approver.
- [ ] The Incident Timeline card reflects the `approved` state alongside the approval card.

## Backend automated coverage (for reference)

The approval contract itself is locked by Go tests; this checklist only covers the UI.
See `internal/api/approval_handler_test.go`, `internal/api/approval_test.go`,
`internal/incidents/approval_test.go`, and `internal/remediation/` tests for the
success path and every rejection path (not found, wrong method, missing approver,
no recommendation, invalid state transition, invalid/non-catalog action, duplicate).
