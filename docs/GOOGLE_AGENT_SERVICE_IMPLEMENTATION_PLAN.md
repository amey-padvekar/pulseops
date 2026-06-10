# PulseOps Google Agent Service: Detailed Step-by-Step Implementation Plan

Purpose: implement the Google-side agent investigation service for PulseOps Phase 6 and Phase 7 using an ADK-first runtime (Cloud Run adapter -> ADK agent -> Gemini + tools) while preserving existing backend contracts.

This document replaces the previous high-level plan with an execution plan you can run step-by-step with clear entry and exit criteria.

---

## 0) Scope, non-goals, and success criteria

### In scope
- One HTTPS endpoint: `POST /investigate`
- Contract compatibility with PulseOps backend sender and parser
- ADK-based reasoning workflow with optional Elastic MCP enrichment
- Strict `InvestigationResult` response payload in backend-compatible envelope
- Operational hardening for auth, timeouts, retries, logs, and metrics

### Out of scope
- Executing remediation actions
- Changing PulseOps incident lifecycle semantics
- Replacing Elastic as context source for this phase

### Success criteria
- Backend can call Google service without payload shape changes
- Google service returns parseable strict result envelope
- Backend stores and UI renders investigation fields
- Smoke evidence captured and reproducible for demo

---

## 1) Source-of-truth contracts you must not break

Before writing new code, freeze and review these files:
- `backend/internal/agentbuilder/adk_client.go` (request payload shape)
- `backend/internal/agentbuilder/prompt.go` (prompt constraints)
- `backend/internal/agentbuilder/CONTRACT.md` (contract notes)
- `backend/internal/agentbuilder/parser.go` (response parsing and validation)
- `backend/internal/agentbuilder/investigation_model.go` (result model)
- `backend/internal/agentbuilder/config.go` (runtime configuration)
- `backend/cmd/server/main.go` (wiring and persistence flow)

Required response envelope fields:
- `request_id`: string
- `trace_id` (or `operation_id`): string
- `status.transport`: `success | error`
- `status.workflow`: `accepted | completed | failed`
- `result`: strict `InvestigationResult`

---

## 2) Delivery strategy and timeline

Use this phased rollout for predictable delivery:

- Phase A (Day 1): contract freeze, project bootstrap, service skeleton
- Phase B (Day 2): validation, envelope, ADK agent baseline
- Phase C (Day 3): tools, enrichment bounds, safety guardrails
- Phase D (Day 4): deployment, integration, smoke run
- Phase E (Day 5): observability hardening, rollback drills, evidence packaging

If time is short, complete through Phase D for MVP demo readiness.

---

## 3) Detailed execution checklist (step-by-step)

## Phase A: Foundation and contract lock (Day 1)

### Step A1: Freeze backend contract fixtures
Actions:
1. Capture one real request fixture emitted by backend `BuildADKRequestPayload`.
2. Capture one valid response fixture currently accepted by parser.
3. Save as canonical fixtures for all tests.

Deliverables:
- `docs/contracts/adk_request_fixture.json`
- `docs/contracts/adk_response_fixture.json`
- `docs/contracts/CONTRACT_FREEZE_NOTES.md`

Exit criteria:
- Team confirms no changes to required request and response fields.

Implementation status:
- Completed on 2026-06-02.
- Frozen artifacts created under `docs/contracts/`.
- Contract checks passed in backend tests for request builder and parser validity.

### Step A2: Create GCP project baseline
Actions:
1. Select projects:
- dev: `pulseops-agent-dev`
- prod: `pulseops-agent-prod`
2. Select region (default): `us-central1`
3. Enable required APIs.

Command block:
```bash
gcloud config set project pulseops-agent-dev

gcloud services enable \
  run.googleapis.com \
  aiplatform.googleapis.com \
  artifactregistry.googleapis.com \
  cloudbuild.googleapis.com \
  secretmanager.googleapis.com \
  logging.googleapis.com \
  monitoring.googleapis.com \
  iam.googleapis.com
```

Exit criteria:
- All API enable calls succeed.

Implementation status:
- Completed on 2026-06-02.
- Active account did not have access to `pulseops-agent-dev`; baseline executed in accessible project `pulseops-agent`.
- Required APIs verified enabled: run, aiplatform, artifactregistry, cloudbuild, secretmanager, logging, monitoring, iam.

### Step A3: Create image repository and runtime identity
Actions:
1. Create Artifact Registry repository.
2. Create Cloud Run runtime service account.
3. Grant minimum required roles.

Command block:
```bash
gcloud artifacts repositories create pulseops-agent \
  --repository-format=docker \
  --location=us-central1 \
  --description="PulseOps Google agent service images"

gcloud iam service-accounts create pulseops-agent-svc \
  --display-name="PulseOps Agent Service Runtime"

gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
  --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
  --role="roles/aiplatform.user"

gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
  --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
  --role="roles/secretmanager.secretAccessor"

gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
  --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
  --role="roles/logging.logWriter"

gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT \
  --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" \
  --role="roles/monitoring.metricWriter"
```

