# PulseOps AI Phase 5 Detailed Plan

Phase: 5 - Elastic integration for logs and telemetry  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Make telemetry, incidents, and selected endpoint log context queryable in Elastic so the observability layer is real, useful, and ready for Elastic MCP-backed investigation in later phases.

Phase 4 established detection and incident lifecycle. Phase 5 makes those operational events searchable and correlatable in Elastic.

At the end of Phase 5:
- telemetry events are indexed into Elastic
- incident events are indexed into Elastic
- optional endpoint log snippets are indexed into Elastic
- indexed documents contain stable, search-ready fields needed for later MCP queries
- at least one failing-device context query returns meaningful records

This phase is not about perfect observability architecture. It is about building a credible, working Elastic-backed context layer that later phases can query through Elastic MCP.

---

## 2) Rule-aware constraints for Phase 5

These constraints from [docs/rules.md](docs/rules.md) must directly shape implementation:

1. Elastic must be meaningful, not cosmetic
- Data must actually land in Elastic and be queryable.
- Do not fake Elastic integration with local-only logging or screenshots.

2. Stack compliance
- Keep using Gemini and Google Cloud Agent Builder in later phases for reasoning and orchestration.
- Do not introduce competing observability or cloud platforms.

3. Functional submission requirement
- Indexing must be runnable locally or against a configured Elastic deployment.
- The project must function as demonstrated, so Phase 5 should end with a reproducible query path.

4. Demo readiness
- Prefer simple index naming, deterministic document shapes, and low-latency ingestion.
- Optimize for explainability and visible proof over high-volume correctness.

5. Security and repository readiness
- Keep secrets out of committed files.
- Use environment variables for Elastic endpoint and credentials.

---

## 3) Phase 5 definition of done

Phase 5 is complete only when all are true:

1. Telemetry events are indexed into Elastic on ingest.
2. Incident events are indexed into Elastic on create/update.
3. Indexed documents contain required fields: `deviceId`, `serviceName`, `serviceStatus`, `severity`, `incidentId`, `timestamp`.
4. Log snippets are either indexed or a documented MVP fallback is implemented.
5. Backend tolerates Elastic failures without breaking core detect/update behavior.
6. At least one query against Elastic returns telemetry records for a known device.
7. At least one query against Elastic returns incident records for a known incident.
8. Evidence exists showing the failing device has searchable records.
9. Phase 5 rows in [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md) pass.

---

## 4) Work breakdown structure

### 4.1 Elastic document strategy and index naming

Goal: define stable, low-friction index names and document categories for the MVP.

Tasks:
1. Use these index prefixes:
- `telemetry-events-*`
- `incident-events-*`
- `endpoint-logs-*`
2. Use daily or date-suffixed index naming for simplicity, for example:
- `telemetry-events-2026.05.23`
- `incident-events-2026.05.23`
- `endpoint-logs-2026.05.23`
3. Keep naming compatible with later index patterns and MCP queries.
4. Use UTC-based event timestamps consistently.

Output:
- a simple index strategy that is easy to reason about and query during the demo.

---

### 4.2 Elastic configuration model

Goal: centralize Elastic connectivity configuration without hard-coding secrets.

Tasks:
1. Create `backend/internal/elastic/config.go`.
2. Read environment variables:
- `ELASTIC_ENDPOINT`
- `ELASTIC_API_KEY`
- `ELASTIC_INDEX_TELEMETRY`
- `ELASTIC_INDEX_INCIDENTS`
- `ELASTIC_INDEX_LOGS`
3. Provide sensible local defaults for index names.
4. Validate endpoint format and fail clearly when required Elastic settings are missing for enabled mode.
5. Add a simple enable flag strategy:
- either treat non-empty endpoint + API key as enabled
- or add `ELASTIC_ENABLED=true|false`

Output:
- one runtime config source for all Elastic ingestion behavior.

---

### 4.3 Elastic client wrapper

Goal: isolate Elasticsearch SDK usage behind a small backend package.

