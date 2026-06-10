# Plan: Make Agent Builder + Elastic MCP *meaningful* (not stubs)

> Status: proposed · Created 2026-06-06 · Deadline 2026-06-11

## Context

The hackathon rules ([docs/rules.md:49-57](./rules.md), [rules.md:193-195](./rules.md)) require **three** things applied *meaningfully, not cosmetically*: **Gemini** reasoning, **Google Cloud Agent Builder** orchestration, and the **Elastic MCP server**. Stage-One judging is pass/fail on exactly this. Today is **2026-06-06**; submission deadline is **2026-06-11 (5 days)**.

Current reality — all three are scaffolds:
- **Gemini**: the ADK `Agent` is built with `gemini-2.5-pro` but never invoked; `run_investigation` returns hardcoded Python ([google-agent-service/agent/agent.py:27](../google-agent-service/agent/agent.py), [workflows/investigate.py:22-46](../google-agent-service/agent/workflows/investigate.py)).
- **Agent Builder**: the Cloud Run adapter returns a `StubClient` hardcoded result ([cloudrun-adapter/internal/adk/client.go:13-40](../google-agent-service/cloudrun-adapter/internal/adk/client.go), wired in [cmd/server/main.go:30](../google-agent-service/cloudrun-adapter/cmd/server/main.go)).
- **Elastic MCP**: no MCP anywhere — backend uses the direct ES REST SDK ([backend/internal/agentbuilder/retriever.go](../backend/internal/agentbuilder/retriever.go)); the Python `fetch_elastic_context` returns `{"items": []}` ([tools/elastic_tool.py:4-6](../google-agent-service/agent/tools/elastic_tool.py)).

The operational loop (detect→approve→execute→validate→summarize) is real and solid; only the "intelligence layer" is fake. This plan makes it real.

**Decisions locked (with the user):** Full GCP + Vertex AI; live Elastic Cloud; **official** Elastic MCP server; deploy the Python agent to **Vertex AI Agent Engine** (= Agent Builder) and have the **backend call it directly** (retire the Go adapter); run the Elastic MCP server as a **separate Cloud Run HTTP service** the agent reaches over streamable-HTTP.

### Verified against the codebase (2026-06-06)

Every file/line claim above was re-checked and is accurate: `agent.py:16-34` builds the ADK `Agent(gemini-2.5-pro)` but `run_local_baseline` calls the hardcoded `run_investigation` ([investigate.py:10-49](../google-agent-service/agent/workflows/investigate.py)); `fetch_elastic_context` returns `{"items": []}` ([elastic_tool.py:4-6](../google-agent-service/agent/tools/elastic_tool.py)); the Go adapter `StubClient` is canned ([client.go:13-40](../google-agent-service/cloudrun-adapter/internal/adk/client.go)); `requirements.txt` has only `google-adk` + `google-cloud-aiplatform`, **no `mcp`**. Three findings reshape the plan below:

> 1. **No MCP exists, yet code and docs already claim it.** The live backend path is the direct ES SDK retriever, but its call site is commented *"Retrieve compact evidence summary from Elastic MCP"* ([main.go:493](../backend/cmd/server/main.go)) and `README.md` has whole "Elastic MCP" sections (≈ lines 472, 550-560, 712, 1056-1067) describing a flow that isn't wired. Publishing MCP claims the code doesn't back is a Stage-One credibility risk → W4/W5 now include truth-in-advertising fixes.
> 2. **Two hard Stage-One items are entirely missing from this plan.** There is **no `LICENSE` file** (rules require an OSI license visible in the GitHub *About* section, [rules.md:140-141,197](./rules.md)) and **no plan to host the actual product** — the React dashboard + Go backend — at a public URL ([rules.md:122,196,212](./rules.md): "Hosted Project URL … live, accessible, functional"). Both are pass/fail. Added as W0 / W6.
> 3. **The leaked key is in committed history,** not just the working tree (`backend/.env.example` was committed as far back as `bfa5889`), and `.gitignore` *already* excludes `.env`/`.env.*` while keeping `.env.example`. So the original W0 "fix .gitignore" step is already done, and **rotation is mandatory, not optional** — scrubbing the file does not remove the value from the public history.

