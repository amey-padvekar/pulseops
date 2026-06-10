<#
W6 - Build and deploy the PulseOps dashboard to Firebase Hosting.

Bakes the Cloud Run backend URL into the build (Vite reads VITE_* at build time),
builds the SPA, and deploys to Firebase Hosting. The resulting URL is the
hackathon "Hosted Project URL" judges open.

Prerequisites:
  - npm i -g firebase-tools ; firebase login
  - Firebase enabled on the GCP project (firebase projects:addfirebase <project>, once)
  - Backend already deployed (W6 backend step) so you have its URL.

Usage:
  pwsh -NoProfile -File .\scripts\deploy-frontend-firebase.ps1 `
      -Project pulseops-agent `
      -BackendUrl "https://pulseops-backend-xxxxx-uc.a.run.app" `
      -DeviceId CLOUD-VM-01

Outputs: the Firebase Hosting URL (https://<project>.web.app).
#>

param(
    [string]$Project = 'pulseops-agent',
    [Parameter(Mandatory = $true)][string]$BackendUrl,
    # The deviceId the dashboard defaults to (match the agent's AGENT_DEVICE_ID).
    [string]$DeviceId = 'DEV-AGENT-01'
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$frontendDir = Join-Path $repoRoot 'frontend'

# WS URL is derived from VITE_API_BASE_URL in code (http->ws, https->wss), so only
# the API base is needed. Write a production env the Vite build will pick up.
$envProd = @(
    "VITE_APP_ENV=production",
    "VITE_API_BASE_URL=$($BackendUrl.TrimEnd('/'))",
    "VITE_AGENT_DEVICE_ID=$DeviceId",
    # Judge-facing "Simulate Service Failure" panel (must match backend DEMO_MODE=true).
    "VITE_DEMO_MODE=true"
) -join "`n"
$envPath = Join-Path $frontendDir '.env.production'
Set-Content -Path $envPath -Value $envProd -Encoding utf8
Write-Host "Wrote ${envPath}:" -ForegroundColor Cyan
Write-Host $envProd -ForegroundColor Gray

Push-Location $frontendDir
try {
    Write-Host "Installing deps + building..." -ForegroundColor Cyan
    & npm ci
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed" }
    & npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed" }

    Write-Host "Deploying to Firebase Hosting..." -ForegroundColor Cyan
    & firebase deploy --only hosting --project $Project
    if ($LASTEXITCODE -ne 0) { throw "firebase deploy failed" }
}
finally {
    Pop-Location
}

Write-Host ""
Write-Host "== Frontend deployed ==" -ForegroundColor Green
Write-Host "Hosted URL: https://$Project.web.app  (also https://$Project.firebaseapp.com)" -ForegroundColor Yellow
Write-Host "Ensure the backend's CORS_ALLOWED_ORIGIN matches this URL (re-run the backend deploy if needed)." -ForegroundColor Gray