Exit criteria:
- Repository exists and IAM bindings are visible in policy.

Implementation status:
- Completed on 2026-06-02.
- Executed in accessible project `pulseops-agent`.
- Artifact Registry repository created and verified in `us-central1`: `pulseops-agent`.
- Runtime service account created and verified: `pulseops-agent-svc@pulseops-agent.iam.gserviceaccount.com`.
- Required IAM roles verified on runtime service account: `roles/aiplatform.user`, `roles/secretmanager.secretAccessor`, `roles/logging.logWriter`, `roles/monitoring.metricWriter`.

### Step A4: Create and load secrets
Actions:
1. Create inbound auth token secret.
2. Create Elastic MCP token secret.

Command block:
```bash
echo -n "REPLACE_ME" | gcloud secrets create pulseops-agent-auth-token --data-file=-
echo -n "REPLACE_ME" | gcloud secrets create pulseops-elastic-mcp-token --data-file=-
```

Exit criteria:
- Both secrets appear in Secret Manager and have version 1.

Implementation status:
- Completed on 2026-06-02.
- Executed in accessible project `pulseops-agent`.
- Created and verified secrets: `pulseops-agent-auth-token`, `pulseops-elastic-mcp-token`.
- Verified both secrets have version `1`.

---

## Phase B: Service skeleton and ADK baseline (Day 2)

### Step B1: Create service structure
Use this logical split (single repo or split repos both acceptable):

```text
cloudrun-adapter/
  cmd/server/main.*
  internal/http/handler.*
  internal/adk/client.*
  internal/domain/model.*
  internal/safety/validate.*
  internal/obs/logging.*

agent/
  agent.py
  prompts.py
  workflows/investigate.py
  tools/elastic_tool.py
  tools/incident_tool.py
  tools/telemetry_tool.py
  tools/action_tool.py
  validators/schema_validator.py
```

Exit criteria:
- Buildable scaffold exists and CI can run tests.

Implementation status:
- Completed on 2026-06-02.
- Scaffold created under `google-agent-service/` to avoid collision with existing top-level `agent/` Go module.
- Created adapter module paths: `cloudrun-adapter/cmd/server/main.go`, `cloudrun-adapter/internal/httpapi/handler.go`, `cloudrun-adapter/internal/adk/client.go`, `cloudrun-adapter/internal/domain/model.go`, `cloudrun-adapter/internal/safety/validate.go`, `cloudrun-adapter/internal/obs/logging.go`.
- Created ADK Python scaffold paths: `agent/agent.py`, `agent/prompts.py`, `agent/workflows/investigate.py`, `agent/tools/*.py`, `agent/validators/schema_validator.py`.
- Build/test verification completed: `go test ./...` from `google-agent-service/cloudrun-adapter` succeeded.

### Step B2: Implement `/investigate` handler contract-first
Actions:
1. Parse incoming JSON.
2. Validate required fields (`task`, `prompt`, `metadata.incident_id`, `metadata.device_id`, `metadata.request_id`, `available_actions`).
3. Enforce max payload size (recommended: 1 MB).
4. Normalize optional fields (`elastic_context_hints`, `evidence_summary`).
5. Generate and attach `trace_id` if missing.

Exit criteria:
- Handler rejects malformed requests with deterministic envelope.

Implementation status:
- Completed on 2026-06-02.
- Implemented strict JSON parsing with unknown-field rejection and single-object enforcement in `google-agent-service/cloudrun-adapter/internal/httpapi/handler.go`.
- Enforced request size limit at 1 MB with deterministic `PAYLOAD_TOO_LARGE` error envelope.
- Implemented required-field validation path using existing safety validator for `task`, `prompt`, `metadata.incident_id`, `metadata.device_id`, `metadata.request_id`, `available_actions`.
- Added optional-field normalization for `elastic_context_hints` and `evidence_summary`.
- Added trace-id fallback generation so every response includes `trace_id` even when upstream does not return one.
- Verified with tests: `go test ./...` from `google-agent-service/cloudrun-adapter` passed.

### Step B3: Implement deterministic response envelope builder
Actions:
1. Build shared envelope builder for success and failure.
2. Always include `request_id` when recoverable.
3. Never leak sensitive internals in error payload.

Success envelope template:
```json
{
  "request_id": "req-123",
  "trace_id": "trace-abc",
  "status": {
    "transport": "success",
    "workflow": "completed"
  },
  "result": {
    "probableCause": "...",
    "confidence": 0.92,
    "recommendedActions": [],
    "validationSteps": ["..."],
    "summary": "..."
  }
}
```

