<#
Workstream 3 - Deploy the PulseOps ADK agent to Vertex AI Agent Engine.

Wraps `adk deploy agent_engine` (ADK 2.1.0). The agent's env vars
(ELASTIC_MCP_URL, ELASTIC_MCP_API_KEY, GOOGLE_*, GEMINI_MODEL) are read by the
deployer from google-agent-service/agent/.env and baked into the deployment, and
agent/requirements.txt is staged automatically. On success the deployer prints
the reasoningEngine resource name -> put it in the backend as
AGENT_ENGINE_RESOURCE (W4).

Prerequisites (user actions):
  - gcloud auth login
  - gcloud auth application-default login         (deploy uses ADC)
  - billing enabled on the project (already done)
  - google-agent-service/agent/.env set for DEPLOY (Vertex mode) - see google-agent-service/DEPLOY.md
    (GOOGLE_GENAI_USE_VERTEXAI=true, NO GOOGLE_API_KEY, GEMINI_MODEL=gemini-2.5-flash)

Usage:
  pwsh -NoProfile -File .\scripts\deploy-agent-engine.ps1 -Project pulseops-agent -Region us-central1
  # update an existing engine instead of creating a new one:
  pwsh -NoProfile -File .\scripts\deploy-agent-engine.ps1 -AgentEngineId 1234567890123456789
#>

param(
    [string]$Project = 'pulseops-agent',
    [string]$Region = 'us-central1',
    [string]$DisplayName = 'pulseops-investigator',
    [string]$Description = 'PulseOps incident investigator (Gemini + Elastic MCP)',
    [string]$AgentEngineId
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$svcDir = Join-Path $repoRoot 'google-agent-service'
$agentDir = Join-Path $svcDir 'agent'
$envFile = Join-Path $agentDir '.env'

if (-not (Test-Path $envFile)) {
    throw "Missing $envFile. Create it from agent/.env.example with Vertex DEPLOY settings (see google-agent-service/DEPLOY.md)."
}

# Guard against deploying the AI Studio key path: Agent Engine should use Vertex.
$envText = Get-Content $envFile -Raw
if ($envText -match '(?im)^\s*GOOGLE_GENAI_USE_VERTEXAI\s*=\s*false') {
    throw "agent/.env has GOOGLE_GENAI_USE_VERTEXAI=false. For Agent Engine set it to true and remove GOOGLE_API_KEY (see DEPLOY.md)."
}

# Resolve the adk CLI from the project venv if available, else PATH.
$adk = Join-Path $svcDir '.venv\Scripts\adk.exe'
if (-not (Test-Path $adk)) { $adk = 'adk' }

Write-Host "== W3: deploy agent to Vertex AI Agent Engine ==" -ForegroundColor Green
Write-Host "project=$Project region=$Region display_name=$DisplayName" -ForegroundColor Gray

Write-Host "Enabling required APIs (Vertex AI + Storage for staging)..." -ForegroundColor Cyan
& gcloud services enable aiplatform.googleapis.com storage.googleapis.com --project $Project
if ($LASTEXITCODE -ne 0) { throw "failed to enable required APIs" }

# AGENT positional must be the agent folder name, resolved from the service dir.
Push-Location $svcDir
try {
    $deployArgs = @(
        'deploy', 'agent_engine',
        '--project', $Project,
        '--region', $Region,
        '--display_name', $DisplayName,
        '--description', $Description
    )
    if ($AgentEngineId) { $deployArgs += @('--agent_engine_id', $AgentEngineId) }
    $deployArgs += 'agent'

    Write-Host "> $adk $($deployArgs -join ' ')" -ForegroundColor Cyan
    & $adk @deployArgs
    if ($LASTEXITCODE -ne 0) { throw "adk deploy agent_engine failed (exit $LASTEXITCODE)" }
}
finally {
    Pop-Location
}

Write-Host ""
Write-Host "== Deploy finished ==" -ForegroundColor Green
Write-Host "Copy the 'reasoningEngines/...' resource name printed above into the backend (W4):" -ForegroundColor Yellow
Write-Host "  AGENT_ENGINE_RESOURCE=projects/$Project/locations/$Region/reasoningEngines/<ID>" -ForegroundColor Yellow