Tasks:
1. Create `backend/internal/elastic/client.go`.
2. Use the official Go Elasticsearch client.
3. Implement a wrapper with methods such as:
- `IndexTelemetryEvent(ctx, doc)`
- `IndexIncidentEvent(ctx, doc)`
- `IndexLogEvent(ctx, doc)`
4. Keep method signatures narrow and stable.
5. Return typed errors or wrapped errors for logging.

Output:
- backend code can index documents without leaking raw SDK usage into handlers.

---

### 4.4 Telemetry event document mapping

Goal: convert live device state into a search-friendly telemetry document.

Tasks:
1. Create `backend/internal/elastic/telemetry.go`.
2. Define `TelemetryEventDocument` containing at least:
- `eventType`
- `timestamp`
- `deviceId`
- `serviceName`
- `serviceStatus`
- `heartbeat`
- `networkReachable`
- `cpuUsage`
- `memoryUsage`
- `incidentId` (optional)
- `recentLogs` (bounded)
3. Preserve raw operational meaning while keeping the shape flat enough for simple queries.
4. Include a schema version field if useful.
5. Build from Phase 3 device state and incoming telemetry payloads.

Output:
- telemetry documents that are easy to query by device, service, and state.

---

### 4.5 Incident event document mapping

Goal: index every meaningful incident lifecycle change with searchable context.

Tasks:
1. Create `backend/internal/elastic/incidents.go`.
2. Define `IncidentEventDocument` containing at least:
- `eventType`
- `timestamp`
- `incidentId`
- `deviceId`
- `serviceName`
- `serviceStatus`
- `severity`
- `state`
- `reason`
- `active`
3. Emit documents on:
- incident created
- incident state updated
- incident resolved/failed later
4. Keep one event per lifecycle change rather than overwriting a single record.

Output:
- incident history is queryable as an event stream.

---

### 4.6 Endpoint log snippet document mapping

Goal: make recent log context searchable enough for believable investigation queries.

Tasks:
1. Create `backend/internal/elastic/logs.go`.
2. Convert telemetry `recentLogs` entries into log documents with fields:
- `eventType`
- `timestamp`
- `deviceId`
- `serviceName`
- `incidentId` (optional)
- `message`
- `source` (`agent_recent_logs`)
3. Deduplicate or bound volume so every heartbeat does not explode log count.
4. For MVP, it is acceptable to index only the latest N log lines per telemetry cycle.

Output:
- searchable endpoint log snippets with enough context for demo narratives.

---

### 4.7 Backend ingestion wiring

Goal: index data as part of normal backend flow without breaking core functionality when Elastic is down.

Tasks:
1. Initialize Elastic client in `backend/cmd/server/main.go`.
2. On telemetry receipt:
- upsert device state
- index telemetry event asynchronously or best-effort synchronously
- optionally index recent logs
3. On incident create/update:
- index incident event
4. If Elastic indexing fails:
- log the failure clearly
- do not fail the HTTP telemetry request solely because Elastic is unavailable
5. Keep core Phase 3 and Phase 4 behavior functional without Elastic.

Output:
- Elastic acts as an observability sink, not a single point of failure.

---

### 4.8 Query-ready field set and search ergonomics

Goal: guarantee the fields required in the implementation plan are always present and consistent.

Tasks:
1. Ensure these fields are indexed where applicable:
- `deviceId`
- `serviceName`
- `serviceStatus`
- `severity`
- `incidentId`
- `timestamp`
2. Normalize field casing and spelling across telemetry and incident docs.
3. Prefer flat field names over deeply nested structures for MVP query simplicity.
4. Add `eventType` so mixed result streams remain understandable.

Output:
- later Elastic MCP queries can reliably filter by core operational dimensions.

---

### 4.9 Mapping/templates and index creation strategy

Goal: prevent accidental bad field inference while keeping setup lightweight.

Tasks:
1. Create `backend/internal/elastic/templates.go` if needed.
2. For MVP choose one of two paths:
- explicit index template creation at backend startup
- document-first indexing with disciplined field shapes if template setup is too heavy
3. If creating templates, define keyword/date/boolean/float types for core fields.
4. Keep template setup idempotent.

Recommendation:
- use explicit templates if the client setup remains simple
- otherwise document the fallback and keep field names stable

Output:
- predictable field mappings for demo queries.

---

### 4.10 Backend tests for Elastic mapping and failure handling