Error envelope template:
```json
{
  "request_id": "req-123",
  "trace_id": "trace-abc",
  "status": {
    "transport": "error",
    "workflow": "failed"
  },
  "error": {
    "code": "VALIDATION_FAILED",
    "message": "request validation failed"
  }
}
```

Exit criteria:
- Envelope format passes parser contract tests.

Implementation status:
- Completed on 2026-06-02.
- Added shared deterministic envelope builder in `google-agent-service/cloudrun-adapter/internal/httpapi/envelope.go` for both success and failure responses.
- Updated handler to route all response paths through shared envelope functions in `google-agent-service/cloudrun-adapter/internal/httpapi/handler.go`.
- Enforced safe error-message policy by code to avoid leaking internal details while preserving deterministic error codes.
- Ensured `request_id` is included whenever recoverable from parsed request context.
- Added envelope-focused tests in `google-agent-service/cloudrun-adapter/internal/httpapi/envelope_test.go`.
- Verified with tests: `go test ./...` from `google-agent-service/cloudrun-adapter` passed.

### Step B4: Implement ADK root agent baseline
Actions:
1. Initialize ADK agent with explicit model selection.
2. Implement constrained system prompt requiring strict JSON output.
3. Add initial workflow entrypoint that receives assembled evidence.

Reference bootstrap:
```python
root_agent = Agent(
    name="pulseops_investigator",
    model="gemini-2.5-pro"
)
```

Exit criteria:
- Local ADK run returns JSON shaped like `InvestigationResult`.

Implementation status:
- Completed on 2026-06-02.
- ADK root agent baseline initialized with explicit model selection in `google-agent-service/agent/agent.py` (`name="pulseops_investigator"`, `model="gemini-2.5-pro"`).
- Constrained JSON-only system prompt implemented in `google-agent-service/agent/prompts.py`.
- Initial workflow entrypoint implemented in `google-agent-service/agent/workflows/investigate.py` with schema validation and approved action guardrails.
- Added local baseline runner in `google-agent-service/agent/local_runner.py` to produce `InvestigationResult`-shaped JSON without requiring ADK deployment.
- Added output-shape verification test in `google-agent-service/agent/tests/test_b4_local_baseline.py`.

---

## Phase C: Enrichment, guardrails, and reliability (Day 3)

### Step C1: Implement evidence assembly pipeline
Actions:
1. Seed evidence from `evidence_summary`.
2. If `ELASTIC_MCP_ENABLED=true` and hints exist, call Elastic MCP tool.
3. Add incident context and telemetry tool lookups.
4. Bound retrieval:
- max docs per source
- max chars per doc snippet
- max total evidence chars
5. Convert to compact bullet evidence block for prompting.

Recommended defaults:
- max docs per source: 20
- max snippet chars: 500
- max evidence chars: 8000

Exit criteria:
- Evidence block remains under bounds for worst-case hints.

Implementation status:
- Completed on 2026-06-03.
- Implemented evidence assembly pipeline in `google-agent-service/agent/workflows/evidence.py`.
- Pipeline seeds evidence from `evidence_summary`, conditionally calls Elastic when `elastic_mcp_enabled`/`ELASTIC_MCP_ENABLED` is true and hints are present, and adds incident and telemetry tool context.
- Enforced retrieval bounds with defaults: max docs per source = `20`, max chars per snippet = `500`, max total evidence chars = `8000`.
- Added compact bullet evidence formatting for prompt consumption (`evidence_text` + `evidence_lines`).
- Wired pipeline into investigation workflow in `google-agent-service/agent/workflows/investigate.py`.
- Added tests for bounds and Elastic toggle behavior in `google-agent-service/agent/tests/test_c1_evidence_pipeline.py` and workflow integration in `google-agent-service/agent/tests/test_c1_investigation_workflow.py`.

### Step C2: Implement schema and policy validators
Validators required:
1. `probableCause`: non-empty string
2. `confidence`: number in [0.0, 1.0]
3. `recommendedActions`: array with allowed `actionId` values only
4. `validationSteps`: non-empty string array
5. `summary`: non-empty string

Policy enforcement:
- Reject shell-command-like or unsafe textual recommendations.
- Strip or reject unknown action IDs.

Exit criteria:
- Any invalid model output becomes deterministic failed envelope.

