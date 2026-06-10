<#
Workstream 1 — Deploy the official Elastic MCP server to Cloud Run.

Runs the published image `docker.elastic.co/mcp/elasticsearch` in streamable-HTTP
mode, private (IAM-gated), with the read-only ES API key sourced from Secret
Manager. There is NO application code here — only deploy glue.

IMPORTANT SECURITY NOTE:
  The Elastic MCP server's HTTP listener has no authentication of its own. The
  ES credential is baked into the container via env var, so any caller that can
  reach the URL can query Elasticsearch. Therefore this service MUST stay private
  (--no-allow-unauthenticated). Cloud Run IAM is the only gate. Never expose it
  publicly.

DEPRECATION NOTE:
  This image is deprecated (critical-security-updates only). It is superseded by
  the Elastic Agent Builder MCP endpoint built into Elastic 9.2.0+ / Serverless.
  If your Elastic deployment exposes that endpoint, you may not need this service
  at all — point the agent (W2) directly at the Elastic-hosted MCP endpoint.

Prerequisites:
  - gcloud CLI authenticated: `gcloud auth login` and `gcloud config set project <id>`
  - APIs enabled (the script enables them): run, secretmanager
  - The read-only ES API key from W0 (encoded form), in a file you pass via -EsApiKeyFile

Usage (typical):
  pwsh -NoProfile -File .\deploy\elastic-mcp\deploy.ps1 `
      -Project pulseops-agent `
      -Region us-central1 `
      -EsUrl "https://your-deployment.es.asia-south1.gcp.elastic.cloud:443" `
      -EsApiKeyFile .\es-readonly-key.txt

  # After W3 mints the Agent Engine service account, grant it invoker:
  pwsh -NoProfile -File .\deploy\elastic-mcp\deploy.ps1 -EsUrl "..." `
      -AgentEngineServiceAccount "service-PROJNUM@gcp-sa-aiplatform-re.iam.gserviceaccount.com"

Outputs:
  - The Cloud Run service URL and the value to use as ELASTIC_MCP_URL ( = <url>/mcp )
#>

param(
    [string]$Project = 'pulseops-agent',
    [string]$Region = 'us-central1',
    [string]$Service = 'pulseops-elastic-mcp',
    [Parameter(Mandatory = $true)][string]$EsUrl,
    [string]$SecretName = 'elastic-mcp-es-api-key',
    [string]$EsApiKeyFile,
    # Pin to a specific version/digest for a reproducible submission (recommended).
    [string]$Image = 'docker.elastic.co/mcp/elasticsearch:latest',
    [string]$AgentEngineServiceAccount,
    [ValidateSet('all', 'internal', 'internal-and-cloud-load-balancing')]
    [string]$Ingress = 'all',
    [switch]$EsSslSkipVerify
)

$ErrorActionPreference = 'Stop'

function Invoke-Gcloud {
    param([Parameter(Mandatory = $true)][string[]]$GcloudArgs)
    Write-Host "» gcloud $($GcloudArgs -join ' ')" -ForegroundColor Cyan
    & gcloud @GcloudArgs
    if ($LASTEXITCODE -ne 0) { throw "gcloud failed (exit $LASTEXITCODE): $($GcloudArgs -join ' ')" }
}

Write-Host "== Workstream 1: deploy Elastic MCP server to Cloud Run ==" -ForegroundColor Green
Write-Host "project=$Project region=$Region service=$Service ingress=$Ingress" -ForegroundColor Gray

# 1) Enable required APIs (idempotent).
Invoke-Gcloud @('services', 'enable', 'run.googleapis.com', 'secretmanager.googleapis.com', '--project', $Project)

# 2) Ensure the Secret Manager secret exists and has a version.
$secretExists = $true
& gcloud secrets describe $SecretName --project $Project 2>$null | Out-Null
if ($LASTEXITCODE -ne 0) { $secretExists = $false }

if (-not $secretExists) {
    Invoke-Gcloud @('secrets', 'create', $SecretName, '--replication-policy=automatic', '--project', $Project)
}

if ($EsApiKeyFile) {
    if (-not (Test-Path $EsApiKeyFile)) { throw "EsApiKeyFile not found: $EsApiKeyFile" }
    Invoke-Gcloud @('secrets', 'versions', 'add', $SecretName, "--data-file=$EsApiKeyFile", '--project', $Project)
}
else {
    # Confirm at least one version exists; otherwise the deploy would fail at runtime.
    & gcloud secrets versions list $SecretName --project $Project --limit 1 --format 'value(name)' 2>$null | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "Secret '$SecretName' has no versions. Re-run with -EsApiKeyFile <path-to-readonly-key>, or run: gcloud secrets versions add $SecretName --data-file=<file> --project $Project"
    }
}

# 3) Build env-var list.
$envVars = "ES_URL=$EsUrl"
if ($EsSslSkipVerify) { $envVars += ",ES_SSL_SKIP_VERIFY=true" }

# 4) Deploy the published image in `http` mode, private.
$deployArgs = @(
    'run', 'deploy', $Service,
    '--image', $Image,
    '--region', $Region,
    '--project', $Project,
    '--port', '8080',
    '--args', 'http',
    '--no-allow-unauthenticated',
    '--ingress', $Ingress,
    '--set-env-vars', $envVars,
    '--set-secrets', "ES_API_KEY=${SecretName}:latest",
    '--cpu', '1',
    '--memory', '512Mi',
    '--min-instances', '0',
    '--max-instances', '2'
)
Invoke-Gcloud $deployArgs

# 5) Optionally grant the Agent Engine service account invoke permission (from W3).
if ($AgentEngineServiceAccount) {
    Invoke-Gcloud @('run', 'services', 'add-iam-policy-binding', $Service,
        '--region', $Region, '--project', $Project,
        '--member', "serviceAccount:$AgentEngineServiceAccount",
        '--role', 'roles/run.invoker')
    Write-Host "Granted roles/run.invoker to $AgentEngineServiceAccount" -ForegroundColor Green
}
else {
    Write-Host "NOTE: no -AgentEngineServiceAccount given. After W3, grant it roles/run.invoker on this service." -ForegroundColor Yellow
}

# 6) Resolve the URL and print the agent-facing value.
$url = (& gcloud run services describe $Service --region $Region --project $Project --format 'value(status.url)').Trim()
if ([string]::IsNullOrWhiteSpace($url)) { throw "Could not resolve service URL" }
$mcpUrl = "$url/mcp"

Write-Host ""
Write-Host "== Deployed ==" -ForegroundColor Green
Write-Host "Service URL : $url"
Write-Host "ELASTIC_MCP_URL (set this for the agent in W2/W3): $mcpUrl" -ForegroundColor Cyan

# 7) Verify the private /ping endpoint using a Google identity token (caller needs run.invoker).
try {
    $idToken = (& gcloud auth print-identity-token).Trim()
    $resp = Invoke-WebRequest -Uri "$url/ping" -Headers @{ Authorization = "Bearer $idToken" } -UseBasicParsing -TimeoutSec 20
    Write-Host "Health check /ping -> HTTP $($resp.StatusCode)" -ForegroundColor Green
}
catch {
    Write-Host "Health check could not be confirmed automatically: $($_.Exception.Message)" -ForegroundColor Yellow
    Write-Host "Manual check: curl -H `"Authorization: Bearer `$(gcloud auth print-identity-token)`" $url/ping" -ForegroundColor Gray
}