## Target architecture

```
agent(Go) → backend(Go) ── Vertex AI Agent Engine (ADK + Gemini 2.5 Pro)
                               │  uses MCPToolset (streamable-HTTP)
                               ▼
                       Elastic MCP server (Cloud Run, docker.elastic.co/mcp/elasticsearch http)
                               │  ES_URL + ES_API_KEY (read-only)
                               ▼
                          Elastic Cloud  ←── backend still indexes telemetry/incidents/logs (unchanged)
```

The backend keeps indexing into Elastic (Phase 5, unchanged). The agent **reads** that data back **through Elastic MCP** during Gemini reasoning — that is the "meaningful MCP" the judges score.

---

## Workstream 0 — Security + licensing fix (do first, blocks public repo)

`backend/.env.example:11-12` contains a **live Elastic Cloud endpoint + API key**, committed and present in git history. Repo must go public ([rules.md:139-141](./rules.md)).
- **Rotate** that Elastic API key in Elastic Cloud (user action — exact steps at execution time). **Mandatory**: the key is in committed history, so a public repo exposes it regardless of file scrubbing — rotation is what actually neutralizes it.
- Replace the value in [backend/.env.example](../backend/.env.example) with a placeholder; also scan `agent/.env.example` and `frontend/.env.example` for any other committed secret.
- `.gitignore` already excludes `.env` / `.env.*` while keeping `.env.example` — **verified, no change needed.** (Optional hardening: purge the old value from history with `git filter-repo`; rotation makes this cosmetic.)
- Create a **minimal-privilege** ES API key (read-only on `telemetry-events*`, `incident-events*`, `endpoint-logs*`) for the MCP server.
- **Add an OSI `LICENSE` file at the repo root** (none exists today — confirmed) — e.g. Apache-2.0 or MIT — and verify GitHub auto-detects it in the **About** section. Stage-One pass/fail ([rules.md:140-141,197](./rules.md)), independent of all MCP work; do it now so it's not forgotten at the deadline.

## Workstream 1 — Deploy Elastic MCP server (Cloud Run)

**Artifacts built** in [deploy/elastic-mcp/](../deploy/elastic-mcp/): `deploy.ps1` (idempotent deploy script, mirrors `scripts/phase7-*.ps1`), `service.yaml` (declarative alternative), `README.md`. No app code — uses the published image. **Running the deploy is a user action** (needs authenticated `gcloud` + project + billing).

Verified against the official README (2026-06-07):
- Run `docker.elastic.co/mcp/elasticsearch` with `http` as the container **arg** (not a flag), **port 8080**. MCP endpoint is `<url>/mcp` (← `ELASTIC_MCP_URL`); health is `<url>/ping`. Env: `ES_URL` + `ES_API_KEY` (read-only key) from Secret Manager; optional `ES_SSL_SKIP_VERIFY`.
- **The HTTP listener has no auth of its own** → Cloud Run IAM is the *only* gate. Deploy `--no-allow-unauthenticated`; the agent must send a Google **ID token** (`Authorization: Bearer …`) for the Cloud Run audience. The Agent Engine SA gets `roles/run.invoker` (granted in W3). Default `--ingress all` (IAM-gated); `internal` needs Direct VPC egress from Agent Engine.
- Tools exposed: `search`, `esql`, `get_mappings`, `list_indices`, `get_shards` (confirmed).

> **Decision before W3 — this service may be unnecessary.** The image is *deprecated* (security-only), superseded by the **Elastic Agent Builder MCP endpoint** native to Elastic **9.2.0+/Serverless**. The deployment URL (`*.es.*.gcp.elastic.cloud`) looks like Serverless, so the agent could point `ELASTIC_MCP_URL` directly at the Elastic-hosted MCP endpoint (auth via ES API key) and skip Cloud Run entirely. Trade-off table in [deploy/elastic-mcp/README.md](../deploy/elastic-mcp/README.md). Either choice still satisfies "meaningful Elastic MCP."
> **Cross-workstream risk (resolve in W2):** confirm ADK `MCPToolset` (streamable-HTTP) can attach + refresh a Google OIDC ID token for the private Cloud Run audience; if not, use the Serverless endpoint or an internal load balancer.

