# Shared Contracts

The `shared` directory is the source of truth for cross-component payload contracts.

## Ownership rule

- Update schemas here first when changing telemetry, incident, or remediation payloads.
- Backend validates inbound and outbound payloads against these schemas.
- Agent and frontend must consume payloads that conform to these schemas.

## Current schema set

- `telemetry.schema.json`
- `incident.schema.json`
- `remediation.schema.json`

## Versioning rule

- `schemaVersion` is required on all contracts.
- Backward-incompatible schema changes must increment schema version before rollout.
