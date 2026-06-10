# Remediation Command Contract (Phase 9, step 4.1)

Purpose
- Define the one durable payload format the backend dispatches and the endpoint agent
  executes. This is the single execution contract shared between the backend control
  plane and the Go endpoint agent.

## Canonical artifact
- docs/contracts/remediation_command_fixture.json

## Source of truth (Go)
- Backend (producer): `backend/internal/remediation/dispatch.go` — `DispatchCommand`, `NewDispatchCommand`
- Agent (consumer): `agent/internal/remediation/contract.go` — `Command`, `Action`

The backend and agent are independent Go modules, so the contract is mirrored as two
separate structs with identical JSON tags rather than a shared import. The fixture
above is the tie-breaker; both structs must round-trip it.

## Payload shape

```json
{
  "incidentId": "INC-1001",
  "deviceId": "DEV-AGENT-01",
  "approvedBy": "demo.operator",
  "approvedAt": "2026-05-23T22:10:00Z",
  "actions": [
    { "actionId": "restart_service", "target": "OpenVPNService" }
  ],
  "dispatchedAt": "2026-05-23T22:10:05Z",
  "requestId": "rem-12345"
}
```

Field notes
- `incidentId`, `deviceId`: route the command to the correct device for the correct incident.
- `approvedBy`, `approvedAt`: approval metadata carried through verbatim for traceability.
- `actions[]`: bounded, action-ID-based steps only.
  - `actionId`: a backend catalog action (`restart_service`, `flush_dns`, `reconnect_vpn`).
    The agent maps this to a platform-specific implementation; it is never executed literally.
  - `target` (optional): the subject of the action (e.g. a service name).
- `dispatchedAt`: when the backend handed the command off.
- `requestId`: correlation id for a single dispatch attempt; ties together backend and agent logs.

## Rules and safety constraints (MUST enforce)
- The payload is action-ID-based only. It MUST NOT carry raw shell commands or freeform
  command strings from model output or dashboard input.
- Every `actionId` MUST be in the backend remediation catalog
  (`backend/internal/remediation/catalog.go`). The producing constructor `NewCommand`
  enforces this before a command is ever queued or dispatched.
- The wire payload MUST NOT include backend-only queue state (no `status` field).
- The agent MUST reject any `actionId` it cannot map to a known platform implementation
  (enforced in later Phase 9 steps).

## Verification commands
Backend module root:

    go test ./internal/remediation -run TestNewDispatchCommand -v
    go test ./internal/remediation -run TestDispatchCommand_JSONShapeMatchesContract -v

Agent module root:

    go test ./internal/remediation -run TestCommand_DeserializesCanonicalFixture -v

---

# Dispatch Mechanism (Phase 9, step 4.3)

Delivery path
- `GET /devices/{deviceId}/commands` — the endpoint agent polls this to fetch the
  remediation commands approved for its device. Fetching is the dispatch act.

Why polling: the agent already polls the backend over HTTP for its heartbeat, so a
device-scoped fetch endpoint is the simplest mechanism that fits the existing
architecture and stays observable for the demo. No new transport or streaming layer.

Response body (`PendingCommandsResponse`):

```json
{
  "deviceId": "DEV-AGENT-01",
  "commands": [ /* zero or more DispatchCommand */ ]
}
```

Lifecycle and guarantees
- Commands enter the queue only via `NewCommand`, which requires an `approved`
  incident — so only approved work is ever dispatched.
- On fetch, each `queued` command transitions to `dispatched`, is stamped with a fresh
  `requestId` and `dispatchedAt`, and its `dispatchCount` is incremented.
- A command is dispatched exactly once; a second poll returns `[]`. Re-dispatch is an
  explicit opt-in via `Queue.RequeueForRetry`.
- `acknowledged` is an optional lifecycle state (`Queue.MarkAcknowledged`) used for
  observability only; it does not affect repeat-dispatch prevention.

Source of truth (Go)
- `backend/internal/remediation/queue.go` — `ClaimPendingForDevice`, `RequeueForRetry`, `MarkAcknowledged`
- `backend/internal/api/remediation_handler.go` — `PendingCommandsHandler`

---

# Result Ingestion (Phase 9, step 4.7)

Endpoint
- `POST /remediation/results` — the agent posts an ExecutionResult (the step 4.2 shape)
  here after attempting a command.

Validation (all must pass)
1. Shape: `incidentId`, `deviceId`, `requestId` non-empty; `status` in the normalized vocabulary.
2. Incident exists.
3. `deviceId` matches the incident's device.
4. `requestId` matches the command the backend actually dispatched for that incident.

Lifecycle effects
- At dispatch (step 4.3) the incident moves `approved -> executing` (task 4.7.3). If a
  result arrives while still `approved`, ingestion performs that step implicitly.
- Post-result boundary (task 4.7.5):
  - `succeeded` -> `validating` (Phase 10 decides whether health was actually restored;
    Phase 9 never resolves on command success alone)
  - anything else -> `failed`
- The execution outcome (status, requestId, timestamps, per-action results) is persisted
  on the incident; logs are bounded and timestamps normalized to UTC before storage.
- The updated incident is broadcast over the websocket.

Reporter (agent side)
- `agent/internal/remediation/reporter.go` — `HTTPReporter` POSTs the result; wired into
  the Executor via `WithReporter`, closing the loop.

Source of truth (Go)
- `backend/internal/api/remediation_handler.go` — `RemediationResultHandler`
- `backend/internal/incidents/store.go` — `MarkExecuting`, `SaveRemediationResult`
- `backend/internal/incidents/remediation_result.go` — `ExecutionOutcome`, `RemediationActionResult`

