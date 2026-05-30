# PulseOps AI Phase 12 Detailed Plan

Phase: 12 - Dashboard polish and demo readiness  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Turn the functional MVP into a clear, compelling, demo-ready dashboard experience.

Phases 3 through 11 build the operational flow: live telemetry, incident lifecycle, AI investigation, approval, remediation execution, recovery validation, and final summary. Phase 12 makes that flow legible under demo conditions by reducing noise, strengthening visual hierarchy, and ensuring the entire story is visible and understandable on one screen.

At the end of Phase 12:
- the dashboard shows the full incident flow in a coherent layout
- state transitions are visually obvious in real time
- important evidence is visible without overwhelming the user
- the interface is easy to narrate during a 3-minute demo
- the UX supports both healthy baseline and failure/recovery transitions cleanly

---

## 2) Rule-aware constraints for Phase 12

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Judging alignment
- The dashboard directly affects the Design criterion.
- The interface should also reinforce Technological Implementation by making Agent Builder, Gemini, Elastic evidence, and endpoint action understandable at a glance.
- The UI should support the project’s real-world impact story, not just look polished.

2. Functional submission requirement
- The dashboard must reflect real backend state, not scripted frontend-only mock behavior.
- The demo UI should work reliably in the hosted version and in local rehearsal.
- The flow shown in the UI must match the actual system behavior depicted in the demo video.

3. Stack compliance
- Keep the existing web stack and architecture intact.
- Do not introduce unrelated frameworks or third-party services just for visual polish.
- Preserve the hackathon-required story: Gemini reasoning, Agent Builder orchestration, Elastic context, human approval, real endpoint action.

4. Demo readiness
- The full operator story should fit on one screen when practical.
- High-value changes should be obvious in less than a second: healthy, incident detected, investigation ready, approved, executing, validating, resolved.
- The UI must remain readable on a laptop during screen recording.

5. Phase boundary
- Phase 12 improves presentation, layout, clarity, and demo ergonomics.
- It should not add new backend workflow semantics unless a small missing UX support field is required.
- Phase 13 will handle rehearsal scripts, fallbacks, and demo failure-proofing in detail.

---

## 3) Phase 12 definition of done

Phase 12 is complete only when all are true:

1. Full workflow is visible from one main dashboard experience.
2. Lifecycle states are color-coded and visually distinct.
3. Real-time status changes are easy to follow during incident progression.
4. AI investigation, approval, execution, validation, and final summary sections are all readable.
5. Key evidence is visible in compact form without flooding the page.
6. Healthy baseline and recovered state are visually distinct from incident/failure states.
7. Dashboard is usable for live demo narration on desktop and acceptable on narrower screens.
8. Phase 12 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are supported by rehearsal readiness.

---

## 4) Work breakdown structure

### 4.1 Dashboard information architecture

Goal: organize the screen around the incident story judges need to understand quickly.

Recommended top-level sections:
- live endpoint health
- incident timeline and current lifecycle state
- AI investigation and recommended actions
- human approval and execution status
- recovery validation
- final summary

Tasks:
1. Replace placeholder-only layout with a narrative-first layout.
2. Order panels so they match the demo flow from top to bottom or left to right.
3. Keep the most important current-state information above the fold.
4. Ensure the dashboard still communicates value when no active incident exists.

Output:
- one screen presents the full detect -> investigate -> remediate -> validate -> summarize story.

---

### 4.2 Primary hero state and health panel

Goal: make the current operational state instantly obvious.

Tasks:
1. Elevate the endpoint health card into a primary hero panel.
2. Show at minimum:
- device identity
- monitored service name
- service status
- heartbeat/connectivity state
- last telemetry timestamp
3. Add strong state treatment for:
- healthy
- degraded
- stopped
- recovering
- resolved
4. Make the before/after state easy to compare during narration.

Output:
- viewers can immediately tell whether the endpoint is healthy or in trouble.

---

### 4.3 Incident timeline and lifecycle strip

