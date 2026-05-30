# PulseOps AI Phase 11 Detailed Plan

Phase: 11 - Incident summary generation  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Generate a polished, structured final incident report after the incident lifecycle is complete.

Phase 10 established a trustworthy incident outcome by proving recovery from fresh telemetry or by recording failure explicitly. Phase 11 turns that completed lifecycle into an operator-facing summary artifact that explains what happened, what evidence supported the diagnosis, what actions were taken, and how the incident ended.

At the end of Phase 11:
- backend can build a summary request from the completed incident record
- Agent Builder and Gemini can produce a concise final report grounded in real evidence
- backend stores the summary on the incident
- dashboard renders the final summary clearly
- the summary becomes a strong closing artifact for the live demo and submission materials

---

## 2) Rule-aware constraints for Phase 11

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Stack compliance
- Gemini remains the reasoning engine for the narrative summary.
- Google Cloud Agent Builder remains the orchestration layer used to produce the summary.
- Elastic MCP remains meaningful by contributing operational evidence and context where needed.
- Do not introduce competing AI summarization APIs or alternate orchestration layers.

2. Accuracy and evidence discipline
- The summary must be grounded in actual incident data, not invented narrative.
- Summary generation should use the stored incident record first, with Elastic-backed evidence only as needed to enrich it.
- The summary must distinguish between successful recovery and failed remediation outcomes.

3. Functional submission requirement
- The summary shown in the dashboard must come from a real completed incident.
- The report should be understandable to judges within seconds during a live demo.
- The generated result must be stored so it survives refresh and can support submission artifacts.

4. Demo readiness
- Keep the summary compact, structured, and readable on one screen.
- Prefer deterministic formatting over overly creative prose.
- Include enough detail to prove intelligence and operational depth without flooding the dashboard.

5. Phase boundary
- Phase 11 generates and presents the final summary.
- Phase 12 will refine presentation and polish the overall dashboard experience.
- Phase 11 should not add new execution or validation behavior.

---

## 3) Phase 11 definition of done

Phase 11 is complete only when all are true:

1. A resolved or failed incident can trigger summary generation.
2. Summary request contains root cause, evidence, actions taken, and final result inputs.
3. Agent Builder/Gemini returns a structured final summary.
4. Backend stores the summary on the incident.
5. Dashboard renders the summary clearly.
6. Summary remains visible after page refresh.
7. Summary content reflects the actual incident outcome.
8. Phase 11 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Final summary contract

Goal: define the exact schema for the summary response so backend and frontend can rely on stable fields.

Suggested summary shape:

```json
{
  "rootCause": "OpenVPN service unexpectedly stopped.",
  "evidence": [
    "Telemetry showed serviceStatus=stopped while heartbeat remained true.",
    "Validation telemetry later confirmed serviceStatus=running after remediation."
  ],
  "actionsTaken": [
    "Approved action: restart_service for OpenVPNService.",
    "Agent executed the restart successfully."
  ],
  "result": "Service health recovered and the incident was resolved.",
  "operatorSummary": "The monitored service stopped, the approved remediation restarted it, and recovery was confirmed by fresh telemetry."
}
```

Tasks:
1. Create a strict summary DTO in `backend/internal/agentbuilder` or the incident domain package.
2. Require fields for:
- `rootCause`
- `evidence`
- `actionsTaken`
- `result`
- optional `operatorSummary`
3. Keep evidence and actions as arrays so the UI can render them cleanly.
4. Keep the format deterministic and easy to validate.

Output:
- summary generation and rendering share one stable contract.

---

### 4.2 Summary request payload design

Goal: define what backend sends into the Agent Builder summary workflow.

Backend should include at minimum:
- incident metadata
- original probable cause
- recommended actions
- approved actions
- execution results
- validation outcome
- selected evidence snippets from telemetry/logs/incident timeline

Tasks:
1. Create `FinalSummaryRequest` model.
2. Populate it from incident state accumulated across Phases 4-10.
3. Keep the payload bounded and summarize large logs before inclusion.
4. Include explicit outcome state so failed incidents can be summarized accurately.

