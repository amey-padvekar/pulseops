# Phase 11 — Final Summary Panel: Manual Verification Checklist

The frontend has no automated UI test harness (no Vitest/RTL configured), so the final
summary panel is verified through the production build plus this manual checklist
(step 4.11 task 3).

## Automated gates (run before manual checks)

```bash
cd frontend
npx tsc --noEmit     # type-check passes
npm run lint         # no new errors in FinalSummaryPanel.tsx / dashboard.ts
npm run build        # production build succeeds
```

## Manual checklist

Render the dashboard against a backend with at least one completed incident.

### States (step 4.8 task 3)
- [ ] **Idle** — for an endpoint with no terminal incident, the panel shows the
      "A final report is generated once the incident is resolved or failed…" placeholder.
- [ ] **Pending** — immediately after an incident resolves/fails (before the report lands),
      the panel shows the pulsing "Generating final incident report…" state.
- [ ] **Ready (resolved)** — once generated, root cause, evidence list, actions taken, and a
      green ✓ result are shown; the chip reads "incident resolved".
- [ ] **Ready (failed)** — for a failed incident, the result is a red ✕ line that does NOT
      claim recovery; the chip reads "incident failed".
- [ ] **Fallback** — when the live AI summary is unavailable, an amber banner reads
      "Auto-generated from the incident record…", and the structured report still renders.
- [ ] **Failed (no summary)** — if generation failed without a stored fallback, the panel
      shows the "could not be generated" note pointing to the Validation/Execution panels.

### Content (step 4.8 task 2)
- [ ] Optional `operatorSummary` renders as the highlighted lead sentence when present.
- [ ] `rootCause` renders as a single readable sentence.
- [ ] `evidence` renders as a bulleted list (no raw log dumps).
- [ ] `actionsTaken` renders as a bulleted list; shows "No remediation actions were
      executed." when empty.
- [ ] `result` matches the incident outcome (resolved vs failed) and is color-coded.

### Persistence (step 4.8 / definition-of-done #6)
- [ ] Reload the page — the generated summary is still shown (served from the stored
      incident record, not regenerated).

### Copy/export (step 4.9)
- [ ] The **Copy** button appears only in the ready state.
- [ ] Clicking it copies clean plain text (header, optional operator summary, root cause,
      bulleted evidence, bulleted actions, result, generated timestamp) and flips to
      "✓ Copied" for ~2s.
- [ ] Pasted text is readable in a plain-text target (notes, chat, issue description).
- [ ] The rendered body is selectable for partial copy / screenshots.

### Layout (step 4.8 task 4)
- [ ] The panel sits in the same dashboard grid as the other status cards and reads
      clearly on one screen without overflowing.