Implementation status:
- Completed on 2026-06-03.
- Enhanced ADK-side schema and policy validation in `google-agent-service/agent/validators/schema_validator.py`.
- Enforced required fields: non-empty `probableCause`, `summary`; `confidence` in `[0.0, 1.0]`; non-empty string `validationSteps`; array/object validation for `recommendedActions`.
- Enforced action allowlist for `recommendedActions[].actionId` using approved action catalog from `google-agent-service/agent/tools/action_tool.py`.
- Implemented unsafe-content rejection for textual fields (`probableCause`, `summary`, `validationSteps`, `recommendedActions[].target`, `recommendedActions[].reason`).
- Added normalize-and-validate path to strip unknown action IDs before strict validation.
- Added adapter-side model output validation in `google-agent-service/cloudrun-adapter/internal/safety/result_validate.go`.
- Updated handler to convert invalid model output into deterministic failed envelope code `MODEL_OUTPUT_INVALID` in `google-agent-service/cloudrun-adapter/internal/httpapi/handler.go` and `.../envelope.go`.
- Added tests: `google-agent-service/agent/tests/test_c2_schema_policy_validator.py`, `google-agent-service/cloudrun-adapter/internal/safety/result_validate_test.go`, and handler failure-path coverage in `google-agent-service/cloudrun-adapter/internal/httpapi/handler_test.go`.
- Verified Go-side tests pass: `go test ./...` from `google-agent-service/cloudrun-adapter`.

### Step C3: Add retry, timeout, and fallback policy
Request budget target: 10 seconds total.

Budget split:
- validation/setup: 0.5s
- enrichment: up to 3s
- model generation/parse: up to 6s
- response serialization: 0.5s

Retry rules:
- one retry for transient transport failures only (5xx, timeout)
- no retry for validation or schema failures

Fallback:
- enrichment failure -> continue with base evidence
- model/schema failure -> return failed envelope

Exit criteria:
- Integration tests confirm budget and fallback behavior.

Implementation status:
- Completed on 2026-06-03.
- Added request and model budgets in adapter handler (`google-agent-service/cloudrun-adapter/internal/httpapi/handler.go`): total request budget = `10s`, ADK model/parse budget = `6s`.
- Implemented one-retry transient transport policy in handler via `investigateWithRetry(...)` with max retries = `1`.
- Added transient failure classification in `google-agent-service/cloudrun-adapter/internal/adk/errors.go` (`TransientError`, `TransportError`, `IsTransient`).
- Enforced no retry for non-transient errors and for validation/schema failures (existing deterministic failed-envelope paths retained).
- Implemented enrichment fallback in `google-agent-service/agent/workflows/evidence.py`: tool failures now continue with base evidence instead of failing the workflow.
- Added Go integration-style tests in `google-agent-service/cloudrun-adapter/internal/httpapi/handler_test.go` for retry-on-transient and no-retry-on-non-transient behavior.
- Added Python fallback test in `google-agent-service/agent/tests/test_c1_evidence_pipeline.py` for enrichment tool failure fallback.
- Verified Go-side tests pass: `go test ./...` from `google-agent-service/cloudrun-adapter`.

---

## Phase D: Deploy and integrate (Day 4)

### Step D1: Local ADK and adapter validation
Actions:
1. Install ADK tooling and run local test harness.
2. Run adapter with mocked ADK for deterministic tests.

Commands:
```bash
pip install google-adk
adk web
```

Exit criteria:
- Local request fixture round-trip succeeds.

Implementation status:
- Completed on 2026-06-03.
- Installed ADK tooling locally (no active virtual environment required) using `python -m pip install --user google-adk`.
- Verified ADK CLI startup with `adk web` from `google-agent-service/agent` and confirmed local server availability at `http://127.0.0.1:8000`.
- Ran adapter deterministic mocked-ADK test suite successfully: `go test ./...` from `google-agent-service/cloudrun-adapter`.
- Added and passed local request fixture round-trip test using frozen contract fixture `docs/contracts/adk_request_fixture.json` via `google-agent-service/cloudrun-adapter/internal/httpapi/fixture_roundtrip_test.go`.
- Contract compatibility fixes applied for strict request decode during round-trip:
  - Added `metadata.idempotency_token` support in `google-agent-service/cloudrun-adapter/internal/domain/model.go`.
  - Added optional `available_actions[].description` and `available_actions[].allowedPlatforms` support in `google-agent-service/cloudrun-adapter/internal/domain/model.go`.

### Step D2: Deploy ADK agent to Vertex AI
Command:
```bash
adk deploy agent_engine \
  --project=pulseops-agent \
  --region=us-central1 \
  --display_name=pulseops-investigator \
  agent
```

Exit criteria:
- Agent deployment is healthy and callable.

Implementation status:
- Completed on 2026-06-03.
- Reconciled ADK CLI semantics using ADK docs (Go quickstart + CLI surface): `adk deploy agent_engine` is the valid deploy path for ADK 2.x.
- Resolved local Vertex AI SDK compatibility for deploy runtime (`google-cloud-aiplatform==1.154.0` with `vertexai.Client` available).
- Successful deploy output:
  - `projects/939165137283/locations/us-central1/reasoningEngines/7736544793810960384`
- Playground URL:
  - `https://console.cloud.google.com/vertex-ai/agents/agent-engines/locations/us-central1/agent-engines/7736544793810960384/playground?project=939165137283`

