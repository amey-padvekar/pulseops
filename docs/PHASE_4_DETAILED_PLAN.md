# PulseOps AI Phase 4 Detailed Plan

Phase: 4 - Incident detection and state machine  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Convert unhealthy telemetry into a managed incident lifecycle that is deterministic, deduplicated, and visible in the dashboard.

Phase 3 proved the backend can ingest telemetry and maintain live device state. Phase 4 adds the operational decision layer: when service health degrades, create and manage incidents with safe state transitions.

At the end of Phase 4:
- service `stopped` while heartbeat is present creates an incident automatically
- repeated failing heartbeats do not create duplicate active incidents
- incident state transitions to at least `detected` -> `investigating`
- incident data is queryable through backend APIs
- incident updates are broadcast to frontend in real time
- dashboard clearly shows an active red incident state

---

## 2) Rule-aware constraints for Phase 4

These constraints from `docs/rules.md` and the overall architecture must shape implementation:

1. Required stack alignment
- Keep backend logic deterministic and explainable; defer AI reasoning to later phases.
- Do not add competing AI/cloud dependencies.

2. Functional submission requirement
- Incident detection must be reproducible from a local failure trigger.
- The same stopped-service scenario must produce the same lifecycle behavior each run.

3. Safety and governance readiness
- State transitions should be explicit and validated; reject illegal transitions.
- Keep action execution out of Phase 4 (execution starts in later phases).

4. Demo readiness
- Detection should happen within one heartbeat interval after failure.
- UI must show a visible red state with incident identifier and current status.

---

## 3) Phase 4 definition of done

Phase 4 is complete only when all are true:

1. Telemetry with `serviceStatus=stopped` and `heartbeat=true` creates a new incident.
2. Subsequent telemetry for the same device/service failure reuses the same active incident.
3. Duplicate active incidents are not created for the same unresolved failure.
4. New incident receives severity and timestamps.
5. Incident transitions to `investigating` path according to the state machine.
6. `GET /incidents` returns active/recent incidents.
7. `GET /incidents/{incidentId}` returns full incident detail.
8. Incident state changes are pushed over WebSocket.
9. Dashboard incident panel shows active incident in red.
10. Phase 4 rows in `docs/PHASE_ACCEPTANCE_CRITERIA.md` pass.

---

## 4) Work breakdown structure

### 4.1 Incident domain model and state definitions

Goal: define a single incident schema and transition vocabulary used by APIs, storage, and UI.

Tasks:
1. Create `backend/internal/incidents/model.go`.
2. Define `IncidentState` constants:
- `healthy`
- `detected`
- `investigating`
- `awaiting_approval`
- `approved`
- `executing`
- `validating`
- `resolved`
- `failed`
3. Define `Severity` constants for MVP (`low`, `medium`, `high`, `critical`).
4. Define `Incident` struct with at least:
- `IncidentID`
- `DeviceID`
- `ServiceName`
- `ServiceStatus`
- `State`
- `Severity`
- `CreatedAt`
- `UpdatedAt`
- `DetectedAt`
- `LastSeenAt`
- `Reason`
- `Active`
5. Add helper constructors for new incidents with consistent timestamping.

Output:
- stable incident DTO for store, API, and websocket payloads.

---

### 4.2 In-memory incident store with deduplication index

Goal: keep incident storage fast and deterministic while preventing duplicates.

Tasks:
1. Create `backend/internal/incidents/store.go`.
2. Add thread-safe maps protected by `sync.RWMutex`:
- `byID map[string]*Incident`
- `activeByKey map[string]string` where key is `deviceId|serviceName|failureSignature`
3. Implement:
- `CreateOrGetActive(key string, seed Incident) (Incident, boolCreated)`
- `GetByID(incidentID string) (Incident, bool)`
- `List(filter IncidentFilter) []Incident`
- `UpdateState(incidentID string, nextState IncidentState, reason string) (Incident, error)`
- `Resolve(incidentID string)` to clear active index
4. Return copies on read operations to avoid races.
5. Sort list output by `UpdatedAt` descending.

