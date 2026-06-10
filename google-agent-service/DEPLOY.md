# Deploy the PulseOps agent to Vertex AI Agent Engine (W3)

Deploys the ADK `root_agent` (Gemini + Elastic MCP) to **Vertex AI Agent Engine**
— the Google Cloud "Agent Builder" runtime. The backend (W4) then calls this
deployed engine instead of running the agent locally.

```
backend(Go) --:query--> Vertex AI Agent Engine (root_agent: Gemini 2.5 Flash)
                              └─ streamable-HTTP + ApiKey ─> Elastic Serverless Agent Builder MCP
```

## Why no Cloud Run / IAM grant here
Because we chose **Option A** (the agent reaches Elastic's Serverless MCP endpoint
directly with a static `ApiKey` header), there is **no Cloud Run MCP service** and
therefore **no `roles/run.invoker` grant** to wire. Agent Engine gets Gemini access
from its managed runtime. The only Elastic credential is the API key in the env.

## Prerequisites (user actions)
```powershell
gcloud auth login
gcloud auth application-default login          # the deploy uses ADC
gcloud config set project pulseops-agent
```
- Billing enabled on `pulseops-agent` (done).
- Agent Engine runs in a supported region — use **`us-central1`** (not `asia-south1`).

## 1. Set `google-agent-service/agent/.env` for DEPLOY (Vertex mode)
The deployer reads this file and bakes the values into the deployment. Use Vertex
(not the AI Studio key) so the engine authenticates via its own service account:

```bash
# Gemini via Vertex (REQUIRED for Agent Engine)
GOOGLE_GENAI_USE_VERTEXAI=true
GOOGLE_CLOUD_PROJECT=pulseops-agent
GOOGLE_CLOUD_LOCATION=global          # model serving; global serves gemini-2.5-flash
GEMINI_MODEL=gemini-2.5-flash         # pro 429s on this project; flash is reliable
# IMPORTANT: do NOT set GOOGLE_API_KEY for deploy (that forces AI Studio/Express mode)

# Elastic MCP (Option A) — baked into the deployment env
ELASTIC_MCP_TRANSPORT=http
ELASTIC_MCP_URL=https://my-elasticsearch-project-aa48e8.kb.asia-south1.gcp.elastic.cloud/api/agent_builder/mcp
ELASTIC_MCP_API_KEY=<the Agent-Builder read-only key>
ELASTIC_MCP_AUTH_SCHEME=ApiKey
```
> The MCP API key is baked into the Agent Engine deployment env. Acceptable for the
> hackathon; for production move it to Secret Manager.

## 2. Deploy
```powershell
pwsh -NoProfile -File .\scripts\deploy-agent-engine.ps1 -Project pulseops-agent -Region us-central1
```
The script enables the Vertex AI API, then runs `adk deploy agent_engine agent`
from `google-agent-service/` (staging `agent/requirements.txt` and `agent/.env`
automatically). Deployment takes a few minutes.

## 3. Capture the resource name → backend (W4)
On success the deployer prints a `reasoningEngines/...` resource name. Record it:
```
AGENT_ENGINE_RESOURCE=projects/pulseops-agent/locations/us-central1/reasoningEngines/<ID>
```
This becomes the backend's `AGENT_ENGINE_RESOURCE` in W4.

## 4. Smoke-test the deployed engine (optional)
```python
# pip install "google-cloud-aiplatform[adk,agent_engines]"
import vertexai
from vertexai import agent_engines

vertexai.init(project="pulseops-agent", location="us-central1")
engine = agent_engines.get("projects/pulseops-agent/locations/us-central1/reasoningEngines/<ID>")

prompt = ("Investigate: device dev-300, service OpenVPNService is stopped while "
          "heartbeat=true and networkReachable=true. Return InvestigationResult JSON.")
for event in engine.stream_query(user_id="smoke", message=prompt):
    print(event)
```
Expect a tool-using run that ends in InvestigationResult-shaped JSON.

## Updating vs recreating
Re-running the script creates a **new** engine each time. To update the existing
one in place, pass its id:
```powershell
pwsh -NoProfile -File .\scripts\deploy-agent-engine.ps1 -AgentEngineId <ID>
```

## Troubleshooting
- **429 RESOURCE_EXHAUSTED** at query time → keep `gemini-2.5-flash`; if it persists,
  try `GOOGLE_CLOUD_LOCATION=us-central1`.
- **404 model not found** → wrong region/model; `gemini-2.5-flash` + `global` is known-good.
- **Import validation failure** → run `pip install -r agent/requirements.txt` in the
  deploy environment so `mcp` / `google-adk` import cleanly.
- **`code: 13` rollback, logs show `ModuleNotFoundError: No module named 'a2a'`** →
  the deployed api_server needs `a2a-sdk` (already in `requirements.txt`). If you see
  this, confirm `a2a-sdk>=0.3.4,<0.4` is present and redeploy. Read build logs with:
  `gcloud logging read 'resource.type="aiplatform.googleapis.com/ReasoningEngine"' --project pulseops-agent --freshness=3h --order=desc --limit=80 --format="value(textPayload)"`
- **Elastic 401/403 from the deployed agent** → the baked `ELASTIC_MCP_API_KEY` lacks
  `feature_agentBuilder.read`; re-mint (see W0/W2 notes) and redeploy.
