# PulseOps AI Phase 13 Detailed Plan

Phase: 13 - Demo script, rehearsal, and failure-proofing  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Make the end-to-end PulseOps AI demo repeatable, fast, and resilient under hackathon conditions.

Phases 0 through 12 establish the product and polish the dashboard. Phase 13 turns that product into a reliable presentation system: deterministic failure trigger, clear operator script, timed rehearsal, fallback branches for flaky integrations, and submission-ready artifacts that prove the workflow works as shown.

At the end of Phase 13:
- the full demo can be executed in under 3 minutes consistently
- one fallback scenario is rehearsed end to end
- the operator has a deterministic script and recovery plan
- core logs, screenshots, and artifacts are ready for submission support
- the team can recover quickly from common live-demo failure modes without improvising

---

## 2) Rule-aware constraints for Phase 13

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Submission compliance
- The demo video must be 3 minutes or less.
- The demo must show the project functioning on the target platform.
- The flow shown in the demo must match what the project actually does.
- The hosted project, repository, and description must all remain consistent with the demo story.

2. Stack and story compliance
- The demo must clearly preserve the required story:
  Gemini for reasoning, Agent Builder for orchestration, Elastic MCP for operational context, human approval before endpoint action.
- Do not replace required integrations with competing technologies for the final demonstrated product.
- Fallback paths may simplify the scenario, but they must not undermine the rule-compliant architecture narrative.

3. Functional credibility
- The demo must show real system behavior, not disconnected slides or fake transitions.
- Failure-proofing should prefer deterministic inputs, cached evidence, and stable workflow branches rather than fabricated UI-only output.
- Human approval must remain visible before execution.

4. Rehearsal discipline
- Timing must be measured, not guessed.
- Each integration used in the demo needs a fallback or stop condition.
- Evidence capture should happen during rehearsal, not only at the final recording attempt.

5. Phase boundary
- Phase 13 focuses on operational readiness, rehearsal, artifacts, and fallback handling.
- It should avoid large new feature work unless a narrow blocker prevents reliable demo execution.

---

## 3) Phase 13 definition of done

Phase 13 is complete only when all are true:

1. Full demo completes under 3 minutes in rehearsal.
2. One fallback scenario is rehearsed end to end.
3. Deterministic failure trigger path is documented and tested.
4. Deterministic recovery or reset path is documented and tested.
5. Operator script includes narration and hard stop conditions.
6. Core demo artifacts are captured and organized.
7. Critical integrations have clear fallback behavior.
8. Phase 12 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are supported by rehearsal evidence.

---

## 4) Work breakdown structure

### 4.1 Final demo script

Goal: lock one concise operator flow that fits within the 3-minute limit.

Primary sequence should remain:
1. show healthy endpoint
2. trigger service failure
3. show incident detection
4. show AI investigation output
5. show remediation recommendation
6. approve remediation
7. show agent execution
8. show recovery validation
9. show final incident summary

Tasks:
1. Convert [docs/DEMO_RUNBOOK.md](docs/DEMO_RUNBOOK.md) into a final spoken operator script.
2. Add short narration lines for each state transition.
3. Keep narration technically accurate and compact.
4. Mark exact click points and expected screen states.

Output:
- presenter has one deterministic sequence instead of ad hoc narration.

---

### 4.2 Segment timing budget

Goal: enforce the 3-minute constraint with measured timing.

Tasks:
1. Keep a segment budget similar to the current runbook:
- intro and healthy state
- trigger failure
- detection and investigation
- recommendation and approval
- remediation execution
- recovery and summary
2. Measure actual timings across multiple rehearsals.
3. Trim narration or waiting time where segments repeatedly exceed budget.
4. Record target and actual times in rehearsal notes.

Output:
- the demo length is controlled by data, not optimism.

---

### 4.3 Deterministic failure trigger path

Goal: ensure the incident starts the same way every time.

Tasks:
1. Standardize the monitored service/process used in rehearsal.
2. Document the exact failure trigger command for the chosen OS.
3. Validate that the agent keeps heartbeating while the monitored service is stopped.
4. Add a backup monitored process/service if the primary service behaves inconsistently on the host.
5. Keep the trigger low-risk and reversible.

Output:
- the incident can be triggered on demand with minimal surprise.

---