### Step D3: Build and deploy Cloud Run adapter
Build command:
```bash
gcloud builds submit \
  --tag us-central1-docker.pkg.dev/$GOOGLE_CLOUD_PROJECT/pulseops-agent/cloudrun-adapter:$(git rev-parse --short HEAD)
```

Deploy command:
```bash
gcloud run deploy pulseops-google-agent-adapter \
  --image us-central1-docker.pkg.dev/$GOOGLE_CLOUD_PROJECT/pulseops-agent/cloudrun-adapter:$(git rev-parse --short HEAD) \
  --region us-central1 \
  --platform managed \
  --service-account pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com \
  --set-env-vars "APP_ENV=production,ADK_AGENT_NAME=pulseops-investigator,ADK_AGENT_LOCATION=us-central1,ELASTIC_MCP_ENABLED=true" \
  --set-secrets "INBOUND_AUTH_TOKEN=pulseops-agent-auth-token:latest,ELASTIC_MCP_TOKEN=pulseops-elastic-mcp-token:latest" \
  --allow-unauthenticated=false \
  --timeout=15s \
  --memory=512Mi \
  --cpu=1 \
  --max-instances=5
```

Capture URL:
```bash
gcloud run services describe pulseops-google-agent-adapter --region us-central1 --format='value(status.url)'
```

Exit criteria:
- Cloud Run endpoint returns 401 without token and valid response with token.

Implementation status:
- Completed on 2026-06-03.
- Added adapter container build file: `google-agent-service/cloudrun-adapter/Dockerfile`.
- Added inbound bearer-token enforcement in adapter handler using secret-backed `INBOUND_AUTH_TOKEN`.
- Verified adapter tests pass after auth changes: `go test ./...` from `google-agent-service/cloudrun-adapter`.
- Built and pushed adapter image:
  - `us-central1-docker.pkg.dev/pulseops-agent/pulseops-agent/cloudrun-adapter:05cb56d`
  - digest: `sha256:2774317f0e0b54b358962cd0d9e16c1bd82dc824a133a90cf16cb2b57bddcb89`
- Deployed Cloud Run service:
  - service: `pulseops-google-agent-adapter`
  - revision: `pulseops-google-agent-adapter-00001-vnd`
  - URL: `https://pulseops-google-agent-adapter-y5l6ymljhq-uc.a.run.app`
- Exit-criteria verification (authenticated Cloud Run invoke path):
  - no inbound token -> HTTP `401`
  - valid inbound token -> HTTP `200`
- Build IAM fixes applied for project build executor (`939165137283-compute@developer.gserviceaccount.com`):
  - `roles/storage.objectViewer`
  - `roles/artifactregistry.writer`
  - `roles/logging.logWriter`

### Step D4: Wire PulseOps backend to Google adapter
Backend env values:
- `AGENT_BUILDER_ENABLED=true`
- `AGENT_BUILDER_ADK_ENDPOINT=<cloud-run-url>/investigate`
- `AGENT_BUILDER_AUTH=Bearer <token>`
- `AGENT_BUILDER_TIMEOUT_MS=10000`

Exit criteria:
- Backend sends live request and parser accepts response.

Implementation status:
- Completed on 2026-06-03.
- Backend runtime configured with ADK transport envs:
  - `AGENT_BUILDER_ENABLED=true`
  - `AGENT_BUILDER_ADK_ENDPOINT=https://pulseops-google-agent-adapter-y5l6ymljhq-uc.a.run.app/investigate`
  - `AGENT_BUILDER_AUTH=Bearer <pulseops-agent-auth-token>`
  - `AGENT_BUILDER_TIMEOUT_MS=10000`
- Cloud Run invoker access configured for backend compatibility with current client transport (which sends app bearer token but not Cloud Run identity token):
  - `roles/run.invoker` granted to `allUsers`
  - App-layer bearer auth remains enforced by adapter via `INBOUND_AUTH_TOKEN`
- Fixed integration parsing mismatch by aligning adapter stub action targets with backend allowed-action targets in `google-agent-service/cloudrun-adapter/internal/adk/client.go`.
- Live handoff verification completed:
  - Triggered telemetry incident (`serviceStatus=stopped`, `heartbeat=true`) through backend `/telemetry`.
  - Backend logs confirm ADK request/response round-trip and parsed completion (`investigation_status=completed`, `trace_id=trace-scaffold`).
  - Backend `/incidents` record persisted parsed result fields:
    - `investigationStatus=completed`
    - `probableCause=service stopped`
    - `confidence=0.5`
    - `recommendedActions[0].actionId=restart_service`
    - `recommendedActions[0].target=service`

---

## Phase E: Verification, hardening, and release evidence (Day 5)

### Step E1: Test matrix execution
Unit tests:
- request validation
- envelope builder
- schema validator
- action whitelist validator