## Workstream 2 — Make the Python ADK agent real (Gemini + Elastic MCP) — ✅ IMPLEMENTED

**Decision: Option A — point the agent directly at the Elastic Serverless *Agent Builder MCP endpoint*** (`{KIBANA_URL}/api/agent_builder/mcp`, GA on Serverless), auth via a static `Authorization: ApiKey <key>` header. This drops the Cloud Run hop, the OIDC-token plumbing, and a documented ADK↔Cloud-Run streamable-HTTP hang bug ([adk-python #2615](https://github.com/google/adk-python/issues/2615)). W1's `deploy/elastic-mcp/` is kept as a fallback artifact; stdio-to-docker is kept as a local-dev transport.

Verified against the installed **ADK 2.1.0** source (not guessed): `from google.adk.tools.mcp_tool import McpToolset, StreamableHTTPConnectionParams, StdioConnectionParams`; `McpToolset(connection_params=StreamableHTTPConnectionParams(url=…, headers=…, timeout=…))`; `LlmAgent(model, name, instruction, tools=[toolset])`; `InMemoryRunner`. ADK 2.1.0 requires `mcp>=1.24,<2`.

What was built (`google-agent-service/agent/`):
- **`config.py`** (new) — env-driven `ElasticMcpConfig` + pure, ADK-free `streamable_http_kwargs` / `stdio_docker_argv` builders (unit-testable without ADK/mcp). Transports: `http` (Option A, default) | `stdio` (local docker) | `disabled`.
- **`agent.py`** — `build_elastic_mcp_toolset()` (http/stdio), `build_root_agent()` = `LlmAgent(gemini-2.5-pro, tools=[toolset])`, and module-level `root_agent` (synchronous, as Agent Engine requires). Import-safe without ADK/`mcp` (degrades to 0 tools).
- **`workflows/adk_runner.py`** (new) — runs `root_agent` via `InMemoryRunner`, extracts JSON, enforces it with `normalize_and_validate_investigation_result` + the `restart_service|flush_dns|reconnect_vpn` allow-list (no `output_schema`, per ADK constraint).
- **`workflows/investigate.py`** — live ADK+Gemini+MCP path when configured; **deterministic baseline fallback** on any failure/offline (keeps the guaranteed demo path). Gate `_live_mode_enabled()` correctly disables live mode when `mcp` is absent.
- **`prompts.py`** — system prompt now mandates *use the Elastic tools before concluding*; tool names are discovered via MCP (not hardcoded — Agent Builder exposes its own names).
- **`requirements.txt`** — `google-adk>=2.1.0,<3`, `mcp>=1.24,<2`, `google-cloud-aiplatform`.
- **Tests** — new `tests/test_c3_mcp_wiring.py` asserts URL/`ApiKey` header/transport construction without ADK/mcp; existing `assemble_evidence` bounds tests preserved. **18 passed.**
- **Docs/env** — `agent/README.md`, `agent/.env.example` (Option A primary, stdio fallback). `tools/elastic_tool.py` kept as the offline-only baseline fallback (documented), not the MCP path.

> **User action for W2 to go live (no code):** (1) grab the **Kibana** endpoint (not the ES data endpoint) → `ELASTIC_MCP_URL=<kibana>/api/agent_builder/mcp`; (2) **re-mint the W0 key with Agent Builder privileges**: cluster `monitor_inference`, index `read`+`view_index_metadata`, Kibana app `feature_agentBuilder.read`+`feature_actions.read` (missing the last → 403); (3) `pip install -r agent/requirements.txt`, `gcloud auth application-default login`, set `AGENT_INVESTIGATION_BACKEND=adk`, run `python -m agent.local_runner`.

## Workstream 3 — Deploy the agent to Vertex AI Agent Engine — ✅ ARTIFACTS BUILT (deploy = user action)

**Built:** [scripts/deploy-agent-engine.ps1](../scripts/deploy-agent-engine.ps1) (wraps `adk deploy agent_engine`, mirrors `phase7-*.ps1`), [google-agent-service/DEPLOY.md](../google-agent-service/DEPLOY.md), and `agent/__init__.py` now exposes `root_agent` so ADK's loader discovers it (verified: `import agent` → `agent.root_agent` is an `LlmAgent`).

Verified against the **installed ADK 2.1.0 CLI** (differs from older docs):
- `adk deploy agent_engine [opts] agent` — the `AGENT` positional is the folder; run from `google-agent-service/`. `--env_file`/`--requirements_file`/`--staging_bucket` are **deprecated**: the deployer auto-reads `agent/.env` for env vars and stages `agent/requirements.txt` (auto-adding `google-cloud-aiplatform[adk,agent_engines]`).
- Env baked from `agent/.env` (DEPLOY profile): `GOOGLE_GENAI_USE_VERTEXAI=true`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION=global`, `GEMINI_MODEL=gemini-2.5-flash`, `ELASTIC_MCP_TRANSPORT=http`, `ELASTIC_MCP_URL`, `ELASTIC_MCP_API_KEY`. **No `GOOGLE_API_KEY`** at deploy (the script guards against `USE_VERTEXAI=false`).
- **Region `us-central1`** for the engine (Agent Engine isn't in `asia-south1`); model `gemini-2.5-flash`.
- **No `roles/run.invoker` grant needed** — that was an Option-B (Cloud Run) artifact. With Option A the agent reaches Elastic's MCP directly via the `ApiKey` header, and Agent Engine gets Gemini access from its managed runtime. (This corrects the original W3 bullet.)
- Capture the printed **`reasoningEngines/...`** name → backend `AGENT_ENGINE_RESOURCE` (W4).

**User action:** `gcloud auth login` + `gcloud auth application-default login`, set the DEPLOY `agent/.env`, then `pwsh -File scripts/deploy-agent-engine.ps1 -Project pulseops-agent -Region us-central1`. Optional SDK smoke-test in DEPLOY.md.

## Workstream 4 — Backend: new Agent Engine transport, retire Go adapter — ✅ IMPLEMENTED

**Built + tested** (`go build ./...` + `go test ./...` green): `internal/agentbuilder/agentengine_client.go` (`AgentEngineClient` → `:streamQuery` with `{"classMethod":"stream_query","input":{message,user_id}}`, parses SSE/array/NDJSON ADK events → extracts InvestigationResult JSON → `RawPayload`; injected `TokenProvider` keeps it unit-testable) + `agentengine_client_test.go` (6 tests). `cmd/server/agentengine_auth.go` (package main) provides the ADC `oauth2/google` cloud-platform token source (isolated so the agentbuilder package needs no oauth2). `config.go` adds `Transport`/`AgentEngineResource`/`GoogleProject`/`GoogleLocation` (+ 60s default timeout for agent_engine, project/location inferred from the resource path). `main.go` wiring switched to `transport` with an `agent_engine` branch (summarySubmitter left nil → deterministic fallback summary). Verified contract against the installed SDK: AdkApp only registers `stream_query` (no non-streaming `query`). Added `golang.org/x/oauth2`. Fixed the misleading "Elastic MCP" comment at `main.go` and two pre-existing `go vet` printf errors. **Retire-adapter** note below still applies.


Backend `Client` interface is small — `SubmitInvestigation(ctx, AgentBuilderRequest) (AgentBuilderResponse, error)` ([backend/internal/agentbuilder/client.go:16](../backend/internal/agentbuilder/client.go)). Add a third transport alongside HTTP/ADK.

- **New file** `backend/internal/agentbuilder/agentengine_client.go`: `AgentEngineClient` implementing `Client`. It POSTs to the Vertex AI Agent Engine query endpoint
  `https://{location}-aiplatform.googleapis.com/v1/{resource}:query` (or `:streamQuery`), auth via Google ADC access token (`golang.org/x/oauth2/google`, `cloud-platform` scope). It maps the request → agent input, and the returned InvestigationResult JSON → `AgentBuilderResponse{RawPayload, TraceID, ReceivedAt}` so the **existing** parse/persist path in [backend/cmd/server/main.go:648-717](../backend/cmd/server/main.go) (`ParseInvestigationResultWithAllowedActions`, `SaveInvestigationResult`, `PromoteToAwaitingApproval`) works unchanged.
- **Config** [backend/internal/agentbuilder/config.go](../backend/internal/agentbuilder/config.go): add `AgentEngineResource`, `GoogleProject`, `GoogleLocation`, and a transport selector (`AGENT_BUILDER_TRANSPORT=agent_engine|http|adk`). Default: if `AGENT_ENGINE_RESOURCE` set → agent_engine. Keep `Enabled` semantics.
- **Wiring** [backend/cmd/server/main.go:788-840](../backend/cmd/server/main.go): add an `agent_engine` branch that builds `AgentEngineClient`; keep the existing enabled/disabled informative logs. `summarySubmitter` for Agent Engine: implement `SubmitSummary` on the new client (Phase 11) OR leave nil → deterministic fallback summary still works.
- **Retire the Go adapter**: stop referencing `google-agent-service/cloudrun-adapter` as the live path. Keep the folder (or move to `archive/`) but document it as superseded. No backend code depends on it directly — it's reached only via `AGENT_BUILDER_ENDPOINT`.
- Keep `AGENT_BUILDER_FALLBACK_MODE=local_stub_actions` working for offline demo (already in [main.go:575-643](../backend/cmd/server/main.go)).
- `go-elasticsearch` direct retriever ([retriever.go](../backend/internal/agentbuilder/retriever.go)) stays as an **offline/no-MCP fallback** and as the producer of `elastic_context_hints` — not deleted, just no longer the "MCP" story.
- **Truth-in-advertising:** correct the misleading *"Retrieve … from Elastic MCP"* comment at [main.go:493](../backend/cmd/server/main.go) — that call is the direct-ES retriever, not MCP. After this plan, the word "MCP" should only describe the W2 agent path. Reserve it there and nowhere else in code/docs.

## Workstream 5 — Env, docs, frontend

- [backend/.env.example](../backend/.env.example): add `AGENT_BUILDER_TRANSPORT`, `AGENT_ENGINE_RESOURCE`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_CLOUD_LOCATION`; set `AGENT_BUILDER_ENABLED=true`; placeholders only.
- Frontend already renders cause/confidence/recommendation ([frontend/src/pages/DashboardPage.tsx](../frontend/src/pages/DashboardPage.tsx)). **Recommended, not optional:** surface the MCP-derived evidence in the UI — an "evidence retrieved via Elastic MCP" badge **plus** the actual retrieved telemetry/log lines (or the ES|QL the agent ran). Visible MCP evidence is the single strongest proof of "meaningful, not cosmetic" for Stage One *and* scores directly on Stage-Two **Design** + **Technological Implementation** (equal-weighted). Cheap to add, high judging leverage.
- Rewrite the `README.md` "Elastic MCP" sections (≈ lines 472, 550-560, 712, 1056-1067) — they currently describe an MCP flow that isn't wired. Make them match the W1-W3 reality (Agent Engine → Elastic MCP on Cloud Run → Elastic Cloud). Update [docs/COMPLIANCE_EVIDENCE_MATRIX.md](./COMPLIANCE_EVIDENCE_MATRIX.md) and mark `docs/rules.md:193-195` satisfiable with evidence links.

## Workstream 6 — Host the product at a public URL (Stage-One pass/fail)

**Kit built** (chosen stack: Cloud Run backend + Firebase Hosting frontend + Compute Engine **Windows** VM agent — all GCP-native). Runbook: [docs/HOSTING.md](./HOSTING.md). Artifacts: [backend/Dockerfile](../backend/Dockerfile), [scripts/deploy-backend-cloudrun.ps1](../scripts/deploy-backend-cloudrun.ps1), [frontend/firebase.json](../frontend/firebase.json) + [scripts/deploy-frontend-firebase.ps1](../scripts/deploy-frontend-firebase.ps1), [scripts/create-agent-vm.ps1](../scripts/create-agent-vm.ps1) + [deploy/agent-vm/setup-agent.ps1](../deploy/agent-vm/setup-agent.ps1). Backend now honors `$PORT` (Cloud Run); WS already gates on `CORS_ALLOWED_ORIGIN` (set to the Firebase URL). Deploy = user action (gcloud/firebase auth + billing). Verified: backend builds, scripts parse, frontend typechecks.

The rules require a **live, accessible, functional** hosted Project URL ([rules.md:122,196,212](./rules.md)) — that means the *dashboard*, not the agent. None of W1-W5 hosts the actual product, so this is a standalone gap.
- Deploy the **Go backend** to **Cloud Run** (it already speaks HTTP and can use the same ADC identity it needs for the W4 Agent Engine call). Set `AGENT_BUILDER_TRANSPORT=agent_engine` + the W3 resource name in the service env.
- Deploy the **React dashboard** as a static site (Firebase Hosting, or a Cloud Run/Cloud Storage static bucket), built against the public backend URL + WebSocket origin. Set `CORS_ALLOWED_ORIGIN` / `FRONTEND_BASE_URL` to the hosted dashboard origin.
- The endpoint **agent (Go)** keeps running on the demo laptop and POSTs telemetry to the **hosted** backend over the internet — that is what makes the live "stop the service → watch the dashboard" demo real for judges.
- **Acceptance:** from a clean browser with no localhost, open the public dashboard, trigger an incident on the laptop agent, and watch the full detect → diagnose (via MCP) → approve → remediate → validate → resolve → summarize loop.
- If full cloud hosting slips, the deadline-safe fallback is a hosted dashboard + backend running `AGENT_BUILDER_FALLBACK_MODE=local_stub_actions` so the operational loop is still live at a public URL while the recorded video shows the real Gemini+MCP path (rules.md:100 — "function as depicted in the … video").

---

## Verification (end-to-end)

**Local (no deploy):**
1. `docker run -i --rm -e ES_URL=<live> -e ES_API_KEY=<readonly> docker.elastic.co/mcp/elasticsearch stdio` reachable; agent `local_runner.py` runs `root_agent`, logs show Gemini calling `search`/`esql` MCP tools, returning valid InvestigationResult JSON that passes `schema_validator`.
2. `go test ./...` in `backend` and `pytest` in `google-agent-service/agent` green.

**Deployed:**
3. Elastic MCP Cloud Run service responds (private, IAM-gated); agent on Agent Engine returns a real diagnosis for a seeded incident.
4. Backend with `AGENT_BUILDER_TRANSPORT=agent_engine` + resource → stop the monitored service → incident detected → backend calls Agent Engine → dashboard shows Gemini cause + recommendation **derived from MCP-retrieved evidence** → approve → execute → validate → resolved → summary.
5. Full demo script ([docs/DEMO_RUNBOOK.md](./DEMO_RUNBOOK.md)) runs under 3 min.

## Risks / fallbacks
- **Agent Engine + MCP latency** will exceed the current 10s `AGENT_BUILDER_TIMEOUT_MS` — Gemini 2.5 Pro doing several MCP tool round-trips realistically takes 20-60s. Mitigation: give the agent_engine transport a dedicated **~60s** timeout (separate from HTTP's 10s, set in W4 config); bound MCP result size via the W2 hints; pre-warm the engine before recording. Keep `local_stub_actions` as the guaranteed-working fallback path.
- **Elastic MCP image coordinates / tool names** — confirm the exact published image, run-mode flags (`http` vs `stdio`), and exposed tool set against Elastic's *current* MCP docs before W1. Don't burn deploy cycles on a guessed `docker.elastic.co/mcp/elasticsearch` tag; pin the version you actually pull and align W2's tool calls (`search`/`esql`/`get_mappings`) to it.
- **Agent Engine can't launch stdio subprocesses** — that's why MCP is a separate HTTP service (chosen). Local dev keeps stdio.
- **ADK `output_schema` + tools conflict** — handled via instruction + existing validators, not `output_schema`.
- **Time**: order is W0 → W2 (local Gemini+MCP working, the credibility core) → W1/W3 deploy → W4 backend wiring → W5 polish. If deploy slips, a locally-run agent + recorded demo still satisfies "functioning as depicted."

## Out of scope
- Reworking the operational loop (detect/approve/execute/validate) — already real.
- Elastic index schema changes — current indices suffice for MCP queries.

---

## Submission gating (non-code, but Stage-One pass/fail)

Not engineering work, but a missed one disqualifies the entry just as hard as a broken MCP call ([rules.md:118-141,210-218](./rules.md)):
- [ ] Public GitHub repo with an OSI **LICENSE** detected in the **About** section (W0).
- [ ] Live **Hosted Project URL** for the dashboard (W6).
- [ ] **Demo video ≤ 3:00**, English, on YouTube/Vimeo, showing the flow *functioning* (build on [DEMO_RUNBOOK.md](./DEMO_RUNBOOK.md)).
- [ ] **Text description**: features, tech stack (name Gemini + Vertex AI Agent Builder/Agent Engine + Elastic MCP explicitly), learnings.
- [ ] Devpost form submitted before **2:00 PM PT, 2026-06-11**; track = **Elastic**; all team members added.

> Note: first commit is `2026-05-21`, inside the May 5 – Jun 11 contest window, so the "newly created during the Contest Period" requirement ([rules.md:93](./rules.md)) is satisfied.

---

## Execution order (checklist)

- [~] **W0** Code done (scrubbed `.env.example`, Apache-2.0 LICENSE added); **pending user**: rotate leaked key + mint Agent-Builder-privileged read-only key
- [x] **W2** Real Python agent **DONE + VALIDATED LIVE** (2026-06-07) — Option A (Serverless Agent Builder MCP) + **Gemini 2.5 Flash** via ADK `InMemoryRunner` returned a genuine, schema-valid diagnosis end-to-end (not the fallback). 21 tests green; `mcp_smoke` lists 22 tools, agent allow-lists 7 read/query `platform_core_*`. Runtime: project `pulseops-agent`, `GOOGLE_CLOUD_LOCATION=global`, model `gemini-2.5-flash` (Pro 429s / asia-south1 404s on this new project).
- [~] **W1** Deploy script/config built in `deploy/elastic-mcp/`; **now a fallback** (Option A chosen) — deploy only if not using the Serverless built-in MCP
- [x] **W3** **DEPLOYED LIVE** (2026-06-07) to Vertex AI Agent Engine: `projects/pulseops-agent/locations/us-central1/reasoningEngines/7212240475082719232`. Fix needed during deploy: add `a2a-sdk` to requirements (ADK 2.2.0 api_server crash-loops on `ModuleNotFoundError: No module named 'a2a'` otherwise). No run.invoker grant (Option A).
- [x] **W4** Backend `AgentEngineClient` transport **done** — config + wiring + ADC oauth2 + 6 client tests; `go build`/`go test ./...` green; false "MCP" comment fixed. **Pending user**: set `AGENT_ENGINE_RESOURCE` + `AGENT_BUILDER_ENABLED=true` + ADC, run backend end-to-end
- [ ] **W5** Env/docs/frontend polish; rewrite README MCP sections; visible MCP-evidence badge; demo dry-run
- [~] **W6** Hosting kit built (Cloud Run + Firebase + Windows-VM agent; see [HOSTING.md](./HOSTING.md)); backend honors `$PORT`. **Pending user**: run the 3 deploy scripts (gcloud/firebase auth) → live `*.web.app` URL
- [ ] **Submission** LICENSE in About · hosted URL live · ≤3-min video · text description · Devpost (track=Elastic) before 2026-06-11 14:00 PT