### 4.4 Deterministic reset and recovery path

Goal: make it easy to return to a healthy baseline between takes.

Tasks:
1. Document the exact reset steps after each rehearsal.
2. Add a manual or scripted recovery fallback if the normal remediation path fails.
3. Confirm backend, agent, and dashboard can return to healthy state cleanly.
4. Ensure logs/artifacts from previous runs do not block the next run.

Output:
- the team can rehearse repeatedly without environment drift.

---

### 4.5 Integration-specific fallback plan

Goal: prevent one flaky dependency from killing the entire demo.

Tasks:
1. Rehearse fallback A from [docs/DEMO_RUNBOOK.md](docs/DEMO_RUNBOOK.md): cached or latest successful recommendation when AI latency is high.
2. Rehearse fallback B: pre-seeded telemetry/log scenario that preserves the workflow story.
3. Rehearse fallback C: alternate monitored service or process when the primary service mismatches the demo host.
4. Add explicit operator wording for when a fallback is invoked.
5. Decide which fallback branches are acceptable for the final recording versus live judging conversation.

Output:
- critical failure modes have rehearsed backup paths instead of improvised reactions.

---

### 4.6 Stop conditions and abort rules

Goal: protect the demo from spiraling into confusion mid-run.

Tasks:
1. Preserve and refine hard stop conditions from [docs/DEMO_RUNBOOK.md](docs/DEMO_RUNBOOK.md), including:
- heartbeat drops entirely
- approval control is missing
- recovery telemetry does not confirm health within the allowed window
2. Define the operator response for each stop condition:
- restart from healthy baseline
- switch to fallback scenario
- abandon the current take
3. Keep stop rules explicit and binary.

Output:
- the presenter knows when to stop and reset instead of narrating a broken run.

---

### 4.7 Rehearsal checklist and cadence

Goal: make rehearsal systematic enough to surface issues before recording day.

Tasks:
1. Create a repeated rehearsal cadence, for example:
- one technical dry run without narration
- one narrated run with timer
- one fallback-path run
2. Track pass/fail for each rehearsal.
3. Record what caused slowdowns or confusion.
4. Tighten the script after each rehearsal rather than improvising anew.

Output:
- rehearsal becomes a debugging and polish loop instead of a one-off test.

---

### 4.8 Artifact and evidence capture

Goal: build a reliable set of proof assets during rehearsal.

Tasks:
1. Continue using evidence directories under `artifacts/`.
2. Capture at minimum:
- backend logs
- agent logs
- screenshots or clips of key dashboard states
- one example final summary
- one example failure-path artifact if helpful
3. Timestamp and organize artifacts so the latest good run is obvious.
4. Use artifacts both for debugging and for submission support.

Output:
- the project has concrete proof of working behavior beyond memory or terminal scrollback.

---

### 4.9 Demo environment hardening

Goal: reduce surprises from the machine and runtime environment used during recording.

Tasks:
1. Freeze the demo machine configuration as much as possible.
2. Confirm environment variables, monitored service names, and device IDs are aligned.
3. Verify ports, browser tabs, and terminal layout before each take.
4. Disable or avoid obvious distractions such as notifications, sleep behavior, or unrelated heavy background processes.
5. Ensure the chosen host version is the same one used in final rehearsal.

Output:
- fewer failures come from the environment rather than the app itself.

---

### 4.10 AI latency and payload trimming

Goal: keep cloud-assisted parts fast enough for the demo window.

Tasks:
1. Reduce summary and investigation payload size to the minimum evidence needed.
2. Trim verbose logs before handing them to Agent Builder.
3. Favor deterministic compact prompts and bounded evidence sets.
4. Measure whether latency is acceptable after each trimming pass.

Output:
- AI-dependent steps stay within the demo timing envelope more reliably.

---

### 4.11 Scripted launch and smoke verification

Goal: ensure each demo run starts from a verified baseline.

Tasks:
1. Reuse and extend existing run scripts where needed:
- `scripts/run-backend.ps1`
- `scripts/run-agent.ps1`
- `scripts/run-frontend.ps1`
- `scripts/smoke-check.ps1`
2. Add a pre-demo checklist that confirms:
- backend health endpoint is live
- device telemetry is present
- frontend loads correctly
- expected device ID and service name match the host
3. Keep startup flow short and repeatable.