Goal: visualize how the incident moved through the system.

Tasks:
1. Add a compact lifecycle strip or timeline for states such as:
- detected
- investigating
- awaiting approval
- approved
- executing
- validating
- resolved or failed
2. Highlight the current state and completed states clearly.
3. Attach timestamps or compact labels where useful.
4. Keep the component readable in screen recordings without tiny text.

Output:
- the dashboard communicates process progression without verbal explanation alone.

---

### 4.4 AI investigation presentation

Goal: make the AI contribution visible, credible, and compact.

Tasks:
1. Present:
- probable cause
- confidence
- recommended actions
- validation steps
2. Add a confidence/rationale treatment that is readable but not flashy.
3. Show a small amount of supporting evidence, such as key log or telemetry snippets.
4. Distinguish AI recommendation content from operator approval and system execution status.

Output:
- judges can see where Gemini and Agent Builder added value.

---

### 4.5 Approval and execution visualization

Goal: make the human-in-the-loop control and the real endpoint action easy to follow.

Tasks:
1. Place approval controls and execution status near the recommendation section.
2. Show approval metadata clearly:
- approved by
- approved at
3. Show execution progress and result with compact per-action status.
4. Make it obvious that execution happens only after approval.

Output:
- the governance and action layers read as one coherent sequence.

---

### 4.6 Recovery validation and final summary presentation

Goal: close the loop visually after remediation completes.

Tasks:
1. Show validation progress clearly while the system is confirming recovery.
2. Differentiate `validating` from `resolved` and `failed` with unmistakable styling.
3. Render the final summary in a concise, screen-friendly format.
4. Keep closure artifacts visible enough that the dashboard ending feels complete.

Output:
- the dashboard has a satisfying and understandable end state for the incident story.

---

### 4.7 Compact evidence strategy

Goal: show meaningful logs and telemetry without drowning the user in raw data.

Tasks:
1. Choose a small number of evidence elements to surface by default, for example:
- latest telemetry snapshot
- one or two key logs
- one validation proof point
2. Truncate long text safely.
3. Use compact labels and badges instead of verbose prose where possible.
4. Preserve full detail for backend logs and artifacts rather than the main UI.

Output:
- the UI feels informative, not cluttered.

---

### 4.8 Real-time transition polish

Goal: make state updates legible in motion during a live demo.

Tasks:
1. Add clear transition handling for websocket-driven updates.
2. Avoid jarring layout shifts as cards update.
3. Use restrained motion to emphasize change, for example:
- status pulse
- subtle highlight on new state
- gentle section reveal when new incident data appears
4. Keep transitions reliable and lightweight.

Output:
- viewers can follow system changes as they happen.

---

### 4.9 Visual system and styling cleanup

Goal: give the dashboard a consistent, intentional visual language.

Tasks:
1. Standardize color usage for lifecycle states.
2. Define consistent badge, panel, spacing, and typography rules.
3. Reduce placeholder-looking sections and generic empty states.
4. Ensure contrast and readability remain strong in screen recordings.
5. Keep styling aligned with the existing codebase, but push beyond a bare scaffold.

Output:
- the interface looks deliberate and presentation-ready.

---

### 4.10 Healthy baseline, incident, and recovery comparison

Goal: help judges quickly understand what changed during the demo.

Tasks:
1. Make the healthy baseline visually calm and stable.
2. Make incident state visibly urgent without becoming noisy.
3. Make recovery/resolution feel clearly different from both healthy idle and active failure.
4. Preserve some visual memory of the incident path so the narrative does not disappear immediately after recovery.

Output:
- the before/failure/after progression is easy to understand from visuals alone.

---

### 4.11 Responsive and recording-friendly layout checks

Goal: ensure the dashboard works well in likely demo capture conditions.

Tasks:
1. Optimize for laptop-width desktop first.
2. Confirm layout still works on narrower widths without breaking section order.
3. Avoid tiny fonts, dense tables, or horizontal scrolling in core panels.
4. Consider screen recording crop and browser chrome when sizing key content.

