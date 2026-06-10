# Demo Mode — Judge-Driven "Simulate Service Failure" Panel

## Context

For hackathon judging we want a demo that is **reliable, interactive, and visibly compliant**, without misrepresenting anything. The full loop already works: detect → **real Gemini + Agent Builder + Elastic MCP** diagnose → approve → remediate → validate → resolve. The risk is that a live demo depends on a VM, a service, an agent, networking, and timing — something always breaks on stage.

This feature adds a `DEMO_MODE`-gated panel where **a judge picks the selected device, picks a service scenario, and clicks "Simulate Service Failure"**, then watches that incident animate through its whole lifecycle live over the existing WebSocket — driven by the **real** AI investigation, with the failure *source* and the *execution* simulated server-side so it always resolves. No endpoint agent required.

Design facts that shape this (verified in code):

- Detection fires only on `serviceStatus == "stopped"` **and** `heartbeat == true` — [detector.go:24-41](../backend/internal/incidents/detector.go#L24-L41), `EvaluateTelemetry`. We stay on this proven trigger; **variety comes from which service the incident names**, not new detection types.
- The agent is **Windows-only** and the remediation catalog is exactly three actions — `restart_service`, `flush_dns`, `reconnect_vpn` ([catalog.go:10-20](../backend/internal/remediation/catalog.go#L10-L20)); the Windows restart is `Restart-Service -Name <serviceName> -Force` ([remediation.go:61-66](../agent/internal/platform/remediation.go#L61-L66)). **Scenarios therefore name only services that actually exist on the demo VM**, so synthetic telemetry is honest and a real restart would succeed. See Part A.
- `/telemetry` needs **no auth** and accepts **any `deviceId`** — [main.go:65-246](../backend/cmd/server/main.go#L65-L246), `makeTelemetryHandler`. So synthetic telemetry drives the *real* pipeline.
- Every state change already broadcasts over WebSocket via `publishIncidentUpdate` → `ws.BroadcastIncidentUpdated` ([events.go:35-38](../backend/internal/ws/events.go#L35-L38)); the dashboard re-renders live through `useIncidents` ([useIncidents.ts](../frontend/src/hooks/useIncidents.ts)). "Real time" needs **no new transport** — only paced server-side transitions.
- The lifecycle can be driven without an agent using existing store methods: `CreateOrGetActive` ([store.go:40-79](../backend/internal/incidents/store.go#L40-L79)), `MarkExecuting` ([store.go:321-345](../backend/internal/incidents/store.go#L321-L345)), `SaveRemediationResult` ([store.go:356-422](../backend/internal/incidents/store.go#L356-L422), `succeeded` → `validating`), validation via `processTelemetryValidation` → `RecordValidationObservation` ([validation.go:42-106](../backend/internal/incidents/validation.go#L42-L106), 2 healthy cycles → `resolved`), and `Delete` ([store.go:195-210](../backend/internal/incidents/store.go#L195-L210)).
- The `AGENT_BUILDER_FALLBACK_MODE=local_stub_actions` block ([main.go:575-649](../backend/cmd/server/main.go#L575-L649)) already shows the pattern for seeding an investigation result and promoting to approval **without** a live agent — the template the demo lifecycle mirrors (while still using the **real** agent for diagnosis).
- A closing report already exists: `FinalSummary {RootCause, Evidence[], ActionsTaken[], Result, OperatorSummary}` (Phase 11), generated on resolution and already present on the frontend `Incident` type ([dashboard.ts:37-93](../frontend/src/types/dashboard.ts#L37-L93)) — so "Download Report" reuses data that already flows in.

**Everything here is additive and gated by `DEMO_MODE`. The normal (non-demo) flow is never altered.**

---

## What ships

- **Core panel:** a `DEMO_MODE`-gated "Simulate Service Failure" control that creates a real service-stopped incident on the **selected device** and self-drives it to **resolved** with paced, live broadcasts. Approval stays a **real, mandatory** gate (auto-approve is an opt-in toggle, **OFF by default**). Default scenario = **Endpoint Security (Microsoft Defender / `WinDefend`)**; **`Spooler`** is the real-agent remediation proof.
- **Enhancement 1 — "Root Cause Identified" step:** a display-only timeline node that lights up the moment Gemini's diagnosis lands (no backend state change).
- **Enhancement 2 — Gemini / Agent Builder / Elastic MCP badges:** make the orchestration layer visible in the investigation UI and timeline.
- **Enhancement 3 — Download Report:** client-side incident summary export built from the existing `FinalSummary` + incident fields.

---

## Part A — Service scenarios (demo guidance; real services only)

Detection stays service-stopped. Variety = **which real service** the incident names; Gemini produces a distinct diagnosis per service from its name + scenario-flavored `RecentLogs`. **Every scenario names a service that actually exists on the Windows demo device**, so the synthetic telemetry is honest and a real agent's `Restart-Service` would succeed. VPN is **dropped** (no VPN service installed). "EDR" is rendered **honestly** via Microsoft Defender's real service, not a fictional `EDRService`.

| Scenario (label) | Real Windows service (short name) | Remediation | Story |
|---|---|---|---|
| **Endpoint Security — Microsoft Defender** (default narrative) | `WinDefend` | **Simulated** (Defender tamper protection may block a real restart) | Defender AV stopped → AI restores endpoint protection. Strongest, Elastic-friendly security story. |
| **Database — MySQL** | `MySQL` / `MySQL80` (confirm exact short name via `Get-Service`) | Real, if safe to bounce on the demo VM | Data tier down → AI restarts MySQL. |
| **Web Server — IIS** | `W3SVC` | Real, if IIS installed | Site returning 503s → AI restarts IIS. |
| **Print Spooler** (real-agent proof) | `Spooler` | **Real & safe** on every Windows box | The designated live-agent restart proof for the recorded demo. |

- All four map to the single catalog action **`restart_service <serviceName>`** ([catalog.go:10-20](../backend/internal/remediation/catalog.go#L10-L20)); the demo seeds a *simulated* `restart_service` result, so execution always resolves regardless of the service.
- The scenario catalog is the single source of truth in **[backend/internal/demo/scenarios.go](../backend/internal/demo/scenarios.go)** (serviceName + mode + scenario-flavored event logs) with a frontend mirror in **[frontend/src/data/demoScenarios.ts](../frontend/src/data/demoScenarios.ts)** (`{ key, label, serviceName, mode }[]` for the dropdown). Correct exact short names (e.g. `MySQL80`) to match the box — confirm each with `Get-Service` — and drop any service you don't have.
- No detection or remediation code changes for Part A — only the scenario→`serviceName`/logs mapping (echoed into the synthetic telemetry the backend builds).

> **Status — Part A implemented.** Catalog (`WinDefend` / `MySQL80` / `W3SVC` / `Spooler`, default `defender`) + flavored event-logs in `backend/internal/demo/scenarios.go` (+ `scenarios_test.go`, green); frontend mirror `DEMO_SCENARIOS` in `frontend/src/data/demoScenarios.ts` (`tsc -b` clean). Parts B/C import these; nothing else is wired yet.

---

## Part B — Backend (`backend/`)

All demo code is additive and only wired when `DEMO_MODE` is true.

### B1. Config gate — `DEMO_MODE`

Add a `DEMO_MODE` bool following the existing env-bool idiom at [config.go:89-97](../backend/internal/agentbuilder/config.go#L89-L97) (`switch` on lowercased/trimmed value: `true|1|yes` → true, else false). Read it in `main()` near the other config loads ([main.go:738-805](../backend/cmd/server/main.go#L738-L805)). Register `/demo/*` routes only when true (see B3/B5).

> **Status — B1 implemented.** `DemoMode` added to `Config` + parsed in `loadConfig()` (env-bool idiom, default off), logged at startup (`demo_mode=%t`), and a gated `if cfg.DemoMode { … } else { … }` block sits at the route-registration site where B3/B5 plug in the `/demo/*` handlers. `DEMO_MODE=false` documented in `backend/.env.example`; parsing covered by `TestLoadConfigDemoMode` (10 cases, green).

### B2. Refactor for reuse — `ingestTelemetrySample` (no behavior change)

Extract the body of `makeTelemetryHandler` *after* parse/validate — the pipeline at [main.go:99-141](../backend/cmd/server/main.go#L99-L141): `Upsert` → `processTelemetryIncident` → `publishIncidentUpdate` → `go submitAgentBuilderRequest` → `processTelemetryValidation` (+ its broadcasts/summary trigger) → `BroadcastTelemetryUpdated` — into:

```go
func ingestTelemetrySample(
    state store.DeviceState,
    deviceStore *store.DeviceStore, incidentStore *incidents.Store, hub *ws.Hub,
    elasticClient elastic.Indexer, elasticCfg *elastic.Config,
    agentClient agentbuilder.Client, agentCfg *agentbuilder.Config,
)
```

The real handler keeps its HTTP concerns (method check, JSON decode/validate, **response writing**, and the Elastic *raw-telemetry* indexing closure) and calls `ingestTelemetrySample` for the shared core. The demo endpoints call the same function → **identical pipeline, including the real Gemini + Elastic MCP handoff.** (Note: response writing and the raw-telemetry index goroutine stay in the handler; the helper does the incident pipeline + broadcasts only.)

> **Status — B2 implemented.** `ingestTelemetrySample` extracted in [main.go](../backend/cmd/server/main.go); the `/telemetry` handler now calls it and keeps only HTTP concerns + the raw-telemetry Elastic indexing (under a thin `if ok`). One refinement vs. the sketch: the helper returns `(store.DeviceState, bool)` — the stamped state the handler needs for indexing and that B3/B4 reuse for the healthy validation samples. Behavior preserved (existing telemetry tests pass); new `TestIngestTelemetrySample*` cover incident-open on stopped+heartbeat and no-incident on healthy. Full backend suite green.

### B3. `POST /demo/incident` — Simulate Service Failure

Body: `{ deviceId, serviceName, scenario?, autoApprove? }`. **`deviceId` is the dashboard's currently-selected device** (sent by the frontend).

- Build a synthetic **stopped** `store.DeviceState` for that device: `ServiceStatus="stopped"`, `Heartbeat=true`, `NetworkReachable=true`, `ServiceName=serviceName`, scenario-flavored `RecentLogs` (from `demo.ByKey(scenario).RecentLogs(now)` — [scenarios.go](../backend/internal/demo/scenarios.go)). (`serviceName` is a **real** Windows short name from Part A — `WinDefend`, `Spooler`, `W3SVC`, or `MySQL80`.)
- Call `ingestTelemetrySample(...)` → real detection ([detector.go:24-41](../backend/internal/incidents/detector.go#L24-L41)) creates the incident (`detected` → `investigating`) → `go submitAgentBuilderRequest` ([main.go:466-724](../backend/cmd/server/main.go#L466-L724)) runs the **real Gemini + Elastic MCP** investigation → `SaveInvestigationResult` → `PromoteToAwaitingApproval`.
- Resolve the incident ID for this device/service (via the store), spawn `runDemoLifecycle(incidentID, autoApprove)` (B4), and return `{ incidentId }`.

Each step broadcasts live through the existing `publishIncidentUpdate`.

> **Status — B3 implemented.** `POST /demo/incident` in [demo.go](../backend/cmd/server/demo.go) (`makeDemoIncidentHandler` + `activeIncidentForService`), wired into the `DEMO_MODE` gate in [main.go](../backend/cmd/server/main.go). Body `{ deviceId, serviceName?, scenario?, autoApprove? }`: resolves the Part A scenario (default `WinDefend`; explicit `serviceName` overrides), builds the synthetic stopped sample, runs `ingestTelemetrySample` (real detection → `investigating` → async Gemini/MCP handoff), and returns `{ incidentId, deviceId, serviceName, scenario, state }` (201). The `runDemoLifecycle` spawn is deferred to B4 (seam marked in the handler). Tests `TestDemoIncident*` (create / default / override / validation) green; full backend suite green. Note: reaching `awaiting_approval` needs a real agent (`agent_engine`) or `AGENT_BUILDER_FALLBACK_MODE=local_stub_actions`; with neither, the incident rests in `investigating`.

### B4. `runDemoLifecycle(incidentID, autoApprove)` — the "watch it resolve" goroutine

Advances the incident with small paced delays (≈1–2s/stage) so transitions are visible, broadcasting after each store mutation via `publishIncidentUpdate`:

1. **Wait for investigation** — poll `incidentStore.GetByID` until `state == awaiting_approval` (the real Gemini result has landed), with a timeout safeguard.
2. **Approval (governance gate, kept real):**
   - **Default (`autoApprove=false`):** do nothing — wait for the judge to click **Approve** in the existing `RemediationApprovalCard` ([RemediationApprovalCard.tsx:215-222](../frontend/src/components/RemediationApprovalCard.tsx#L215-L222)); poll until `state == approved`.
   - **`autoApprove=true` (opt-in):** call the same approval store method the `/incidents/{id}/approve` handler uses, selecting all `RecommendedActions` IDs with `approvedBy="demo-auto-approve"`, then broadcast.
3. **Execute (simulated):** `MarkExecuting` ([store.go:321-345](../backend/internal/incidents/store.go#L321-L345)) → `executing`; broadcast. Pause.
4. **Remediation result (simulated):** `SaveRemediationResult(id, outcome, now, StateValidating)` with a seeded `ExecutionOutcome` ([remediation_result.go:9-28](../backend/internal/incidents/remediation_result.go#L9-L28)) — one `restart_service` `RemediationActionResult{Status: succeeded, ExitCode: 0}`. `succeeded` → `validating`. Broadcast. Pause.
5. **Validate & resolve (real validation path):** feed **two healthy** samples through `ingestTelemetrySample` (`ServiceStatus="running"`, `Heartbeat=true`, `NetworkReachable=true`) with a pause between. `processTelemetryValidation` → `RecordValidationObservation` increments `HealthyCycleCount`; at the default 2 cycles → `resolved`, which fires `triggerSummaryGeneration` → `FinalSummary` (powering Download Report). Each sample broadcasts inside the helper.

Mirrors the `local_stub_actions` seeding shapes ([main.go:575-649](../backend/cmd/server/main.go#L575-L649)) but uses the **real** agent for diagnosis. (Optional pacing knob: set `RequiredHealthyCycles=1` for a snappier validate; default 2 reads better as it shows the counter advancing.)

> **Status — B4 implemented.** `runLifecycle` on `demoController` in [demo.go](../backend/cmd/server/demo.go): waits for `awaiting_approval` → approval gate (auto-approve via `incidentStore.Approve` with all recommended action IDs, or poll until the judge approves) → `MarkExecuting` → `SaveRemediationResult(succeeded → validating)` with a seeded `restart_service` outcome → feeds `RequiredHealthyCycles` healthy "running" samples through `ingestTelemetrySample` → `resolved` + closing summary; broadcasts after each step. Pacing is injectable (`demoPacing`), and the B3 handler now spawns it (`go c.runLifecycle`). The freshness boundary is stamped ~2s in the past so the healthy samples are admitted regardless of clock granularity. Tests: `TestRunLifecycleAutoApproveResolves` (gate → resolved, validation 2/2, remediation recorded, `approvedBy=demo-auto-approve`) and `TestRunLifecycleManualGateWaitsWithoutApproval` (no approval → stays at the gate). Full backend suite green.

### B5. `POST /demo/reset`

Body `{ deviceId }`: clear that device's demo incidents via `incidentStore.Delete` ([store.go:195-210](../backend/internal/incidents/store.go#L195-L210)) so judges can re-run. (Delete also clears the dedupe mapping, allowing a fresh incident.)

> **Status — B5 implemented.** `demoController.resetHandler` in [demo.go](../backend/cmd/server/demo.go): body `{ deviceId }` hard-deletes every incident for the device (active + historical) and returns `{ deviceId, deleted, incidentIds }`. There is no removal WebSocket event (matching the existing `DELETE /incidents/{id}` dismiss path), so the frontend Reset clears those IDs locally / refetches. Tests `TestDemoResetDeletesDeviceIncidents` (deletes target, leaves other devices, dedupe cleared so a re-run opens fresh) and `TestDemoResetRequiresDeviceID`.
>
> **Bug fix (surfaced by B5).** Incident IDs were a nanosecond-formatted timestamp in `CreateOrGetActive` ([store.go:51-54](../backend/internal/incidents/store.go#L51-L54)); on coarse clocks (Windows) two incidents created in the same tick collided and silently overwrote each other in `byID`. Added a suffix-on-collision guard + `TestCreateOrGetActiveAssignsUniqueIDs` (50 rapid creates → 50 unique IDs). Latent correctness fix beyond the demo.

### B6. Coexistence with a real agent

The simulated execution is authoritative for demo-generated incidents. If a real agent also sits on the selected device, its periodic `running` telemetry is ignored while the incident is pre-`validating` (detection acts only on `stopped`; validation only on `validating`), and `MarkExecuting` is idempotent so a stray poll can't double-drive the lifecycle. **Caveat:** approving an incident enqueues a real remediation command; on a device with a *live* agent the agent would poll and actually run `Restart-Service` (which would **fail** for `WinDefend` under tamper protection). So for demo runs, **select a device with no competing agent** — the enqueued command is then never picked up and the simulated path is authoritative. Use the **`Spooler` scenario against the real VM agent** as the separate, recorded "real remediation" proof.

> **Status — B6 verified (guarantee by construction, no new feature code).** Detection acts only on `stopped` and validation only on `validating`, so a competing agent's `running` telemetry can't disturb a pre-`validating` demo incident; `MarkExecuting` is idempotent; and the `demoController` holds **no `remediationQueue` reference**, so the auto-approve path (`incidentStore.Approve` directly) *cannot* enqueue a real command (the live-agent risk is confined to the *manual*-approve path). `runLifecycle` also bails safely if the incident is already past `awaiting_approval` (a real agent advanced it). Locked by `TestDemoCoexistenceRunningTelemetryDoesNotDisturbPreValidating`. The manual-approve + live-agent caveat stays operational: use a no-agent device.

### B7. Routes & tests

- Register conditionally in the mux block ([main.go:898-951](../backend/cmd/server/main.go#L898-L951)): `mux.HandleFunc("POST /demo/incident", ...)` and `mux.HandleFunc("POST /demo/reset", ...)` inside `if demoEnabled { ... }`; log that demo routes are active.
- Tests (idiom: `httptest` + in-memory stores, mirroring [main_test.go:18-66](../backend/cmd/server/main_test.go#L18-L66) and [store_test.go](../backend/internal/incidents/store_test.go)):
  - `POST /demo/incident` creates an incident on the given `deviceId` (assert state reaches `awaiting_approval` with the stubbed agent).
  - `runDemoLifecycle` reaches `resolved` (drive with auto-approve + a stub agent client).
  - `/demo/*` returns 404 when `DEMO_MODE` is unset.

> **Status — B7 implemented.** Routes are wired through `registerDemoRoutes(mux, enabled, …)` in [demo.go](../backend/cmd/server/demo.go) (called from `main()`), so the `/demo/*` surface only exists when `DEMO_MODE` is on. Covered by `TestDemoRoutesAbsentWhenDisabled` (404 when off) and `TestDemoRoutesPresentWhenEnabled` (405 method-mismatch = route present when on, without invoking the handler). Demo-incident and lifecycle test coverage landed with B3/B4. **Backend (Part B) is complete; full `go test ./...` green.**

---

## Part C — Frontend (`frontend/src/`)

### C1. `hooks/useDemoMode.ts` (new)
`return import.meta.env.VITE_DEMO_MODE === 'true'` — mirrors [useApiBaseUrl.ts](../frontend/src/hooks/useApiBaseUrl.ts).

> **Status — C1 implemented.** [useDemoMode.ts](../frontend/src/hooks/useDemoMode.ts) returns `import.meta.env.VITE_DEMO_MODE === 'true'`. `tsc -b` clean.

### C2. `hooks/useDemoControls.ts` (new)
`generate(deviceId, serviceName, autoApprove)` + `reset(deviceId)` — `useCallback` + `fetch` to `${apiBaseUrl}/demo/...`, with `status / error / submitting`, mirroring [useApproveIncident.ts:21-85](../frontend/src/hooks/useApproveIncident.ts#L21-L85).

> **Status — C2 implemented.** [useDemoControls.ts](../frontend/src/hooks/useDemoControls.ts) exposes `generate(deviceId, scenario, autoApprove)`, `reset(deviceId)`, `clear()`, and shared `status/error/submitting/response`. Refinement: `generate` takes the selected `DemoScenario` and posts the scenario **key** (→ correct flavored logs) *and* `serviceName`, rather than a bare serviceName. `readError` also handles the demo endpoints' plain-text `http.Error` bodies (not just JSON `{error}`). Mirrors `useApproveIncident`; `tsc -b` clean.

### C3. `components/DemoControls.tsx` (new) — "Simulate Service Failure"
Renders only when `useDemoMode()`. A `.status-card` panel showing: the **active device** (the selected one, from props), a **scenario dropdown** (the Part A real-service list imported from [demoScenarios.ts](../frontend/src/data/demoScenarios.ts) — `DEMO_SCENARIOS`, default `DEFAULT_DEMO_SCENARIO_KEY` = Endpoint Security / `WinDefend`), a primary **"Simulate Service Failure"** button, an **auto-approve** checkbox (**unchecked by default** — keeps the human approval gate front and center), a **Reset** button, and inline status. Receives `selectedDeviceId` as a prop. For `mode: 'simulated'` scenarios (Defender) the panel notes it runs the simulated execution path.

> **Status — C3 implemented.** [DemoControls.tsx](../frontend/src/components/DemoControls.tsx) — `.status-card` panel gated on `useDemoMode()`: active device, `DEMO_SCENARIOS` dropdown (default Defender / `WinDefend`), **Simulate Service Failure** + **Reset** buttons, **auto-approve off by default** (with copy explaining the governance gate), a simulated-path note for Defender, and inline error/success status (surfaces the created `incidentId`). Wired to `useDemoControls`; props also include an optional `onAfterReset`. Lints clean; `tsc -b` clean. New `demo-*` classes are styled in C8.

### C4. `pages/DashboardPage.tsx` — wiring
Render `<DemoControls selectedDeviceId={selectedDeviceId} />` between the control-bar ([DashboardPage.tsx:255-305](../frontend/src/pages/DashboardPage.tsx#L255-L305)) and the incident-list panel ([:310-357](../frontend/src/pages/DashboardPage.tsx#L310-L357)). The page already owns `selectedDeviceId` ([:168-189](../frontend/src/pages/DashboardPage.tsx#L168-L189)) and live `useIncidents`, so the generated incident animates through the existing panels in real time.

> **Status — C4 implemented.** Rendered between the control bar and the incident list in [DashboardPage.tsx](../frontend/src/pages/DashboardPage.tsx) with `selectedDeviceId` plus an `onAfterReset` that dismisses the device's incidents locally (`/demo/reset` deletes them server-side but emits no removal event; `dismissIncident` treats the resulting 404 as success). The generated incident animates through the existing panels via the live `useIncidents`. `tsc -b` clean; my change added **no** new lint errors (the 2 `react-hooks/set-state-in-effect` errors are pre-existing on unrelated device/incident-selection effects). Visible only with `VITE_DEMO_MODE=true` (C9).

### C5. Enhancement 1 — "Root Cause Identified" timeline step
Display-only, **no backend state**. In [LifecycleStrip.tsx](../frontend/src/components/LifecycleStrip.tsx) (STEPS array ~line 16), insert a synthetic node `root_cause_identified` between `investigating` and `awaiting_approval`. Its status is derived, not from `incident.state`:
- **complete** when `incident.probableCause` is set (or `investigationStatus === 'completed'`); timestamp from `investigatedAt`.
- **current** when `state === 'investigating'` and no diagnosis yet.
- Keep the rest of the strip keyed off `incident.state`; type the step key locally as `IncidentState | 'root_cause_identified'`.

This makes judges see *"the AI figured out **why**"* as its own beat.

> **Status — C5 implemented.** [LifecycleStrip.tsx](../frontend/src/components/LifecycleStrip.tsx) — inserted a display-only `root_cause_identified` node (label **"Root Cause"**) between Investigating and Awaiting Approval; the step-key type is widened to `IncidentState | 'root_cause_identified'`. Its status is derived (not index-based): **complete** once `probableCause` is set (or `investigationStatus === 'completed'`), timestamp from `investigatedAt`; **current** while `state === 'investigating'` with no diagnosis yet; otherwise **pending**. All other nodes still key off `incident.state`. `tsc -b` + ESLint clean.

### C6. Enhancement 2 — Gemini / Agent Builder / Elastic MCP visibility
[AiInvestigationPanel.tsx:~101](../frontend/src/components/AiInvestigationPanel.tsx#L101) already shows a "Gemini · Agent Builder" `card-chip`. Extend it to **"Gemini · Agent Builder · Elastic MCP"**, and add a small "Investigating via: Gemini + Agent Builder + Elastic MCP" indicator on/near the `investigating` lifecycle step (reuse `.investigation-status-chip`, [App.css:568-602](../frontend/src/App.css#L568-L602)) so the orchestration layer is visible *during* investigation, not just after.

> **Status — C6 implemented.** [AiInvestigationPanel.tsx](../frontend/src/components/AiInvestigationPanel.tsx) — the always-visible chip now reads **"Gemini · Agent Builder · Elastic MCP"**. Key finding: `investigationStatus` is only set on completion ([store.go:519-522](../backend/internal/incidents/store.go#L519-L522)) — empty *during* investigation — so the pre-completion branch was restructured: a terminal-but-not-completed status (`fallback`/`timeout`/`failed`) shows the failure block, otherwise an **"Investigating via: Gemini / Agent Builder / Elastic MCP"** indicator (three `card-chip badge-ai` pills) renders, making the orchestration layer visible *during* the investigation. `tsc -b` + ESLint clean; `ai-orchestration*` classes styled in C8.

### C7. Enhancement 3 — Download Report
Client-side only; **no backend change**. When `incident.state === 'resolved'` (or `finalSummary` present), show a **Download Report** button. Assemble a Markdown/text report and trigger a `Blob` download. Source data, all already on the `Incident` type ([dashboard.ts:37-93](../frontend/src/types/dashboard.ts#L37-L93)):
- Header: `incidentId`, `serviceName`, `deviceId`, `severity`, key timestamps.
- Root cause: `finalSummary.rootCause` ?? `probableCause`.
- Evidence: `finalSummary.evidence[]` ?? (`reason` + `summary` + last validation snapshot).
- Actions: `recommendedActions` / `approvedActions` / `remediationResults[]`.
- Result: `finalSummary.result` / `operatorSummary`, `validationStatus`.

Prefer `finalSummary` when `summaryStatus` is `generated`/`fallback`; otherwise assemble from raw fields so the export never depends on a live agent.

> **Status — C7 implemented.** [FinalSummaryPanel.tsx](../frontend/src/components/FinalSummaryPanel.tsx) — `buildReportMarkdown(incident)` prefers `finalSummary` (when `summaryStatus` is generated/fallback), else assembles root cause / evidence / actions / result from raw fields (`probableCause`, `reason`, `summary`, `remediationResults`, `validationStatus`, `lastValidationSnapshot`). A **Download Report** button Blob-downloads `incident-<id>.md`, shown at closure (resolved/failed) or whenever `finalSummary` exists, next to the existing Copy button. Never depends on a live agent. `tsc -b` + ESLint clean; button styled in C8.

### C8. `App.css`
Add `.demo-controls-panel` (on `.status-card`) and `.demo-button` (on the `.approval-button` pattern, [App.css:1413-1435](../frontend/src/App.css#L1413-L1435)), reusing existing tokens (`--color-surface`, `--radius-lg`, badge classes). A distinct accent so the panel reads as a **demo/simulation** control.

> **Status — C8 implemented.** Appended to [App.css](../frontend/src/App.css): a distinct **violet** demo accent — `.demo-controls-panel` (left border + tinted gradient over `.status-card`), `.badge-demo` pill, `.demo-button` / `-primary` / `-secondary`, plus the panel's grid / note / auto-approve / status rows. Also styled C6's `.ai-orchestration*` ("Investigating via" chip row) and C7's `.summary-actions` + `.summary-download-button` (blue accent vs. neutral Copy). Reuses `--space-*` / `--radius-*` / `--color-*` / `--state-*` tokens. Full `npm run build` (tsc + vite) green.

### C9. Env
`VITE_DEMO_MODE=true` in the hosted build; add to `frontend/.env.example`. Backend: add `DEMO_MODE=false` to [backend/.env.example](../backend/.env.example).

> **Status — C9 implemented.** `VITE_DEMO_MODE=false` added to [frontend/.env.example](../frontend/.env.example) and `VITE_DEMO_MODE=true` set in the local (gitignored) `frontend/.env`; `DEMO_MODE=false` in [backend/.env.example](../backend/.env.example) (B1) with `DEMO_MODE=true` set in the local (gitignored) `backend/.env`. Both local `.env` files carry the flags, so `go run ./cmd/server` + `npm run dev` enable the demo with no extra env exports. Both flags documented in the env table of [DEPLOYMENT_AND_ENVIRONMENT.md](DEPLOYMENT_AND_ENVIRONMENT.md). **Hosted enablement is a manual deploy step (cannot be done from here):** set `DEMO_MODE=true` on the Cloud Run backend + redeploy, and rebuild the frontend with `VITE_DEMO_MODE=true` (baked into `dist/` at build time, since Firebase serves the prebuilt bundle) + `firebase deploy`.

---

## Decisions (recommended defaults, confirmed)

- Incident is generated on the **selected device** (`deviceId` from the dashboard).
- Lifecycle **self-drives to resolved** with paced, live broadcasts; the only human step is **Approve** (mandatory gate by default; auto-approve is an opt-in toggle, OFF by default).
- Execution + recovery are **simulated server-side** so it always resolves agent-free; the **diagnosis is the real Gemini + Agent Builder + Elastic MCP path**. The real VM-agent restart stays in the recorded demo for the "real remediation" proof.
- Panel labeled **"Simulate Service Failure"** (reads as testing a production system, not faking data).
- **Honesty principle:** scenarios name only **real services on the demo VM** (`WinDefend`, `MySQL`/`MySQL80`, `W3SVC`, `Spooler`). VPN is dropped (not installed); "EDR" is the real Microsoft Defender service, not a fictional one. Defender is simulated-only (tamper protection); `Spooler` is the real-remediation proof.
- All three enhancements (Root Cause step, ADK/MCP badges, Download Report) are in scope.

### Compliance mapping (judge-facing)

| Gemini | Agent Builder / ADK | Elastic MCP | Multi-step workflow | Human approval | Hosted | Judge-interactive |
|---|---|---|---|---|---|---|
| ✅ real | ✅ real | ✅ real | ✅ | ✅ mandatory | ✅ | ✅ |

---

## Verification

- **Go:** `go test ./...` stays green; add tests for `POST /demo/incident` (creates incident on given `deviceId`), `runDemoLifecycle` reaching `resolved`, and `/demo/*` absent when `DEMO_MODE` unset.
- **Frontend:** `tsc -b` clean; `DemoControls` hidden when `VITE_DEMO_MODE` is off; Download Report produces a file from a resolved incident.
- **End-to-end (local):** backend with `DEMO_MODE=true` + agent-engine env; open dashboard, pick a **no-agent** device, choose **Endpoint Security / Microsoft Defender** (`WinDefend`), click **Simulate Service Failure** → incident appears → **Root Cause Identified** lights up with *Gemini + Agent Builder + Elastic MCP* → real cause + `restart_service` → click **Approve** → watch `executing → validating → resolved` animate live → **Download Report**; **Reset** clears it.
- **Real-agent proof (recorded separately):** on the Windows VM running the live agent (`MONITORED_SERVICE_NAME=Spooler`), actually stop `Spooler` → the real agent reports it → the normal (non-demo) flow restarts it → resolved. This is the authentic remediation evidence; the demo panel is the always-works judge interaction.
- **Hosted:** same on `https://pulseops-agent.web.app` with `DEMO_MODE=true` (Cloud Run) + `VITE_DEMO_MODE=true` (Firebase build).

## Out of scope

- New detection types (CPU/DNS/network) or new remediations — staying on service start/stop.
- Changing the real agent/remediation path — unchanged; the demo is additive and gated.
- A new backend lifecycle state — "Root Cause Identified" is display-derived only.
- VPN / DNS scenarios — no VPN service is installed and the chosen services all map to `restart_service`; the `reconnect_vpn` / `flush_dns` catalog actions stay unused.
