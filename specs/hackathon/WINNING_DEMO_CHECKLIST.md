# Winning Demo Checklist

## Goal
Deliver a reliable, high-impact, judge-friendly demo in under 3 minutes.

## Pre-demo readiness
- [ ] Demo machine is Windows and monitored service is confirmed.
- [ ] Telemetry heartbeat visible in dashboard.
- [ ] Incident trigger script tested twice successfully.
- [ ] Recovery fallback script tested and available.
- [ ] Elastic indices contain at least one healthy and one failing sample.
- [ ] Agent Builder request and response logging is enabled.
- [ ] Approval workflow is functional and records approver identity.
- [ ] Remediation whitelist is enforced and deny path is tested.

## Golden path sequence (target: 2-3 minutes)
1. [ ] Show healthy state with heartbeat and service running.
2. [ ] Trigger deterministic service failure.
3. [ ] Show incident auto-detection and timeline transition.
4. [ ] Show AI probable cause with evidence snippet.
5. [ ] Show recommended action ID and confidence.
6. [ ] Approve remediation from dashboard.
7. [ ] Show execution result from endpoint.
8. [ ] Show validation cycles and resolved state.
9. [ ] Show final summary and measurable outcome.

## Judge-visible metrics
- [ ] Detection latency displayed.
- [ ] Approval latency displayed.
- [ ] Remediation execution duration displayed.
- [ ] Mean time to recovery (MTTR) displayed.
- [ ] Success/failure outcome displayed clearly.

## Safety and trust checks to demonstrate
- [ ] Show that free-form command input is rejected.
- [ ] Show only whitelisted action IDs are executable.
- [ ] Show audit trail: recommendation -> approval -> execution -> validation.
- [ ] Show failed validation path handling (if time permits).

## Backup plan (if cloud/integration is flaky)
- [ ] Use canned telemetry/log dataset to continue timeline narrative.
- [ ] Use cached or mocked AI recommendation with identical schema.
- [ ] Keep approval, execution, and validation path live.
- [ ] Continue to final summary without dead ends.

## Presentation quality checks
- [ ] One-screen dashboard with clear state colors and no noisy text.
- [ ] Large, readable labels for state and next action.
- [ ] Timeline and evidence panels are visible without scrolling.
- [ ] Final summary is concise and copyable/exportable.

## Final 10-minute pre-stage checks
- [ ] Restart app stack and verify ports.
- [ ] Run one dry rehearsal from healthy to resolved.
- [ ] Clear old incidents or mark a fresh session.
- [ ] Confirm fallback files and scripts are accessible.
- [ ] Confirm internet-independent path still works.

## Success criteria
- [ ] End-to-end loop completes live.
- [ ] Judges can see evidence, safety gates, and recovery proof.
- [ ] Demo finishes under 3 minutes with no manual improvisation.
