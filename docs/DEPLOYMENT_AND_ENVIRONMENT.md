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

## Deployment Readiness Gates

- [ ] Frontend deployed and reachable publicly.
- [ ] Backend deployed and reachable from frontend and agent.
- [ ] Agent can connect to backend from demo machine.
- [ ] Elastic ingestion confirmed with recent telemetry.
- [ ] Agent Builder call path validated end-to-end.

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
