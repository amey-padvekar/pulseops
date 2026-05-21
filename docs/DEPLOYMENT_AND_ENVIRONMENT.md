# PulseOps AI Deployment and Environment Guide

Purpose: provide a single operational reference for environment setup and deployment readiness.

## Environment Inventory

| Component | Environment | URL/Host | Owner | Status |
|---|---|---|---|---|
| Frontend dashboard |  |  |  | Not Started |
| Backend orchestrator |  |  |  | Not Started |
| Endpoint agent demo host |  |  |  | Not Started |
| Elastic cluster/index endpoint |  |  |  | Not Started |
| Agent Builder endpoint |  |  |  | Not Started |

## Required Configuration (Template)

Populate actual values in your runtime config (do not commit secrets).

- `APP_ENV`
- `BACKEND_PORT`
- `FRONTEND_BASE_URL`
- `WS_ENDPOINT`
- `ELASTIC_ENDPOINT`
- `ELASTIC_INDEX_TELEMETRY`
- `ELASTIC_INDEX_INCIDENTS`
- `ELASTIC_API_KEY` (secret)
- `AGENT_BUILDER_ENDPOINT`
- `AGENT_BUILDER_AUTH` (secret)
- `GEMINI_PROJECT_OR_PROFILE`
- `MONITORED_SERVICE_NAME`
- `INCIDENT_VALIDATION_WINDOW_SECONDS`

## Phase 1 Environment Model (Step 4.5)

This section defines the environment variable inventory for agent, backend, and frontend.

### Canonical sample files

- `agent/.env.example`
- `backend/.env.example`
- `frontend/.env.example`

### Inventory by component

| Variable | Component | Required | Secret | Example / Default | Notes |
|---|---|---|---|---|---|
| `APP_ENV` | agent, backend | Yes | No | `development` | Runtime mode (`development`, `staging`, `production`). |
| `LOG_LEVEL` | agent, backend, frontend (`VITE_LOG_LEVEL`) | Yes | No | `info` | Logging verbosity baseline. |
| `AGENT_DEVICE_ID` | agent | Yes | No | `DEV-AGENT-01` | Unique endpoint identifier. |
| `AGENT_HEARTBEAT_INTERVAL_SEC` | agent | Yes | No | `10` | Telemetry heartbeat interval. |
| `MONITORED_SERVICE_NAME` | agent | Yes | No | `OpenVPNService` | Primary service/process monitored by agent. |
| `BACKEND_BASE_URL` | agent | Yes | No | `http://localhost:8080` | Backend ingestion base URL. |
| `AGENT_REQUEST_TIMEOUT_MS` | agent | No | No | `5000` | HTTP timeout for backend calls. |
| `ENABLE_SIMULATED_LOGS` | agent | No | No | `true` | Optional local demo toggle. |
| `NETWORK_CHECK_HOST` | agent | No | No | `8.8.8.8` | Optional connectivity probe target. |
| `BACKEND_PORT` | backend | Yes | No | `8080` | Backend HTTP port. |
| `FRONTEND_BASE_URL` | backend | No | No | `http://localhost:5173` | CORS/redirect reference for dashboard origin. |
| `INCIDENT_VALIDATION_WINDOW_SECONDS` | backend | No | No | `20` | Validation hold window for recovery checks. |
| `ELASTIC_ENDPOINT` | backend | Yes (Phase 5+) | No | `https://your-elastic-endpoint.example.com` | Elastic endpoint for indexing/querying. |
| `ELASTIC_INDEX_TELEMETRY` | backend | Yes (Phase 5+) | No | `telemetry-events-local` | Telemetry index alias/pattern. |
| `ELASTIC_INDEX_INCIDENTS` | backend | Yes (Phase 5+) | No | `incident-events-local` | Incident index alias/pattern. |
| `ELASTIC_INDEX_LOGS` | backend | No | No | `endpoint-logs-local` | Optional endpoint logs index alias/pattern. |
| `ELASTIC_API_KEY` | backend | Yes (Phase 5+) | Yes | `<set-in-local-.env-only>` | Secret. Never commit real value. |
| `AGENT_BUILDER_ENDPOINT` | backend | Yes (Phase 6+) | No | `https://your-agent-builder-endpoint.example.com` | Agent Builder workflow endpoint. |
| `AGENT_BUILDER_AUTH` | backend | Yes (Phase 6+) | Yes | `<set-in-local-.env-only>` | Secret auth token. Never commit real value. |
| `GEMINI_PROJECT_OR_PROFILE` | backend | Yes (Phase 6+) | No | `your-project-or-profile` | Profile/project reference used by orchestration layer. |
| `VITE_APP_ENV` | frontend | Yes | No | `development` | Frontend runtime mode marker. |
| `VITE_LOG_LEVEL` | frontend | No | No | `info` | Frontend log verbosity marker. |
| `VITE_API_BASE_URL` | frontend | Yes | No | `http://localhost:8080` | Backend API base URL consumed by UI. |
| `VITE_WS_URL` | frontend | No | No | `ws://localhost:8080/ws` | WebSocket endpoint for live updates. |