Goal: verify document shaping logic and ensure indexing failures do not break core flow.

Tasks:
1. Add unit tests for telemetry document builders.
2. Add unit tests for incident document builders.
3. Add unit tests for log snippet document builders.
4. Add tests for disabled/unconfigured Elastic mode.
5. Add tests for indexing failure handling in the telemetry path (best-effort behavior).

Output:
- confidence that observability integration remains stable as later AI phases are added.

---

### 4.11 Phase 5 smoke-check path

Goal: create reproducible evidence that Elastic is receiving and returning operational context.

Manual/Script sequence:
1. Start backend, agent, and frontend.
2. Confirm telemetry is arriving locally.
3. Trigger stopped-service incident.
4. Wait one heartbeat interval.
5. Verify in Elastic:
- telemetry documents exist for the device
- incident event documents exist for the incident
- log snippet documents exist if enabled
6. Run at least one query by `deviceId` and one by `incidentId`.
7. Capture evidence under `artifacts/phase5-smoke/<timestamp>/`:
- backend logs
- sample telemetry query output
- sample incident query output
- optional Kibana screenshot

Output:
- repeatable evidence that Elastic contains meaningful context for the failing device.

---

## 5) Detailed implementation order

Build in this order to keep the backend runnable after each step:

1. `backend/internal/elastic/config.go`
2. `backend/internal/elastic/client.go`
3. `backend/internal/elastic/telemetry.go`
4. `backend/internal/elastic/incidents.go`
5. `backend/internal/elastic/logs.go`
6. Optional template/mapping setup
7. Wire Elastic client into `backend/cmd/server/main.go`
8. Add telemetry indexing path
9. Add incident indexing path
10. Add best-effort logging and failure handling
11. Add tests for mapping/builders/failure tolerance
12. Add smoke-check path and evidence capture

Why this order:
- config and client first define the integration boundary
- document builders next make the data model explicit before wiring
- runtime integration comes only after documents are testable
- smoke-check closes the loop with actual query proof

---

## 6) Key implementation details and pitfalls

1. Do not block core flow on Elastic
- indexing failures should degrade observability, not break telemetry ingestion or incident detection.

2. Keep event documents append-only
- event streams are more useful for later timeline reasoning than overwrite-in-place snapshots.

3. Bound log volume early
- unbounded recent log indexing will create noisy data and slow queries.

4. Preserve timestamps from source events
- use event timestamps from telemetry/incidents, not only indexing time.

5. Keep the story believable
- index only data the system actually has.
- avoid synthetic fields that later MCP queries cannot justify.

6. Stay within contest intent
- Elastic must clearly contribute to observability and later investigation, not just exist in architecture diagrams.

---

## 7) Environment variables reference (Phase 5)

Backend variables for Elastic integration:

- `ELASTIC_ENDPOINT`
- `ELASTIC_API_KEY`
- `ELASTIC_INDEX_TELEMETRY`
- `ELASTIC_INDEX_INCIDENTS`
- `ELASTIC_INDEX_LOGS`
- optional: `ELASTIC_ENABLED`

Keep secrets in local `.env` only, never in committed examples beyond placeholders.

---

## 8) New files to create

- `backend/internal/elastic/config.go`
- `backend/internal/elastic/client.go`
- `backend/internal/elastic/telemetry.go`
- `backend/internal/elastic/incidents.go`
- `backend/internal/elastic/logs.go`
- `backend/internal/elastic/*_test.go`
- optional: `backend/internal/elastic/templates.go`

## 9) Files to modify

- `backend/cmd/server/main.go`
- `backend/.env.example`
- `scripts/smoke-check.ps1` or companion Phase 5 smoke script
- possibly shared schema docs if indexed document shape is promoted into public contracts

---

## 10) Phase 5 acceptance gate

Pass when all three are true (from [docs/PHASE_ACCEPTANCE_CRITERIA.md](docs/PHASE_ACCEPTANCE_CRITERIA.md)):

- [ ] Telemetry and incident events are queryable in Elastic.
- [ ] Required fields (`deviceId`, `serviceName`, `timestamp`, `severity`) are indexed.
- [ ] At least one incident context query returns expected records.