Output:
- the operator can verify readiness before triggering the incident.

---

### 4.12 Submission alignment pass

Goal: make sure the demo artifacts line up with the final submission package.

Tasks:
1. Verify the final demo story matches the hosted app and repository readme/setup path.
2. Ensure the open-source license, public repo requirement, and hosted URL plan are not contradicted by the demo setup.
3. Confirm the final summary and screenshots support the written description.
4. Make sure the demo explicitly shows meaningful use of Google Cloud and Elastic track requirements.

Output:
- the recorded demo, repo, and submission form tell the same story.

---

### 4.13 Final recording checklist

Goal: define the last pass before recording the official demo video.

Tasks:
1. Verify healthy baseline is stable.
2. Start timer before the take.
3. Confirm presenter notes are visible but unobtrusive.
4. Confirm fallback branch to use if the primary run deviates.
5. Confirm artifacts directory for the take is ready.
6. Confirm audio, browser zoom, and dashboard layout are recording-friendly.

Output:
- the final recording starts from a controlled state instead of guesswork.

---

## 5) Recommended implementation order

Implement Phase 13 in this order:

1. Lock the final operator script and timing budget.
2. Standardize failure trigger and reset procedures.
3. Rehearse and document fallback branches.
4. Tighten startup/smoke verification flow.
5. Capture rehearsal artifacts from successful and fallback runs.
6. Run multiple timed rehearsals and trim slow segments.
7. Complete submission-alignment review.
8. Execute final recording checklist.

This order reduces uncertainty early and makes later rehearsal passes increasingly closer to the real recording conditions.

---

## 6) File-by-file implementation map

Expected docs touch points:
- `docs/DEMO_RUNBOOK.md` for the final operator script and fallback wording
- `docs/SUBMISSION_DAY_CHECKLIST.md` for recording and submission readiness alignment
- `docs/DEPLOYMENT_AND_ENVIRONMENT.md` for environment freeze notes if needed
- `docs/PHASE_ACCEPTANCE_CRITERIA.md` only if clarifying rehearsal proof notes are useful

Expected scripts touch points:
- `scripts/smoke-check.ps1` for pre-demo verification improvements
- `scripts/run-all.ps1` if one-command startup becomes useful
- optional small rehearsal helper scripts for deterministic trigger/reset paths

Expected artifact touch points:
- `artifacts/phase3-smoke/` and later artifact directories for rehearsal evidence
- dedicated final-demo artifact folder if you want separation between smoke evidence and demo evidence

Potential product touch points only if a blocker is discovered:
- small timeout or cached-response improvements in backend/agentbuilder flow
- minor dashboard copy/state tweaks that materially improve narration

---

## 7) Pitfalls to avoid

1. Do not rely on perfect live cloud timing.
- Rehearsed fallback branches are mandatory.

2. Do not change the story from one artifact to another.
- The repo, runbook, hosted app, and demo must all tell the same technical story.

3. Do not improvise the failure trigger on recording day.
- The monitored service and commands should be frozen beforehand.

4. Do not wait until final recording to collect evidence.
- Rehearsal artifacts are part of the safety net.

5. Do not let the demo drift above 3 minutes.
- Cut waiting and narration before cutting the core workflow proof.

6. Do not let a failed run continue too long.
- Use explicit abort rules and restart cleanly.

7. Do not forget environment consistency.
- Device IDs, service names, and ports must match across agent, backend, frontend, and scripts.

---

## 8) Acceptance gate for submission readiness

Do not treat the project as demo-ready until all checks below are true:

1. Timed rehearsal completes under 3 minutes.
2. One fallback scenario has been exercised end to end.
3. Startup and smoke verification steps are repeatable.
4. Hard stop conditions and reset steps are documented.
5. Evidence artifacts from a successful rehearsal are saved and easy to locate.
6. The demo clearly shows healthy baseline, incident detection, AI reasoning, approval, execution, validation, and final summary.
7. The Phase 12 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are supported by rehearsal evidence.

---

## 9) Outcome of this phase

When Phase 13 is complete, PulseOps AI will be ready not just as a working project but as a dependable hackathon demonstration. The team will have a timed script, rehearsed fallbacks, repeatable startup and reset procedures, and a clean evidence trail that supports the final video, repository, and submission narrative.