### Secret handling boundaries

- Commit only `.env.example` files with placeholders.
- Never commit `.env` files containing real secrets.
- Treat `ELASTIC_API_KEY` and `AGENT_BUILDER_AUTH` as secret values.
- Frontend variables (`VITE_*`) are exposed to browser bundles; do not put secrets in `VITE_*` variables.

### Local override behavior

- Base template: copy `.env.example` to `.env` in each component folder.
- Local machine overrides: use `.env.local` if your loader/runtime supports it.
- Environment precedence guideline: runtime environment variables > `.env.local` > `.env` > defaults in code.
- Keep defaults in code for non-sensitive local development only.

### No-secrets-committed control

- `.gitignore` already ignores `.env` and `.env.*` while allowing `.env.example`.
- Before commit, verify no real credentials are present with a quick review of staged files.

## Deployment Readiness Gates

- [ ] Frontend deployed and reachable publicly.
- [ ] Backend deployed and reachable from frontend and agent.
- [ ] Agent can connect to backend from demo machine.
- [ ] Elastic ingestion confirmed with recent telemetry.
- [ ] Agent Builder call path validated end-to-end.

## Local Command Sequence (Phase 1)

Run from repository root in PowerShell.

1. Install dependencies

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-deps.ps1
```

2. Start backend

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-backend.ps1
```

3. Start agent

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-agent.ps1
```

4. Start frontend

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-frontend.ps1
```

Optional helper to launch all three:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\run-all.ps1
```

5. Run startup verification

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\smoke-check.ps1
```

## Platform Assumptions (Phase 1)

- Primary validated setup path: Windows + PowerShell.
- Phase 1 scripts in `scripts/` are PowerShell-based for deterministic demo preparation.
- Linux/macOS support is possible, but not the primary acceptance path for Phase 1.

## Startup Troubleshooting Notes

1. Backend does not start on port `8080`
- Check for a conflicting process and free port `8080`.
- Re-run `scripts/run-backend.ps1`.

2. Frontend does not start on port `5173`
- Stop conflicting process on `5173` or launch with another port.

3. Smoke-check fails at backend health probe
- Verify `http://localhost:8080/healthz` returns `{"status":"ok"}` while backend is running.

4. Script execution blocked by policy
- Execute scripts with `powershell -ExecutionPolicy Bypass -File <script>`.

5. Env variables are missing at runtime
- Copy each `.env.example` to `.env` in `agent/`, `backend/`, and `frontend/`.
- Ensure secret placeholders are replaced only in local `.env` files.

## Smoke Test (Post-Deploy)

1. Confirm healthy endpoint status appears in dashboard.
2. Trigger monitored service failure.
3. Confirm incident detection and investigation initiation.
4. Confirm recommendation appears and approval is required.
5. Approve remediation and confirm execution feedback.
6. Confirm recovery validation and incident summary.

## Rollback/Fallback Notes

- Keep a last-known-good deployment tag ready.
- Keep a fallback demo environment or route.
- If cloud integration is unstable, use pre-seeded data path while preserving workflow visibility.

## Operational Contacts

| Area | Primary | Backup |
|---|---|---|
| Hosting/platform |  |  |
| Backend/API |  |  |
| Endpoint agent |  |  |
| AI/Agent Builder integration |  |  |
| Elastic integration |  |  |
