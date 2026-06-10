# Hosting PulseOps for the hackathon (W6)

Public, GCP-native deployment so judges can open one URL and see the full loop.
Chosen stack: **Cloud Run** (backend) + **Firebase Hosting** (dashboard, the
submission URL) + **Compute Engine Windows VM** (the monitoring agent). Already
live from earlier workstreams: **Vertex AI Agent Engine** (the agent) and
**Elastic Serverless** + its **Agent Builder MCP** endpoint.

```
Judge's browser
   │  https://pulseops-agent.web.app                (Firebase Hosting — frontend)
   ▼
Dashboard ──HTTPS/WSS──► Cloud Run (Go backend) ──► Vertex AI Agent Engine (Gemini + Elastic MCP)
                              │  indexes telemetry          │ streamable-HTTP + ApiKey
                              ▼                              ▼
                          Elastic Serverless  ◄────────  Elastic Agent Builder MCP
   ▲
Windows VM (Compute Engine) runs the Go agent ──telemetry/POST──► Cloud Run backend
   (monitors a real service, e.g. Spooler; executes Restart-Service on approval)
```

> Rules note: everything stays on Google Cloud (Cloud Run, Firebase, Compute
> Engine) — avoid AWS/Azure/Vercel/Netlify, which the rules treat as competing
> cloud platforms.

## Prerequisites (once)
- `gcloud auth login` + `gcloud config set project pulseops-agent`, billing enabled.
- `npm i -g firebase-tools` + `firebase login`.
- Enable Firebase on the project (once): `firebase projects:addfirebase pulseops-agent`.
- **Public GitHub repo** (rules require it) — note its zip URL for the VM step:
  `https://github.com/<owner>/pulseops/archive/refs/heads/main.zip`.
- Keys ready: the **WRITE** Elastic key in a file (backend indexer); the read-only
  Agent Builder key is already baked into the deployed Agent Engine (W3).
- Pick one **DeviceId** for the VM agent and use it everywhere below: `CLOUD-VM-01`.

## Step 1 — Backend → Cloud Run
```powershell
pwsh -File .\scripts\deploy-backend-cloudrun.ps1 `
    -Project pulseops-agent `
    -EsApiKeyFile .\es-write-key.txt `
    -EsEndpoint "https://my-elasticsearch-project-aa48e8.es.asia-south1.gcp.elastic.cloud:443" `
    -AgentEngineResource "projects/pulseops-agent/locations/us-central1/reasoningEngines/7212240475082719232" `
    -FrontendOrigin "https://pulseops-agent.web.app"
```
Note the printed **Backend URL** (e.g. `https://pulseops-backend-xxxxx-uc.a.run.app`).
- `CORS_ALLOWED_ORIGIN` is set to the Firebase URL so the dashboard's fetches **and**
  WebSocket pass (the WS upgrader checks Origin against this value).
- `GOOGLE_CLOUD_LOCATION=us-central1` matches the Agent Engine region (the query host
  is built from it) — not `global`.

## Step 2 — Frontend → Firebase Hosting
```powershell
pwsh -File .\scripts\deploy-frontend-firebase.ps1 `
    -Project pulseops-agent `
    -BackendUrl "https://pulseops-backend-xxxxx-uc.a.run.app" `
    -DeviceId CLOUD-VM-01
```
Outputs **https://pulseops-agent.web.app** — the hackathon **Hosted Project URL**.
`VITE_API_BASE_URL` is baked in; the WSS URL is derived from it automatically.
(Use the `.web.app` URL — it matches the backend's CORS origin. If you prefer
`.firebaseapp.com`, set `-FrontendOrigin` to that in Step 1 instead.)

## Step 3 — Agent → Windows VM
```powershell
pwsh -File .\scripts\create-agent-vm.ps1 -Project pulseops-agent
# set a password, get the IP (commands printed by the script), then RDP in:
gcloud compute reset-windows-password pulseops-agent-vm --zone us-central1-a --project pulseops-agent --user pulseops
```
RDP to the VM, open an **Administrator** PowerShell, copy `deploy/agent-vm/setup-agent.ps1`
to it (or paste its contents), and run:
```powershell
pwsh -File .\setup-agent.ps1 `
    -BackendUrl "https://pulseops-backend-xxxxx-uc.a.run.app" `
    -RepoZipUrl "https://github.com/<owner>/pulseops/archive/refs/heads/main.zip" `
    -DeviceId CLOUD-VM-01 -MonitoredService Spooler
```
Leave it running (admin session) so the `Restart-Service` remediation can execute.

## Step 4 — Verify end-to-end (what judges will see)
1. Open **https://pulseops-agent.web.app** → device `CLOUD-VM-01` shows live, healthy.
2. On the VM (admin shell): `Stop-Service Spooler -Force`.
3. Dashboard: incident `detected` → `Diagnosing… (Gemini + Elastic MCP)` → cause +
   `restart_service Spooler` → **Approve** → `executing` → `validating` → `resolved`.
4. The Incidents list shows history; **Clear** removes old ones.

## Costs & teardown
- Cloud Run + Firebase scale to ~zero; the **Windows VM is the main cost** (~e2-medium).
  Stop it when not demoing: `gcloud compute instances stop pulseops-agent-vm --zone us-central1-a`.
- Tear down after judging: `gcloud compute instances delete pulseops-agent-vm --zone us-central1-a`.

## Submission mapping (Stage-One pass/fail)
- **Hosted Project URL** → `https://pulseops-agent.web.app` (Step 2).
- **Public repo + OSI license** → W0 (LICENSE added) — push public, confirm About badge.
- **Demo video ≤ 3 min** → record Step 4 on the hosted URL.
- **Gemini + Agent Builder + Elastic MCP** → all live (W2/W3 + this).

## If the VM is too much / costs are a concern
Fallback to the laptop agent: skip Step 3, run the agent locally pointing
`BACKEND_BASE_URL` at the Cloud Run URL, and show the loop in the video. The hosted
dashboard + backend stay public; only the monitored endpoint is your machine.
