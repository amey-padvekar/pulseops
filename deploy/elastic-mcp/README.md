# Elastic MCP server on Cloud Run (Workstream 1)

Deploys the **official** Elastic MCP server (`docker.elastic.co/mcp/elasticsearch`)
as a **private** Cloud Run service in **streamable-HTTP** mode. The agent
(Workstream 2/3, on Vertex AI Agent Engine) reaches it over HTTP and uses its
tools — `search`, `esql`, `get_mappings`, `list_indices`, `get_shards` — to pull
telemetry/incident/log evidence from Elastic during Gemini reasoning.

There is **no application code** here — only deploy glue (`deploy.ps1`,
`service.yaml`). It runs the published image as-is.

```
Agent Engine (ADK + Gemini)  --streamable-HTTP /mcp-->  Cloud Run (this service)
                                                            │ ES_URL + ES_API_KEY (read-only, Secret Manager)
                                                            ▼
                                                        Elastic Cloud
```

---

## Verified facts (against the official README, 2026-06-07)

| Item | Value |
|---|---|
| Image | `docker.elastic.co/mcp/elasticsearch` |
| HTTP mode | pass `http` as the container **arg** |
| Port | `8080` |
| MCP endpoint | `http://<host>:8080/mcp`  ← this is `ELASTIC_MCP_URL` |
| Health check | `http://<host>:8080/ping` |
| Env vars | `ES_URL`, `ES_API_KEY` (or `ES_USERNAME`+`ES_PASSWORD`); optional `ES_SSL_SKIP_VERIFY=true` |
| Tools | `list_indices`, `get_mappings`, `search`, `esql`, `get_shards` |

> **Security:** the MCP server's HTTP listener has **no authentication of its own**.
> The ES key is baked into the container, so anyone who can reach the URL can query
> Elasticsearch. **Cloud Run IAM (private) is the only gate** — deploy with
> `--no-allow-unauthenticated` and never add an `allUsers` binding.

> **Deprecation:** Elastic marks this image as deprecated (critical-security-updates
> only), superseded by the **Elastic Agent Builder MCP endpoint** built into Elastic
> **9.2.0+ / Serverless**. See "Alternative" below before deploying — if your Elastic
> deployment exposes that endpoint, you can skip this Cloud Run service entirely.

---

## Prerequisites

1. **W0 done:** a **read-only** ES API key (encoded form) scoped to
   `telemetry-events*`, `incident-events*`, `endpoint-logs*` with `cluster: ["monitor"]`.
   Save its `encoded` value to a local file, e.g. `es-readonly-key.txt` (git-ignored).
2. `gcloud` authenticated and pointed at the project:
   ```
   gcloud auth login
   gcloud config set project pulseops-agent
   ```
3. Billing enabled on the project; you have `roles/run.admin` +
   `roles/secretmanager.admin` (or `owner`).

---

## Deploy (recommended path: deploy.ps1)

```powershell
pwsh -NoProfile -File .\deploy\elastic-mcp\deploy.ps1 `
    -Project pulseops-agent `
    -Region us-central1 `
    -EsUrl "https://your-deployment.es.asia-south1.gcp.elastic.cloud:443" `
    -EsApiKeyFile .\es-readonly-key.txt
```

The script (idempotent):
1. enables `run` + `secretmanager` APIs,
2. creates the secret `elastic-mcp-es-api-key` and adds your key as a version,
3. deploys the image in `http` mode, **private**, port 8080, with `ES_URL` (env)
   and `ES_API_KEY` (from Secret Manager),
4. optionally grants the Agent Engine SA `roles/run.invoker` (pass
   `-AgentEngineServiceAccount` after W3),
5. prints the **`ELASTIC_MCP_URL`** ( = service URL + `/mcp`) and verifies `/ping`.

### Alternative: declarative manifest
```
# create the secret first (see below), then:
gcloud run services replace deploy/elastic-mcp/service.yaml `
  --region us-central1 --project pulseops-agent
```

### Create the secret manually (if not using deploy.ps1)
```
gcloud secrets create elastic-mcp-es-api-key --replication-policy=automatic --project pulseops-agent
gcloud secrets versions add elastic-mcp-es-api-key --data-file=.\es-readonly-key.txt --project pulseops-agent
# grant the Cloud Run runtime SA read access:
gcloud secrets add-iam-policy-binding elastic-mcp-es-api-key `
  --member "serviceAccount:$(gcloud projects describe pulseops-agent --format='value(projectNumber)')-compute@developer.gserviceaccount.com" `
  --role roles/secretmanager.secretAccessor --project pulseops-agent
```
(`deploy.ps1` + `--set-secrets` wires the default runtime SA automatically in most
projects; grant explicitly if you use a custom runtime SA.)

---

## After W3: let the agent reach this service

The agent on Agent Engine must (a) have `roles/run.invoker` on this service and
(b) attach a Google **identity token** as `Authorization: Bearer <id-token>` on
its MCP calls (Cloud Run IAM requirement). Grant invoker:

```
gcloud run services add-iam-policy-binding pulseops-elastic-mcp `
  --region us-central1 --project pulseops-agent `
  --member "serviceAccount:<agent-engine-SA-email>" `
  --role roles/run.invoker
```

> Cross-workstream risk (resolve in W2): confirm the ADK `MCPToolset`
> streamable-HTTP client can attach (and refresh) a Google OIDC ID token for the
> Cloud Run audience. If it can't, options are: front the service with an internal
> HTTPS load balancer, use `--ingress internal` + Direct VPC egress from Agent
> Engine, or fall back to the Serverless built-in MCP endpoint (below).

---

## Verify

```powershell
$U = gcloud run services describe pulseops-elastic-mcp --region us-central1 --project pulseops-agent --format='value(status.url)'
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" "$U/ping"
```
Expect HTTP 200 from `/ping`. Anonymous `curl "$U/ping"` (no token) MUST return 401/403 — that confirms it is private.

---

## Alternative architecture (may remove this whole service)

Your Elastic endpoint (`*.es.*.gcp.elastic.cloud`) looks like **Elastic
Serverless**, which ships the **Agent Builder MCP endpoint** natively. If so, the
agent can point `ELASTIC_MCP_URL` directly at the Elastic-hosted MCP endpoint
(authenticated with the read-only ES API key), and **no Cloud Run MCP service is
needed**. Trade-offs:

| | This Cloud Run service | Serverless built-in MCP |
|---|---|---|
| Extra infra | Yes (1 Cloud Run service + secret) | None |
| Auth to MCP | Google IAM (id token) | ES API key header |
| Deprecation | Uses the deprecated standalone image | Current/supported path |
| Network | private, in your GCP project | Elastic-hosted (public TLS) |

Decision needed before W3 — see the parent plan
([../../docs/AGENT_BUILDER_ELASTIC_MCP_PLAN.md](../../docs/AGENT_BUILDER_ELASTIC_MCP_PLAN.md)).

---

## What this repo can/can't do for you

- ✅ Provided: `deploy.ps1`, `service.yaml`, this README.
- ⛔ **You must run the deploy** — it needs your authenticated `gcloud`, the GCP
  project, billing, and the W0 read-only key. These cannot be executed from the
  codebase.
