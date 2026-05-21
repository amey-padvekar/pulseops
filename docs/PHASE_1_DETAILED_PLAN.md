# PulseOps AI Phase 1 Detailed Plan

Phase: 1 - Repository and project setup  
Track: Elastic  
Contest deadline: June 11, 2026 at 2:00 PM PT

---

## 1) Phase objective

Build a clean, runnable baseline for three components:
- endpoint agent (Go)
- backend API (Go)
- frontend dashboard (React)

At the end of Phase 1, all three components must run locally and share the same foundational contracts, config conventions, and development workflow.

This plan is intentionally optimized for hackathon speed while preserving compliance with contest rules and later-phase extensibility.

---

## 2) Rule-aware constraints for Phase 1

These constraints from docs/rules.md must influence setup decisions now:

1. Required stack alignment
- Use Gemini and Google Cloud Agent Builder in later phases.
- Use Elastic MCP meaningfully in later phases.
- Do not introduce alternate AI/cloud competitors into architecture, dependencies, or docs.

2. Functional submission requirement
- The project must be installable and runnable as described.
- Phase 1 must produce reproducible local startup instructions.

3. New project requirement
- Keep this repository clearly new and contest-period scoped.
- Avoid importing old private project code with ambiguous provenance.

4. Public repo and open-source license requirement
- Plan for root LICENSE presence before submission.
- Keep setup docs ready for public consumption and judge reproducibility.

5. Demo and submission readiness
- Documentation and scripts should reduce setup risk for final demo and hosted deployment.

---

## 3) Phase 1 definition of done

Phase 1 is complete only when all are true:

1. Folder structure exists and matches the architecture plan.
2. Agent builds and runs a minimal skeleton process.
3. Backend builds and runs a minimal API server with health endpoint.
4. Frontend installs and runs with a basic shell page.
5. Shared schema files exist and are versioned in shared.
6. Environment variables are documented centrally.
7. One-command or short-command local startup exists for all components.
8. Phase acceptance checks in docs/PHASE_ACCEPTANCE_CRITERIA.md for Phase 1 are satisfied.

---

## 4) Work breakdown structure

### 4.1 Repository structure and ownership boundaries

Create and validate this structure:

- agent/
  - cmd/agent/
  - internal/telemetry/
  - internal/health/
  - internal/logs/
  - internal/remediation/
  - internal/platform/
- backend/
  - cmd/server/
  - internal/api/
  - internal/ws/
  - internal/incidents/
  - internal/telemetry/
  - internal/remediation/
  - internal/elastic/
  - internal/agentbuilder/
  - internal/store/
- frontend/
  - src/components/
  - src/pages/
  - src/hooks/
  - src/types/
- shared/
  - telemetry.schema.json
  - incident.schema.json
  - remediation.schema.json
- scripts/

Output:
- deterministic skeleton that matches implementation plan and reduces refactor churn in later phases.

### 4.2 Go module initialization and baseline binaries

Agent setup tasks:
1. Initialize module in agent.
2. Add cmd/agent entrypoint with structured logging and config load stub.
3. Add internal package placeholders with README notes or minimal Go files to avoid empty-package confusion.
4. Confirm go mod tidy and build pass.

Backend setup tasks:
1. Initialize module in backend.
2. Add cmd/server entrypoint.
3. Expose health route (for example /healthz) returning status JSON.
4. Add internal package placeholders for api/ws/incidents/telemetry/remediation/elastic/agentbuilder/store.
5. Confirm go mod tidy and build pass.

Output:
- runnable Go skeletons with predictable entrypoints for future telemetry and remediation logic.

### 4.3 Frontend scaffold and baseline UI shell

Tasks:
1. Create React app in frontend with stable package manager lockfile.
2. Add a minimal layout with placeholder cards for:
- endpoint health
- incident timeline
- AI investigation
- remediation approval
3. Add API base URL config pattern (env-driven).
4. Confirm local dev server boot.

Output:
- working dashboard shell that can receive live state in Phase 3 onward.

### 4.4 Shared contracts and schema versioning

Tasks:
1. Create initial JSON schemas in shared:
- telemetry.schema.json
- incident.schema.json
- remediation.schema.json
2. Add top-level schema metadata fields:
- schemaVersion
- generatedAt or timestamp requirements
- required fields and types aligned to implementation plan
3. Document schema ownership rule:
- shared is source of truth
- backend validates inbound payloads against shared definitions

Output:
- single source of truth for cross-component payload contracts.

### 4.5 Environment configuration model

Tasks:
1. Define environment variable inventory for agent/backend/frontend.
2. Create sample env files (non-secret) with placeholders.
3. Document secret handling boundaries and local override behavior.
4. Ensure no secrets are committed.