Output:
- the dashboard remains readable in real demo environments.

---

### 4.12 Rehearsal-oriented UX refinements

Goal: support smooth narration and operator control during the demo.

Tasks:
1. Remove low-value noise from the main screen.
2. Ensure key controls and state badges are easy to find quickly.
3. Add minimal helper text only where it reduces confusion for first-time viewers.
4. Keep the story sequence obvious enough that a presenter can narrate without hunting for information.

Output:
- the dashboard becomes a presentation tool, not just a developer console.

---

### 4.13 Final polish verification checklist

Goal: define a concrete finish line for UI readiness.

Tasks:
1. Verify one-screen visibility of the main workflow.
2. Verify state color-coding consistency.
3. Verify websocket updates do not produce confusing jumps.
4. Verify summary and evidence remain readable during and after incident resolution.
5. Verify the dashboard still makes sense when no active incident is present.

Output:
- Phase 12 ends with a repeatable visual quality gate rather than subjective “looks good” judgment.

---

## 5) Recommended implementation order

Implement Phase 12 in this order:

1. Rework dashboard information architecture and section ordering.
2. Promote endpoint health and lifecycle state to the top of the page.
3. Refine investigation, approval, execution, validation, and summary panels.
4. Add compact evidence treatment and remove noise.
5. Apply consistent state colors, spacing, and typography.
6. Add restrained real-time transition polish.
7. Run recording-friendly and narrower-width layout checks.
8. Rehearse the full story on the polished dashboard.

This order fixes comprehension first, then styling, then motion and demo ergonomics.

---

## 6) File-by-file implementation map

Expected frontend touch points:
- `frontend/src/pages/DashboardPage.tsx` for overall layout and section ordering
- `frontend/src/components/StatusCard.tsx` for card-level presentation refinement
- `frontend/src/components/` for extracting lifecycle, investigation, approval, execution, validation, and summary components if needed
- `frontend/src/types/` for any UI-specific display metadata
- `frontend/src/App.css` for page-level styling, grid, and state treatments
- `frontend/src/index.css` if global typography or layout variables need cleanup

Potential backend touch points only if required for display ergonomics:
- incident or telemetry DTOs that need one or two extra UI-facing derived fields
- websocket payloads if a compact status field is missing

Expected docs/scripts touch points:
- demo runbook docs if the polished dashboard changes the presenter flow
- screenshot/artifact capture notes if the UI becomes part of submission assets

---

## 7) Pitfalls to avoid

1. Do not polish placeholders instead of the real flow.
- The dashboard must showcase real incident data and state transitions.

2. Do not add visual noise to look “feature rich.”
- Too many panels, dense labels, or large logs will hurt demo clarity.

3. Do not hide the required technology story.
- Judges should be able to spot AI reasoning, approval, real execution, and recovery proof without a long explanation.

4. Do not over-animate.
- Motion should clarify state change, not distract from it.

5. Do not design only for a perfect wide monitor.
- The likely demo environment is a laptop screen and recorded browser window.

6. Do not let recovery instantly erase the incident narrative.
- The dashboard should preserve enough context to explain what just happened.

---

## 8) Acceptance gate for moving to Phase 13

Do not start Phase 13 until all checks below are true:

1. Main dashboard shows the complete workflow clearly on one primary screen.
2. Core states are color-coded and easy to distinguish.
3. Real-time transitions are legible during incident progression.
4. Evidence, approval, execution, validation, and summary sections are readable without clutter.
5. The dashboard is suitable for screen recording and live narration.
6. The Phase 12 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are supported by rehearsal results.

---

## 9) Outcome of this phase

When Phase 12 is complete, PulseOps AI will not just work end to end; it will present that workflow in a way judges can understand quickly and remember. The dashboard will make the system’s technical depth visible, the human approval boundary obvious, and the recovery story easy to narrate, which is critical for both judging impact and the final demo video.