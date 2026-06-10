<#
W6 - Deploy the PulseOps backend to Cloud Run.

Builds backend/ with Cloud Build (uses backend/Dockerfile), deploys public
(the browser dashboard calls it), wires Elastic + Agent Engine, and grants the
runtime service account the roles it needs (Vertex query + Secret Manager).

Prerequisites:
  - gcloud auth login ; gcloud config set project <project>
  - The WRITE Elastic API key (W0/indexer) saved to a file, e.g. .\es-write-key.txt
  - Agent Engine deployed (W3); resource name handy.

Usage:
  pwsh -NoProfile -File .\scripts\deploy-backend-cloudrun.ps1 `
      -Project pulseops-agent `
      -EsApiKeyFile .\es-write-key.txt `
      -EsEndpoint "https://my-elasticsearch-project-aa48e8.es.asia-south1.gcp.elastic.cloud:443" `
      -AgentEngineResource "projects/pulseops-agent/locations/us-central1/reasoningEngines/7212240475082719232" `
      -FrontendOrigin "https://pulseops-agent.web.app"

Outputs: the Cloud Run service URL -> use as VITE_API_BASE_URL for the frontend.
#>

param(
    [string]$Project = 'pulseops-agent',
    [string]$Region = 'us-central1',
    [string]$Service = 'pulseops-backend',
    [Parameter(Mandatory = $true)][string]$EsEndpoint,
    [Parameter(Mandatory = $true)][string]$EsApiKeyFile,
    [Parameter(Mandatory = $true)][string]$AgentEngineResource,
    # The hosted dashboard origin used for CORS. Default = Firebase default site URL.
    [string]$FrontendOrigin = 'https://pulseops-agent.web.app',
    # Vertex region for the Agent Engine query host — MUST match the engine's region.
    [string]$EngineLocation = 'us-central1',
    [string]$SecretName = 'elastic-backend-api-key'
)

$ErrorActionPreference = 'Stop'
# PowerShell 7.4+ makes native commands honor $ErrorActionPreference='Stop', which turns
# gcloud's EXPECTED non-zero exits (e.g. `secrets describe` on a not-yet-created secret)
# into terminating errors and aborts the script. This script checks $LASTEXITCODE itself,
# so opt out. (Harmless no-op on Windows PowerShell 5.1, where the variable is unused.)
$PSNativeCommandUseErrorActionPreference = $false

function Invoke-Gcloud {
    param([Parameter(Mandatory = $true)][string[]]$GcloudArgs)
    Write-Host "> gcloud $($GcloudArgs -join ' ')" -ForegroundColor Cyan
    & gcloud @GcloudArgs
    if ($LASTEXITCODE -ne 0) { throw "gcloud failed (exit $LASTEXITCODE): $($GcloudArgs -join ' ')" }
}

if (-not (Test-Path $EsApiKeyFile)) { throw "EsApiKeyFile not found: $EsApiKeyFile" }
$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'

Write-Host "== W6: deploy backend to Cloud Run ==" -ForegroundColor Green

# 1) Enable APIs.
Invoke-Gcloud @('services', 'enable', 'run.googleapis.com', 'cloudbuild.googleapis.com',
    'artifactregistry.googleapis.com', 'secretmanager.googleapis.com',
    'aiplatform.googleapis.com', '--project', $Project)

# 2) Secret for the Elastic WRITE key. Probe with `secrets list` (always exits 0) instead of
# `secrets describe` (returns NOT_FOUND on a missing secret, which gcloud's PowerShell wrapper
# raises as a terminating NativeCommandError before this script can handle the exit code).
$existingSecrets = @(& gcloud secrets list --project $Project --format='value(name)' 2>$null)
$secretExists = [bool]($existingSecrets | Where-Object { $_ -like "*$SecretName" })
if (-not $secretExists) {
    Invoke-Gcloud @('secrets', 'create', $SecretName, '--replication-policy=automatic', '--project', $Project)
}
Invoke-Gcloud @('secrets', 'versions', 'add', $SecretName, "--data-file=$EsApiKeyFile", '--project', $Project)

# 3) Grant the default Cloud Run runtime SA the roles it needs.
$projectNumber = (& gcloud projects describe $Project --format='value(projectNumber)').Trim()
$runtimeSA = "$projectNumber-compute@developer.gserviceaccount.com"
Invoke-Gcloud @('projects', 'add-iam-policy-binding', $Project,
    '--member', "serviceAccount:$runtimeSA", '--role', 'roles/aiplatform.user', '--condition', 'None')
Invoke-Gcloud @('secrets', 'add-iam-policy-binding', $SecretName, '--project', $Project,
    '--member', "serviceAccount:$runtimeSA", '--role', 'roles/secretmanager.secretAccessor')

# 4) Deploy from source (Cloud Build uses backend/Dockerfile).
$envVars = @(
    "APP_ENV=production",
    "ELASTIC_ENDPOINT=$EsEndpoint",
    "ELASTIC_INDEX_TELEMETRY=telemetry-events",
    "ELASTIC_INDEX_INCIDENTS=incident-events",
    "ELASTIC_INDEX_LOGS=endpoint-logs",
    "ELASTIC_ENABLED=true",
    "AGENT_BUILDER_ENABLED=true",
    "AGENT_BUILDER_TRANSPORT=agent_engine",
    "AGENT_ENGINE_RESOURCE=$AgentEngineResource",
    "GOOGLE_CLOUD_PROJECT=$Project",
    "GOOGLE_CLOUD_LOCATION=$EngineLocation",
    # Judge-facing "Simulate Service Failure" panel — additive, gated by DEMO_MODE.
    "DEMO_MODE=true",
    "CORS_ALLOWED_ORIGIN=$FrontendOrigin",
    "FRONTEND_BASE_URL=$FrontendOrigin"
) -join ','

Invoke-Gcloud @(
    'run', 'deploy', $Service,
    '--source', $backendDir,
    '--region', $Region, '--project', $Project,
    '--service-account', $runtimeSA,
    '--allow-unauthenticated',
    '--set-env-vars', $envVars,
    '--set-secrets', "ELASTIC_API_KEY=${SecretName}:latest",
    '--cpu', '1', '--memory', '512Mi',
    '--min-instances', '0', '--max-instances', '3',
    '--timeout', '120'
)

$url = (& gcloud run services describe $Service --region $Region --project $Project --format 'value(status.url)').Trim()
Write-Host ""
Write-Host "== Backend deployed ==" -ForegroundColor Green
Write-Host "Backend URL: $url" -ForegroundColor Yellow
Write-Host "Use it as VITE_API_BASE_URL for the frontend:" -ForegroundColor Yellow
Write-Host "  pwsh -File .\scripts\deploy-frontend-firebase.ps1 -Project $Project -BackendUrl $url" -ForegroundColor Gray
Write-Host "NOTE: ensure -FrontendOrigin matches your final Firebase URL (CORS). Re-run with the right origin if needed." -ForegroundColor Gray
