# PulseOps AI — Phase 12 UI Verification Checklist

Phase: 12 — Dashboard polish and demo readiness
Purpose: a **repeatable visual quality gate** for the dashboard, so UI readiness is a
pass/fail checklist rather than a subjective "looks good" judgment (Phase 12 step 4.13).

Run this checklist before recording the demo and after any dashboard change. Each
item lists how to reproduce the state, what to look for, and the pass bar.

---

## 0) Setup

Bring the stack up per [docs/DEPLOYMENT_AND_ENVIRONMENT.md](DEPLOYMENT_AND_ENVIRONMENT.md):

1. Start the backend, then the agent (it heartbeats telemetry for the monitored device).
2. Start the frontend (`npm run dev` in `frontend/`) and open the dashboard.
3. Confirm the build is clean first:
   - `cd frontend && npx tsc --noEmit && npx vite build` → no errors.

Recommended capture conditions (matches likely demo): laptop display, browser window
≈ 1280×720 visible viewport after chrome, default page zoom (100%).

Reproduce the incident flow by inducing a monitored-service stop on the agent host,
then approving remediation in the dashboard, and letting recovery validate. This drives
the lifecycle: `detected → investigating → awaiting_approval → approved → executing →
validating → resolved` (or `failed`).

---

## 1) One-screen visibility of the main workflow

How to verify:
- Load the dashboard at the capture viewport with no active incident, then during a live
  incident.

Pass when:
- [ ] Above the fold (no scroll) shows: header, the **mood banner**, the control bar, and
      the **Endpoint Health hero**. The current operational state is readable without
      scrolling.
- [ ] The complete six-stage workflow (Health → Incident → AI Investigation → Approval &
      Execution → Recovery Validation → Final Summary) is present in narrative order and
      reachable with a single, smooth vertical scroll — no horizontal scroll, no hidden
      panels.
- [ ] Stage numbers (1–6) and flow verbs (Monitor / Detect / Investigate / Approve → Act /
      Validate / Resolve) are visible so a presenter can narrate top-to-bottom.

Notes:
- "One screen" is interpreted per the Phase 12 plan ("when practical"): the *current
  most-relevant state* is above the fold; the full flow is one scroll. A 6-stage flow does
  not fit a single laptop viewport during an active incident, and that is acceptable.

---

## 2) State color-coding consistency

How to verify:
- Walk the incident through each lifecycle state and confirm the colour language is
  identical everywhere a given state appears (lifecycle strip node, hero, panel chips,
  mood banner, outcome banners).

Colour language (single source: tokens in [frontend/src/index.css](../frontend/src/index.css) `:root`):
- healthy / success / resolved / pass → **green** (`--state-ok`)
- in-progress / validating / pending / awaiting → **amber** (`--state-warn`)
- recovering / current step / live → **blue** (`--state-info`)
- AI analysis → **indigo** (`--state-ai`)
- stopped / failed → **red** (`--state-danger`)
- idle / neutral → **slate** (`--state-neutral`)

Pass when:
- [ ] The same state never shows two different colours across panels (e.g. "resolved" is
      green in the hero, the lifecycle strip, the mood banner, and the summary).
- [ ] Repeatable code check passes — no off-palette state colours remain:
      ```
      cd frontend/src
      grep -nE "#b00020|#166534|#d97706" App.css   # → no matches
      ```
- [ ] Badges (`running`/`stopped`/`degraded`/`unknown`) and accents resolve to the token
      palette, not ad-hoc hexes.

---

## 3) Websocket updates do not produce confusing jumps

How to verify:
- Watch the dashboard while the backend pushes live `incident.updated` / `telemetry.updated`
  events through the full flow.

Pass when:
- [ ] On each update the affected stage(s) **briefly highlight** (outline glow) and the
      current lifecycle node pulses — the change is noticeable.
- [ ] No layout jump from the highlight itself (it uses `outline`, which does not affect
      layout) — cards do not shift position when they flash.
- [ ] Panels never reorder; stages stay in fixed narrative positions as data arrives.
- [ ] Colour/shadow state changes ease in (transition) rather than snapping.
- [ ] With OS "reduce motion" enabled, animations are disabled but all state remains
      legible (no reliance on motion to convey state).

---

## 4) Summary and evidence remain readable during and after resolution

How to verify:
- Complete a full incident to `resolved` (and separately to `failed`), then keep watching
  after the incident goes inactive.

Pass when:
- [ ] During the flow: AI investigation (probable cause, confidence meter, recommended
      actions, validation steps, supporting evidence) is readable and not clipped.
- [ ] After resolution (incident `active=false`): the AI investigation, lifecycle strip,
      recovery validation, and **final summary** stay populated — the story is not erased.
      (All closure panels read from the persisted active-or-latest incident.)
- [ ] The final summary renders root cause, evidence, actions, and the result banner; the
      **Copy** button produces clean plain text.
- [ ] Long command output is compact (truncated with an "output truncated" marker) and the
      panel notes that full detail lives in the backend — no wall of raw logs.

---

## 5) Dashboard makes sense with no active incident

How to verify:
- Load / return the dashboard to a healthy baseline (no active or recent incident).

Pass when:
- [ ] Mood banner reads the calm **baseline** state ("All systems healthy"), visually
      distinct (cool slate) from the green recovered state.
- [ ] The hero shows the live healthy endpoint with the telemetry snapshot (CPU/memory,
      connectivity, last telemetry).
- [ ] Downstream panels show **intentional empty states** (tinted "waiting" affordances),
      not broken or blank boxes — e.g. "Investigation not started", "No AI recommendation
      to review yet", "No remediation executed yet".
- [ ] Nothing on screen looks like an error or missing data when simply idle.

---

## Gate result

Phase 12 UI is **ready** only when every box above is checked at the capture viewport.
Record the date and the commit verified:

- Verified on: __________  •  commit: __________  •  by: __________

Last code-level verification (static, pre-rehearsal):
- Build/typecheck clean: ✅
- Color-coding consistency (no off-palette state hexes): ✅ (`grep` returns no matches)
- Closure panels read from persisted incident (persist after resolution): ✅
- Flash highlight uses non-layout `outline`; reduced-motion fallback present: ✅
- Intentional empty states for the no-incident baseline: ✅

Live visual items (one-screen fit, motion legibility, readability in recording) require a
running stack and are checked during Phase 13 rehearsal.