---

# Incident Execution Fields & Timeline (Phase 9, step 4.8)

The incident record carries the execution phase, kept separate from approval metadata.

Execution metadata (on `Incident`, populated once the agent reports back)
- `remediationStatus`, `remediationRequestId`
- `remediationStartedAt`, `remediationFinishedAt`, `remediationReceivedAt`
- `remediationResults[]` — per-action outcomes (the step 4.2 ActionResult shape)

Execution timeline (`timeline[]` of `{type, at, detail?}`) — chronological milestones:
- `command_queued` — emitted at approval when the command is enqueued
- `command_dispatched` — emitted when the agent fetches the command (incident -> executing)
- `command_started` / `command_finished` — emitted on result ingestion, stamped with the
  agent's reported start/finish timestamps

Source of truth (Go)
- `backend/internal/incidents/model.go` — `Incident.Remediation*`, `Incident.Timeline`
- `backend/internal/incidents/timeline.go` — `TimelineEvent`, `TimelineEventType`
- Frontend mirror: `frontend/src/types/dashboard.ts`

---

# Retry & Duplicate-Execution Safeguards (Phase 9, step 4.10)

Stable request id
- Every dispatch stamps a fresh, stable `requestId` (step 4.3). It is the correlation
  key for dedup and retry.

Agent dedup (naive duplicate prevention)
- The agent records each `requestId` it has executed (success or failure) and ignores a
  command delivered again under the same id, logging the duplicate. A confirmed failed
  action is therefore never silently re-run under the same id.
- Source: `agent/internal/remediation/dedup.go`, gated in `Executor.Handle`.

Backend duplicate-result safeguard
- Ingesting a result acknowledges the command. A second `POST /remediation/results` for
  the same (already-acknowledged) `requestId` is logged and answered idempotently with
  the stored incident — no re-persist, no duplicate timeline events.

Backend retry policy (`Queue.RequeueForRetry`)
- Allowed only for a command that is dispatched but **not acknowledged** (e.g. agent
  timeout during retrieval, or a result that never arrived).
- Refused once acknowledged — so a confirmed/failed result is never auto-retried.
- Retry re-arms the command and the next poll assigns a **new** `requestId`, so a
  deliberate retry is never mistaken for a naive duplicate by the agent dedup.
- There is no automatic retry trigger; retry is an explicit, logged decision.

---

# Execution Result Contract (Phase 9, step 4.2)

Purpose
- Define the result payload the agent returns after attempting remediation, so the
  backend can store and render execution results consistently.

## Canonical artifact
- docs/contracts/remediation_result_fixture.json

## Source of truth (Go)
- Backend (consumer/store): `backend/internal/remediation/result.go` — `ExecutionResult`, `ActionResult`, `ExecutionStatus`
- Agent (producer): `agent/internal/remediation/result.go` — same shapes mirrored

## Payload shape

```json
{
  "incidentId": "INC-1001",
  "deviceId": "DEV-AGENT-01",
  "requestId": "rem-12345",
  "status": "succeeded",
  "startedAt": "2026-05-23T22:10:06Z",
  "finishedAt": "2026-05-23T22:10:08Z",
  "results": [
    {
      "actionId": "restart_service",
      "target": "OpenVPNService",
      "status": "succeeded",
      "stdout": "Service restarted successfully",
      "stderr": "",
      "exitCode": 0,
      "durationMs": 1500
    }
  ]
}
```

Field notes
- `requestId`: echoes the dispatched command's `requestId` for end-to-end correlation.
- top-level `status` and per-action `status`: drawn from the normalized vocabulary below.
- `startedAt`/`finishedAt`: UTC, RFC 3339.
- `results[].stdout`/`stderr`: bounded execution logs (see size cap).
- `results[].exitCode`: process exit code (step 4.6). Omitted when the action was
  rejected before any command ran; `-1` when a process failed to start or was signaled.
- `results[].durationMs`: wall-clock duration of the executed command; `0` for rejected
  actions.

## Normalized status vocabulary (`ExecutionStatus`)
One closed set, shared by command lifecycle and execution results so the store and
dashboard render a single vocabulary:

- `queued` — approved, waiting to be dispatched
- `dispatched` — handed to the endpoint, not yet started
- `running` — agent is actively executing
- `succeeded` — completed successfully
- `failed` — ran but did not succeed
- `rejected` — agent refused (e.g. unknown/unmapped action ID)

## Result rules (MUST enforce)
- Timestamps MUST be UTC. `ExecutionResult.Normalize()` forces this on the backend.
- Per-action `stdout`/`stderr` MUST be bounded. `BoundLog` caps each stream at
  `MaxLogBytes` (4096) and appends a truncation marker on a UTF-8 rune boundary.
- `status` values MUST be from the normalized vocabulary; `ExecutionStatus.IsValid()`
  checks membership.

## Verification commands
Backend module root:

    go test ./internal/remediation -run TestExecutionResult -v
    go test ./internal/remediation -run TestBoundLog -v

Agent module root:

    go test ./internal/remediation -run TestExecutionResult_DeserializesCanonicalFixture -v

---

## Change control
Any change to the fixture keys, casing, or required fields requires:
1. Updating the backend and agent structs together (`dispatch.go`/`contract.go` for the
   command; `result.go` on both sides for the result).
2. Updating the corresponding tests on both sides.
3. Updating the affected fixture (`remediation_command_fixture.json` /
   `remediation_result_fixture.json`) and this note.