Output:
- Agent Builder receives a compact but sufficient summary context package.

---

### 4.3 Summary trigger rules

Goal: determine when the system should generate the final summary.

Tasks:
1. Define allowed trigger states:
- `resolved`
- `failed`
2. Decide whether summary generation is:
- automatic immediately after closure
- or operator-triggered for debugging/demo backup
3. Prevent summary generation for incidents still in:
- `investigating`
- `awaiting_approval`
- `approved`
- `executing`
- `validating`
4. Add idempotency rules so the summary is not regenerated accidentally on every refresh unless explicitly requested.

Output:
- summary generation happens at a predictable lifecycle boundary.

---

### 4.4 Agent Builder summary workflow contract

Goal: make the summary workflow reliably produce useful operator-facing output.

Tasks:
1. Define prompt/workflow instructions in `backend/internal/agentbuilder` docs or config.
2. Require the workflow to:
- summarize only from provided evidence
- preserve factual distinction between diagnosis, actions, and outcome
- produce concise operator-facing wording
- avoid speculative language beyond the available record
3. Reuse Agent Builder and Gemini integration patterns established in Phases 6 and 7.
4. Keep the output compact enough for dashboard display and demo narration.

Output:
- the summary workflow produces a consistent closing artifact instead of freeform prose drift.

---

### 4.5 Backend parsing and validation of summary output

Goal: safely accept only well-formed summary responses.

Tasks:
1. Add `ParseFinalSummary(...)` logic in `backend/internal/agentbuilder`.
2. Validate that:
- `rootCause` is non-empty
- `evidence` contains at least one item
- `actionsTaken` contains at least one item when remediation occurred
- `result` is non-empty
3. Preserve raw summary payload for debugging if parsing fails.
4. Provide fallback behavior when summary generation times out or returns malformed output.

Output:
- backend stores only structured, usable summary content.

---

### 4.6 Incident summary storage model

Goal: make the final summary a first-class part of the incident record.

Tasks:
1. Extend incident model with fields such as:
- `finalSummary`
- `summaryGeneratedAt`
- `summaryStatus`
- `summaryRequestId` if helpful for tracing
2. Keep the summary separate from investigation output and validation evidence.
3. Update `UpdatedAt` when summary generation completes.
4. Ensure REST/websocket payloads expose the final summary to frontend.

Output:
- the incident becomes the durable source of truth for the closing report.

---

### 4.7 Evidence selection and compaction

Goal: provide enough operational detail for a meaningful summary without overwhelming the model or UI.

Tasks:
1. Select a small number of high-value evidence points from:
- initial unhealthy telemetry
- relevant log snippets
- AI investigation result
- approved remediation actions
- execution results
- validation telemetry
2. Avoid dumping raw full logs into the summary request.
3. Prefer concise evidence sentences or compact structured facts.
4. Make sure evidence reflects whether the incident resolved or failed.

Output:
- the summary feels grounded and specific without becoming noisy.

---

### 4.8 Frontend final summary panel

Goal: render the closing report clearly in the dashboard.

Tasks:
1. Add a final summary section or card to the incident view.
2. Render:
- root cause
- evidence list
- actions taken
- final result
- optional short operator summary
3. Provide loading and fallback states when summary is pending or unavailable.
4. Keep layout compact and readable on the same dashboard used in the demo.

Output:
- users can see the complete incident story without leaving the dashboard.

---

### 4.9 Export/copy readiness for demo artifacts

Goal: make the summary easy to use in submission prep even if full export features are deferred.

Tasks:
1. Make the summary copy-friendly in the UI.
2. Optionally add a simple copy/export action if it is low effort.
3. Ensure the formatting works well for screenshots, demo narration, and text description reuse.
4. Keep this lightweight; the core requirement is visibility and clarity.

Output:
- the final summary doubles as a reusable demo and submission artifact.

---

### 4.10 Timeout and fallback behavior

Goal: keep completed incidents understandable even if live summary generation is slow or unavailable.

Tasks:
1. Define a summary generation timeout budget.
2. If the workflow fails or times out:
- mark summary status accordingly
- show a clear UI fallback state
- optionally synthesize a minimal deterministic fallback summary from stored incident fields
3. Log request/response identifiers and failure reason.
4. Keep the incident usable even without the polished AI narrative.

