# Contract Freeze Notes for Step A1

Date: 2026-06-02
Owner: PulseOps backend and Google agent integration
Status: Frozen for Phase A1

## Objective
Freeze one canonical request fixture produced by the backend ADK request builder and one canonical response fixture accepted by the backend parser.

## Frozen artifacts
- docs/contracts/adk_request_fixture.json
- docs/contracts/adk_response_fixture.json

## Source of truth used
- backend/internal/agentbuilder/adk_client.go
- backend/internal/agentbuilder/prompt.go
- backend/internal/agentbuilder/model.go
- backend/internal/agentbuilder/investigation_model.go
- backend/internal/agentbuilder/parser.go
- backend/internal/agentbuilder/adk_client_test.go
- backend/internal/agentbuilder/parser_test.go

## Request fixture contract notes
The request fixture keys and shapes follow ADKRequestPayload exactly:
- task
- prompt
- metadata.incident_id
- metadata.device_id
- metadata.request_id
- metadata.idempotency_token (optional and included in fixture)
- elastic_context_hints
- available_actions
- evidence_summary

## Response fixture contract notes
The response fixture is the strict InvestigationResult payload consumed by parser validation:
- probableCause: non-empty
- confidence: [0.0, 1.0]
- recommendedActions: approved action IDs only
- validationSteps: non-empty
- summary: non-empty

## Verification commands
Run from backend module root:

go test ./internal/agentbuilder -run TestBuildADKRequestPayload_IncludesTraceMetadata -v

go test ./internal/agentbuilder -run TestParseInvestigationResult_Valid -v

## Change control
Any change to the fixture keys, casing, or required fields requires:
1. Updating adk_client.go and/or parser.go behavior.
2. Updating corresponding tests.
3. Updating these frozen fixture files and this note.
4. Approval from backend contract owner.