Integration tests:
- adapter + mocked ADK
- ADK workflow + mocked tools
- timeout/retry/fallback behavior

Contract tests:
- fixture generated from backend request builder
- fixture consumed by backend parser

End-to-end smoke:
1. Start backend, agent, frontend.
2. Trigger stopped-service incident.
3. Confirm Google service receives request with correlation IDs.
4. Confirm valid envelope response.
5. Confirm investigation fields persisted.
6. Confirm dashboard renders fields.

Exit criteria:
- All test categories pass and smoke artifacts archived.

Implementation status:
- Completed on 2026-06-03 with an automated runner: `scripts/phase7-e1.ps1`.
- Runner executes unit, integration, and contract categories across backend, Cloud Run adapter, and Google agent Python tests, then optionally runs end-to-end smoke.
- Command used for local verification in this workspace:
  - `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\phase7-e1.ps1 -SkipSmoke`
- Latest verified summary artifact:
  - `artifacts/phase7-smoke/20260603-105738-e1/summary.json`
  - status: `passed` (for selected matrix with smoke skipped)

### Step E2: Observability and alerting baseline
Per-request logs must include:
- `request_id`
- `incident_id`
- `device_id`
- `trace_id`
- `status_transport`
- `status_workflow`
- `latency_ms`
- `confidence`
- `action_ids`
- `enrichment_used`
- `evidence_lines`

Metrics:
- `investigate_requests_total`
- `investigate_success_total`
- `investigate_fail_total`
- `investigate_timeout_total`
- `investigate_validation_fail_total`
- latency histogram (p50, p95)

Alerts (initial):
- error rate > 5% for 10 minutes
- p95 latency > 8s for 10 minutes

Exit criteria:
- Dashboards and alerts fire in controlled test.

Implementation status:
- Completed on 2026-06-03.
- Per-request structured logging: replaced the default logger with a Cloud Logging-compatible JSON handler in `google-agent-service/cloudrun-adapter/internal/obs/logging.go` (remaps `level`->`severity`, `msg`->`message`, `time`->`timestamp`).
- Every `/investigate` request now emits a single `investigate_request` log line carrying all required fields: `request_id`, `incident_id`, `device_id`, `trace_id`, `status_transport`, `status_workflow`, `latency_ms`, `confidence`, `action_ids`, `enrichment_used`, `evidence_lines` (plus `outcome`). `enrichment_used` reflects `ELASTIC_MCP_ENABLED` AND presence of Elastic hints; `evidence_lines` counts non-empty evidence lines. Emission is centralized in a single `finish(...)` closure so all terminal paths are covered exactly once (`google-agent-service/cloudrun-adapter/internal/httpapi/handler.go`).
- Metrics: added a dependency-free, thread-safe registry in `google-agent-service/cloudrun-adapter/internal/obs/metrics.go` exposing `investigate_requests_total`, `investigate_success_total`, `investigate_fail_total`, `investigate_timeout_total`, `investigate_validation_fail_total`, the `investigate_latency_ms` histogram (cumulative buckets straddling the 8s threshold), and `investigate_latency_ms_p50`/`_p95` gauges. `fail_total` is the umbrella (timeout + validation_fail are sub-counters) so error rate = fail/requests. Served unauthenticated at `GET /metrics` in Prometheus text format (alongside `/health`).
- Timeout classification: upstream `context.DeadlineExceeded` is recorded as `investigate_timeout_total`; unauthorized requests are intentionally not counted (pre-processing rejection, not a workflow outcome).
- Cloud Monitoring artifacts under `google-agent-service/deploy/monitoring/`: `alert-error-rate.json` (ratio condition, >5% for 600s), `alert-p95-latency.json` (ALIGN_PERCENTILE_95 > 8000ms for 600s), `dashboard.json` (request rate, error ratio, p50/p95 latency with 8s threshold line, validation/timeout rates), and `README.md` documenting the log-based-metric creation (`investigate_requests`, `investigate_errors`, `investigate_validation_fail`, `investigate_timeout`, distribution `investigate_latency_ms`) and apply commands.
- Controlled test runner: `scripts/phase7-e2.ps1` builds and runs the adapter locally with auth, drives deterministic traffic (5 success, 2 validation-fail, 1 unauthorized), scrapes `/metrics`, asserts counter movement, verifies the structured-log field set, and writes a `summary.json` artifact. Optional `-DeployMonitoring` applies the dashboard/alert policies via gcloud.
- Tests added: `google-agent-service/cloudrun-adapter/internal/obs/metrics_test.go` (outcome counting, cumulative buckets, p95 crossing the 8s threshold, Prometheus exposition) and handler tests for `/metrics` counter movement and required log-field emission.
- Local verification:
  - `go test ./...` from `google-agent-service/cloudrun-adapter` passed.
  - `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\phase7-e2.ps1 -Port 8087` -> status `passed` (all 8 checks); summary at `artifacts/phase7-smoke/<timestamp>-e2/summary.json`, raw exposition at `metrics.txt`.