Suggested minimal env groups:
- app runtime: ports, log level, environment mode
- backend connectivity: Elastic endpoints, Agent Builder endpoint placeholders
- frontend API: backend base URL
- agent identity: device ID, monitored service name, heartbeat interval

Output:
- predictable config model that supports local dev now and cloud deployment later.

### 4.6 Developer workflow and scripts

Tasks:
1. Add scripts for:
- install dependencies
- run agent
- run backend
- run frontend
- optional run-all helper
2. Add basic smoke-check script for local startup verification.
3. Document expected command sequence in docs and README.

Output:
- lower setup friction and higher reproducibility for team and judges.

### 4.7 Documentation completion for Phase 1

Tasks:
1. Update setup documentation with exact prerequisites.
2. Add quickstart section for first-time clone and run.
3. Record known platform assumptions (Windows or Linux path for demo OS).
4. Add troubleshooting notes for common startup issues.

Output:
- docs that satisfy functional-installable expectation from rules and support demo confidence.

---

## 5) Sequence and execution order

Recommended order:

1. Create folder structure and empty ownership boundaries.
2. Initialize Go modules and compile both binaries.
3. Scaffold frontend and verify it runs.
4. Add shared schemas and contract notes.
5. Add env samples and config docs.
6. Add scripts and smoke checks.
7. Run full local startup rehearsal.
8. Mark acceptance checklist evidence.

Why this order:
- early compile/run checks reduce hidden setup debt
- contract-first setup avoids drift between agent/backend/frontend
- scripts and docs at the end capture what is proven working

---

## 6) Deliverables and evidence artifacts

Deliverables to produce in this phase:

1. Runnable skeletons
- agent runnable entrypoint
- backend runnable entrypoint + health route
- frontend runnable shell

2. Shared contracts
- initial telemetry, incident, remediation schemas

3. Environment and scripts
- env examples
- startup scripts
- smoke-check path

4. Documentation
- setup + quickstart
- assumptions and troubleshooting

Evidence to capture for compliance matrix:
- terminal proof of clean install and run
- screenshots of all three services running
- pointer to quickstart docs

---

## 7) Acceptance gates mapped to existing docs

Use these as pass/fail gates before moving to Phase 2.

Gate A: Component startup
- agent starts without runtime panic
- backend serves health endpoint
- frontend starts and renders shell

Gate B: Contract baseline
- shared schemas present and readable by team
- required minimal fields are defined

Gate C: Config baseline
- env vars documented
- sample env files exist and are non-secret

Gate D: Developer reproducibility
- fresh clone can run with documented steps
- scripts execute without manual hidden tweaks

Gate E: Compliance alignment
- no prohibited competing AI/cloud dependencies introduced
- architecture docs still position Gemini + Agent Builder + Elastic MCP as intended stack

These gates align with docs/PHASE_ACCEPTANCE_CRITERIA.md and docs/COMPLIANCE_EVIDENCE_MATRIX.md.

---

## 8) Risks and mitigations in Phase 1

Risk 1: Tooling sprawl and setup delay
- Mitigation: keep stack minimal; avoid optional infra in Phase 1 unless blocking.

Risk 2: Contract drift across components
- Mitigation: treat shared schemas as source of truth from day one.

Risk 3: Environment inconsistency across machines
- Mitigation: pin versions where possible and provide quickstart with prerequisites.

Risk 4: Rule drift (accidental prohibited service use)
- Mitigation: perform dependency and architecture review at end of Phase 1.

Risk 5: Overbuilding before core loop
- Mitigation: stop at runnable skeleton and contract clarity; defer advanced features to later phases.

---

## 9) Suggested time-box plan (single day)

Block 1 (60 to 90 min)
- structure, Go modules, compile checks

Block 2 (60 to 90 min)
- frontend scaffold and local run

Block 3 (45 to 60 min)
- shared schemas and env model

Block 4 (45 to 60 min)
- scripts, docs, smoke-run, acceptance gate pass

Total expected Phase 1 effort:
- about 4 to 6 focused hours

---

## 10) Exit checklist

- [ ] Folder structure created and committed
- [ ] agent builds and runs
- [ ] backend builds and serves health endpoint
- [ ] frontend starts and renders shell
- [ ] shared schemas added
- [ ] env variables documented with sample files
- [ ] startup scripts added
- [ ] quickstart documentation updated
- [ ] Phase 1 acceptance gates marked pass
- [ ] compliance matrix updated with Phase 1 evidence notes

---

## 11) Guardrail reminder for next phases

Phase 1 should set up, not dilute, the core hackathon narrative:
- Gemini for reasoning
- Google Cloud Agent Builder for orchestration
- Elastic MCP for operational context

Keep all setup choices compatible with that story to avoid costly rewrites in Phases 5 to 7.
