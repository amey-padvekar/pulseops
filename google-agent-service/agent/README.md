# PulseOps ADK Agent (Gemini + Elastic MCP)

The investigation agent: an ADK `LlmAgent` running **Gemini 2.5 Pro** that calls
the **Elastic MCP** server's tools (search / ES|QL / mappings) to gather
telemetry, incident, and log evidence, then returns a validated
`InvestigationResult`.

## Architecture (W2)

```
build_root_agent()  ->  LlmAgent(model=gemini-2.5-pro, tools=[Elastic McpToolset])
run_investigation() ->  live ADK+Gemini+MCP  (when configured)
                        └─ fallback ─> deterministic baseline (offline/demo/failure)
```

- `agent.py` — builds `root_agent` (synchronous, as Agent Engine requires) and the
  Elastic `McpToolset` (http or stdio).
- `config.py` — env-driven `ElasticMcpConfig` + pure connection-param builders.
- `workflows/adk_runner.py` — runs `root_agent` via ADK `InMemoryRunner`, extracts
  + validates the JSON.
- `workflows/investigate.py` — selects live vs deterministic baseline; the baseline
  preserves the existing offline behavior and bounds.
- Output is enforced by `validators/schema_validator.py` + the
  `restart_service | flush_dns | reconnect_vpn` allow-list (no `output_schema`,
  which ADK forbids alongside tools).

## Elastic MCP transport — Option A (chosen)

Point the agent at the **Elastic Serverless Agent Builder MCP endpoint** (GA on
Serverless). It's on the **Kibana** host, authenticated with a plain ApiKey header
— no Cloud Run / OIDC plumbing.

```
ELASTIC_MCP_TRANSPORT=http
ELASTIC_MCP_URL=https://<kibana-host>/api/agent_builder/mcp
ELASTIC_MCP_API_KEY=<encoded api key>     # sent as "Authorization: ApiKey <key>"
```

The API key needs (mint in Kibana → Dev Tools): cluster `monitor_inference`,
index `read`+`view_index_metadata` on `telemetry-events*`/`incident-events*`/
`endpoint-logs*`, and Kibana app `feature_agentBuilder.read`+`feature_actions.read`
(without the first → `403`).

### Option B — local dev fallback (standalone docker over stdio)
```
ELASTIC_MCP_TRANSPORT=stdio
ES_URL=https://<deployment>.es.<region>.gcp.elastic.cloud:443
ES_API_KEY=<read-only key>
```
Requires Docker; launches `docker run -i --rm docker.elastic.co/mcp/elasticsearch stdio`.

## Run locally

```powershell
cd google-agent-service
python -m venv .venv; .\.venv\Scripts\Activate.ps1
pip install -r agent/requirements.txt

# Deterministic baseline (no creds needed):
python -m agent.local_runner

# Live Gemini + Elastic MCP (needs GCP ADC + the env above):
gcloud auth application-default login
$env:AGENT_INVESTIGATION_BACKEND="adk"
python -m agent.local_runner
```

## Tests

```powershell
cd google-agent-service
python -m pytest agent/tests -q
```
Wiring tests (`test_c3_mcp_wiring.py`) verify the MCP URL/header/transport
construction without needing ADK or `mcp` installed; the evidence-pipeline bounds
tests are preserved.