Output:
- incident store that enforces one active incident per failing condition.

---

### 4.3 Detection engine on telemetry ingest

Goal: evaluate each telemetry payload and decide if an incident should be created or updated.

Tasks:
1. Create `backend/internal/incidents/detector.go`.
2. Implement `EvaluateTelemetry(state store.DeviceState) DetectionResult`.
3. MVP detection rule:
- if `state.ServiceStatus == "stopped"` and `state.Heartbeat == true`, emit `shouldCreateOrUpdate=true`.
4. Compose a dedupe key:
- `deviceId + "|" + serviceName + "|service_stopped"`
5. Build reason string:
- `service stopped while heartbeat is present`
6. On every matching telemetry, update incident `LastSeenAt` and `UpdatedAt`.

Output:
- deterministic detector that drives incident creation/update logic.

---

### 4.4 Severity assignment strategy

Goal: assign stable severity now, while preserving room for richer logic later.

Tasks:
1. Create `backend/internal/incidents/severity.go`.
2. Implement baseline mapping:
- `service stopped + heartbeat present` => `high`
3. Keep mapper pure (no side effects) for easy unit testing.
4. Include comment extension points for future signals (connectivity, repeated failures).

Output:
- consistent severity attached to every new incident.

---

### 4.5 State machine transition rules

Goal: allow only valid lifecycle transitions.

Tasks:
1. Create `backend/internal/incidents/state_machine.go`.
2. Implement transition table for allowed moves.
3. For Phase 4, minimum live path must include:
- `detected` -> `investigating`
4. Illegal transitions return an error and do not mutate state.
5. Add helper `CanTransition(from, to IncidentState) bool`.

Output:
- enforced incident lifecycle with predictable behavior.

---

### 4.6 Incident API endpoints

Goal: expose incident data to frontend and debugging tools.

Tasks:
1. Create `backend/internal/api/incidents.go`.
2. Implement handlers:
- `GET /incidents`
- `GET /incidents/{incidentId}`
3. Optional filter query params for list:
- `active=true|false`
- `deviceId=<id>`
- `state=<state>`
4. Return JSON error shape on not found:
- `{"error":"incident not found"}`
5. Register routes in `backend/cmd/server/main.go`.

Output:
- incident read APIs available for dashboard integration.

---

### 4.7 WebSocket incident event broadcasts

Goal: push incident changes to frontend immediately.

Tasks:
1. Define WS envelope shape in `backend/internal/ws/events.go`:
```json
{
  "type": "incident.updated",
  "payload": { ...incident }
}
```
2. Broadcast when:
- incident created
- incident state changed
- incident lastSeen updated
3. Reuse existing `hub.Broadcast` in non-blocking mode.
4. Keep telemetry and incident events distinguishable by `type`.

Output:
- frontend can subscribe to incident lifecycle updates without polling.

---

### 4.8 Main wiring: telemetry -> detector -> store -> API/WS

Goal: connect incident logic directly into existing telemetry processing.

Tasks:
1. Update `makeTelemetryHandler` in `backend/cmd/server/main.go`.
2. After device state upsert:
- run detector
- create or update incident in incident store
- transition to `investigating` when created
- broadcast incident event
3. Keep telemetry success response unchanged (`202 accepted`).
4. Add incident store initialization in `main()` and wire handlers.

Output:
- incident lifecycle is triggered in real time by telemetry ingestion.

---

### 4.9 Frontend incident panel baseline

Goal: reflect Phase 4 state in UI with minimal complexity.

Tasks:
1. Add incident types in `frontend/src/types/dashboard.ts` (or new `types/incidents.ts`).
2. Add hook `frontend/src/hooks/useIncidents.ts`:
- initial `GET /incidents`
- live updates from websocket `incident.updated`
3. Update incident card in `DashboardPage.tsx`:
- show active incident id/state/severity
- red styling when active incident exists
4. Keep it intentionally narrow; deeper timeline UX is later-phase work.

Output:
- dashboard shows red incident state when monitored service is stopped.

---

### 4.10 Unit and integration tests

Goal: lock behavior before moving to Phase 5+.