Output:
- summary generation enhances the product without becoming a single point of demo failure.

---

### 4.11 Tests for final summary generation

Goal: lock the summary contract before dashboard polish work begins.

Tasks:
1. Backend tests for:
- summary request payload assembly
- summary parsing success path
- malformed summary rejection
- fallback behavior on timeout
- storage and retrieval of final summary on incident
2. Incident lifecycle tests ensuring summary generation only occurs after closure states.
3. Frontend verification through build plus manual checklist unless UI test conventions already exist.

Output:
- summary generation remains predictable and grounded in stored incident data.

---

### 4.12 Manual rehearsal and evidence capture

Goal: prove that the final summary works as the closing artifact in the live story.

Tasks:
1. Extend rehearsal path to include:
- incident resolves or fails
- summary generation triggers
- summary appears in dashboard
- summary survives refresh
2. Capture one example summary artifact under `artifacts/` if useful.
3. Verify the summary can be read aloud quickly in a 3-minute demo.

Output:
- Phase 11 is validated as a judge-facing artifact, not just a backend feature.

---

## 5) Recommended implementation order

Implement Phase 11 in this order:

1. Define summary request/response DTOs.
2. Extend incident model with final summary storage fields.
3. Build request assembly from incident, execution, and validation data.
4. Add Agent Builder summary workflow contract and parser.
5. Wire summary trigger on resolved/failed incidents.
6. Render the final summary in the dashboard.
7. Add timeout/fallback handling.
8. Run manual rehearsal on one completed incident.

This order stabilizes the data contract before UI work and keeps the summary grounded in existing incident evidence rather than ad hoc rendering logic.

---

## 6) File-by-file implementation map

Expected backend touch points:
- `backend/internal/agentbuilder/` for summary models, request assembly, parser, and workflow client integration
- `backend/internal/incidents/` or `backend/internal/store/` for summary storage on incidents
- `backend/internal/api/` for exposing summary fields to frontend if incident DTOs need extension
- `backend/cmd/server/main.go` for wiring summary trigger or routes if needed

Expected frontend touch points:
- `frontend/src/types/` for final summary fields
- `frontend/src/pages/DashboardPage.tsx` for summary panel rendering
- `frontend/src/components/` for a dedicated summary card if helpful
- `frontend/src/App.css` for summary presentation styling

Expected docs/scripts touch points:
- demo runbook docs if the summary becomes part of the rehearsed narration
- optional artifact capture steps if you want a reusable closing screenshot or transcript

---

## 7) Pitfalls to avoid

1. Do not let the summary invent facts.
- It should summarize the incident record and selected evidence, not speculate beyond them.

2. Do not regenerate the whole incident story from scratch.
- Reuse stored diagnosis, approval, execution, and validation data as the authoritative source.

3. Do not make the summary too long.
- Judges need to understand it quickly during the final seconds of the demo.

4. Do not hide failed outcomes.
- A failed incident still needs a truthful summary with evidence and actions taken.

5. Do not bury the summary behind navigation friction.
- It should be visible directly in the main incident/dashboard experience.

6. Do not make the demo depend entirely on live cloud latency.
- Provide fallback behavior so the completed incident remains explainable even if summary generation is slow.

---

## 8) Acceptance gate for moving to Phase 12

Do not start Phase 12 until all checks below are true:

1. Final summary includes root cause, evidence, actions taken, and result.
2. Summary is stored on the incident and survives refresh.
3. Summary is visible in the dashboard.
4. Summary generation only occurs for completed incidents.
5. Fallback behavior exists for summary timeout or malformed output.
6. The Phase 11 pass conditions in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) are demonstrably satisfied.

---

## 9) Outcome of this phase

When Phase 11 is complete, PulseOps AI will end each completed incident with a concise, evidence-backed final report that explains what failed, what the system did, and how the incident ended. That summary is the closing artifact that ties together the earlier AI reasoning, human approval, endpoint action, and telemetry validation into one coherent story for operators and judges.