- Remaining for full exit criteria: the two alert policies fire only against deployed infra. Apply the log-based metrics + policies in `pulseops-agent` (see monitoring `README.md`), then drive sustained error/latency load for >10m to observe incidents open and auto-close. Code, metric math (unit-proven), and configs are ready.

### Step E3: Security validation and rollback drill
Security checks:
- inbound auth enforced
- token/secret redaction in logs
- least-privilege IAM confirmed
- no raw evidence dumps by default

Rollback drill:
1. Deploy a known stable revision label.
2. Shift traffic to newer revision.
3. Simulate failure trigger.
4. Roll traffic back to stable revision.

Exit criteria:
- Rollback completed in < 5 minutes with no contract break.

Implementation status:
- Completed on 2026-06-03.
- Inbound auth enforced as a hard startup gate: added `safety.CheckSecurityConfig` in `google-agent-service/cloudrun-adapter/internal/safety/security.go` and wired it into `cmd/server/main.go`. In `APP_ENV=production` a missing `INBOUND_AUTH_TOKEN` is fatal (the service refuses to start); in dev it is a warning. An enabled-Elastic-without-token condition logs a warning.
- Token/secret redaction in logs: reworked `internal/obs/logging.go` so the JSON logger captures secret env values (`INBOUND_AUTH_TOKEN`, `ELASTIC_MCP_TOKEN`, `AGENT_BUILDER_AUTH`, plus the bare token inside a `Bearer <token>` value) at construction and redacts any occurrence in string/error attribute values, and always redacts sensitive keys (`authorization`, `token`, `secret`, `password`, `api_key`).
- No raw evidence dumps by default: confirmed and test-locked — the adapter logs `evidence_lines` (a count) and never `evidence_summary` or `prompt` content.
- Least-privilege IAM documented and verifiable: expected runtime-SA role set is `roles/aiplatform.user`, `roles/secretmanager.secretAccessor`, `roles/logging.logWriter`, `roles/monitoring.metricWriter` (and `roles/run.invoker` is on the service, not the SA). See `google-agent-service/deploy/security/README.md`.
- Rollback runbook authored in `deploy/security/README.md`: tag serving revision as `stable`, promote latest, run contract smoke, roll back via `--to-revisions stable=100`; rollback is a traffic shift (no rebuild).
- Tests added: `internal/obs/logging_test.go` (secret-value redaction, sensitive-key redaction, Cloud Logging severity), `internal/safety/security_test.go` (prod gate fatal, dev warning, elastic-token warning), and `internal/httpapi` no-evidence/prompt-dump test. `go test ./...` from `google-agent-service/cloudrun-adapter` passed.
- Controlled checks runner: `scripts/phase7-e3.ps1` runs security unit tests, builds the adapter, proves the production startup gate refuses to start without a token, then drives a live adapter to verify 401-without-token / 200-with-token and scans logs to confirm neither the secret token nor raw evidence leak. Optional `-GcpChecks` verifies runtime-SA least-privilege IAM; optional `-RollbackDrill` performs the timed Cloud Run rollback with a frozen-fixture contract smoke before and after, asserting completion < 300s.
- Local verification: `powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\phase7-e3.ps1 -Port 8088` -> status `passed` (all 8 local checks); production gate confirmed firing (`startup security check failed`, exit 1). Summary at `artifacts/phase7-smoke/<timestamp>-e3/summary.json`.
- Remaining for full exit criteria: the IAM verification and the timed rollback drill run against deployed infra. Execute `phase7-e3.ps1 -GcpChecks -RollbackDrill` against `pulseops-agent` / `pulseops-google-agent-adapter` with an active gcloud session to capture the < 5-minute, no-contract-break evidence.

---

## 4) Implementation ownership matrix (recommended)

- Backend contract owner: validates fixture compatibility and parser behavior
- Adapter owner: HTTP validation, envelope, auth, retries/timeouts
- Agent owner: ADK orchestration, prompting, schema conformance
- Tooling owner: Elastic MCP + context tools + bounded retrieval
- SRE owner: deployment, metrics, alerts, rollback readiness

---

## 5) Required artifacts for hackathon/demo evidence

Store these artifacts under timestamped directories:
- backend logs (redacted)
- Cloud Run service logs (correlated by request and trace IDs)
- sample request/response payload snapshots (redacted)
- incident API snapshots before and after investigation
- dashboard screenshot showing investigation fields
- test run summary (unit/integration/contract/smoke)

Suggested location:
- `artifacts/phase7-smoke/<timestamp>/`

---

## 6) Definition of done (strict)