Tasks:
1. Add detector tests:
- stopped + heartbeat => detection true
- running => detection false
2. Add store dedupe tests:
- same key returns existing active incident
- resolved incident allows new incident next time
3. Add state machine tests:
- valid transitions pass
- invalid transitions fail
4. Add API tests for list/get handlers.
5. Add telemetry-handler integration test covering:
- telemetry in
- incident created
- incident list endpoint returns created incident

Output:
- confidence that incident behavior remains stable as new phases are added.

---

### 4.11 Phase 4 smoke-check path

Goal: produce a deterministic, repeatable check aligned to acceptance criteria.

Manual/Script sequence:
1. Start backend, agent, frontend.
2. Confirm healthy baseline (no active incident).
3. Stop monitored service/process.
4. Wait one heartbeat interval.
5. Verify:
- `GET /incidents` returns active incident
- incident state includes `investigating` path
- dashboard incident panel turns red
6. Continue sending heartbeats in failure state and confirm no duplicate active incidents.
7. Capture evidence under `artifacts/phase4-smoke/<timestamp>/`:
- backend logs
- agent logs
- incidents endpoint snapshot
- optional dashboard screenshot

Output:
- repeatable evidence that service-stop creates one managed incident and avoids duplicates.

---

## 5) Detailed implementation order

Build in this order to keep backend runnable after each checkpoint:

1. `internal/incidents/model.go`
2. `internal/incidents/state_machine.go`
3. `internal/incidents/severity.go`
4. `internal/incidents/store.go`
5. `internal/incidents/detector.go`
6. `internal/api/incidents.go`
7. Wire incident store and handlers in `cmd/server/main.go`
8. Integrate detector call from telemetry handler
9. Add websocket incident event envelope + broadcast calls
10. Add backend tests for detector/store/state/API integration
11. Add frontend incident hook + minimal card rendering
12. Run smoke-check and collect artifacts

Why this order:
- model/state/store first gives deterministic backend core
- API and telemetry integration come only after store is testable
- frontend wiring stays last to avoid blocking backend iteration

---

## 6) Key implementation details and pitfalls

1. Dedupe key scope
- include both device and service; otherwise one service failure can mask another.

2. Active incident semantics
- only unresolved incidents belong in `activeByKey`.
- on resolve/fail, clear key mapping.

3. Transition safety
- never mutate state directly from handlers; always go through state machine helpers.

4. Timestamp consistency
- use UTC for all timestamps.
- update `UpdatedAt` on every state and lastSeen mutation.

5. Broadcast resilience
- incident broadcast must never block telemetry ingestion path.

---

## 7) Environment variables reference (Phase 4 additions)

No mandatory new env vars are required for MVP Phase 4.

Optional if needed later in this phase:
- `INCIDENT_DEDUPE_WINDOW_SEC` (for time-boxed dedupe strategies)
- `INCIDENT_DEFAULT_SEVERITY` (for demos)

Keep defaults in code if these are introduced.

---

## 8) New files to create

- `backend/internal/incidents/model.go`
- `backend/internal/incidents/store.go`
- `backend/internal/incidents/detector.go`
- `backend/internal/incidents/state_machine.go`
- `backend/internal/incidents/severity.go`
- `backend/internal/incidents/*_test.go`
- `backend/internal/api/incidents.go`
- `frontend/src/hooks/useIncidents.ts` (minimal)

## 9) Files to modify

- `backend/cmd/server/main.go`
- `backend/internal/ws/events.go` (or existing ws module for event wrappers)
- `frontend/src/pages/DashboardPage.tsx`
- `frontend/src/types/dashboard.ts` (or split incident types)
- `frontend/src/App.css` (incident red-state style)
- `scripts/smoke-check.ps1` (extend with phase4 mode or companion script)

---

## 10) Phase 4 acceptance gate

Pass when all three are true (from `docs/PHASE_ACCEPTANCE_CRITERIA.md`):

- [ ] Service stop event creates incident automatically.
- [ ] Duplicate events do not create duplicate active incidents.
- [ ] Incident state transitions to investigating path.