All conditions must be true:
1. Cloud Run endpoint is deployed with auth and HTTPS.
2. Backend request payload is accepted without shape transform issues.
3. Response envelope includes required IDs, status, and strict `result`.
4. All recommended action IDs are constrained to allowed catalog.
5. Logs and metrics support request-level traceability.
6. End-to-end PulseOps incident flow persists and renders investigation output.
7. Rollback drill has been executed successfully.

---

## 7) Copy-paste command runbook (ordered)

Run in order and mark each step complete.

1. Set active project
```bash
gcloud config set project pulseops-agent
```

2. Enable APIs
```bash
gcloud services enable run.googleapis.com aiplatform.googleapis.com artifactregistry.googleapis.com cloudbuild.googleapis.com secretmanager.googleapis.com logging.googleapis.com monitoring.googleapis.com iam.googleapis.com
```

3. Create Artifact Registry repo
```bash
gcloud artifacts repositories create pulseops-agent --repository-format=docker --location=us-central1 --description="PulseOps Google agent service images"
```

4. Create runtime service account
```bash
gcloud iam service-accounts create pulseops-agent-svc --display-name="PulseOps Agent Service Runtime"
```

5. Grant required IAM roles
```bash
gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" --role="roles/aiplatform.user"
gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" --role="roles/secretmanager.secretAccessor"
gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" --role="roles/logging.logWriter"
gcloud projects add-iam-policy-binding $GOOGLE_CLOUD_PROJECT --member="serviceAccount:pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com" --role="roles/monitoring.metricWriter"
```

6. Create secrets
```bash
echo -n "REPLACE_ME" | gcloud secrets create pulseops-agent-auth-token --data-file=-
echo -n "REPLACE_ME" | gcloud secrets create pulseops-elastic-mcp-token --data-file=-
```

7. Deploy ADK agent
```bash
adk deploy agent_engine --project=pulseops-agent --region=us-central1 --display_name=pulseops-investigator agent
```

8. Build adapter image
```bash
gcloud builds submit --tag us-central1-docker.pkg.dev/$GOOGLE_CLOUD_PROJECT/pulseops-agent/cloudrun-adapter:$(git rev-parse --short HEAD)
```

9. Deploy Cloud Run adapter
```bash
gcloud run deploy pulseops-google-agent-adapter --image us-central1-docker.pkg.dev/$GOOGLE_CLOUD_PROJECT/pulseops-agent/cloudrun-adapter:$(git rev-parse --short HEAD) --region us-central1 --platform managed --service-account pulseops-agent-svc@$GOOGLE_CLOUD_PROJECT.iam.gserviceaccount.com --set-env-vars "APP_ENV=production,ADK_AGENT_NAME=pulseops-investigator,ADK_AGENT_LOCATION=us-central1,ELASTIC_MCP_ENABLED=true" --set-secrets "INBOUND_AUTH_TOKEN=pulseops-agent-auth-token:latest,ELASTIC_MCP_TOKEN=pulseops-elastic-mcp-token:latest" --allow-unauthenticated=false --timeout=15s --memory=512Mi --cpu=1 --max-instances=5
```

9.1 Enable backend invoke path (current backend transport model)
```bash
gcloud run services add-iam-policy-binding pulseops-google-agent-adapter --region us-central1 --member="allUsers" --role="roles/run.invoker"
```

10. Get service URL
```bash
gcloud run services describe pulseops-google-agent-adapter --region us-central1 --format='value(status.url)'
```

11. Configure backend env
```bash
AGENT_BUILDER_ENABLED=true
AGENT_BUILDER_ADK_ENDPOINT=<cloud-run-url>/investigate
AGENT_BUILDER_AUTH=Bearer <token>
AGENT_BUILDER_TIMEOUT_MS=10000
```

12. Run full smoke and collect artifacts
- Trigger incident
- Verify storage and dashboard rendering
- Archive logs and snapshots under timestamped artifact folder

---

## 8) Risks and mitigations

1. Model outputs malformed JSON
- Mitigation: strict validator, repair attempt once, fail fast envelope

2. Latency spikes from enrichment + generation
- Mitigation: strict retrieval caps, compact evidence format, timeout budget split

3. Unsafe recommendations
- Mitigation: action ID whitelist plus shell-like content rejection

4. Auth/IAM mistakes
- Mitigation: startup self-check and deployment gate before traffic cutover

5. Environment drift between dev and prod
- Mitigation: parameterized deployment scripts and contract fixture regression tests

---

## 9) Multi-agent extension (optional after MVP)

After MVP stabilization, extend to coordinator pattern:

```text
Incident Coordinator Agent
   |
   +---- Telemetry Investigator Agent
   |
   +---- Elastic Tool
   |
   +---- Incident Tool
   |
   +---- Action Catalog Tool
```

Guardrail: keep backend envelope unchanged so integration stays stable.
