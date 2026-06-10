<#
W6 - Set up and run the PulseOps monitoring agent ON a Windows VM.

RUN THIS ON THE VM, in an ELEVATED (Administrator) PowerShell — remediation
(Restart-Service) requires admin. It installs Go, downloads the repo (no git
needed), builds the agent, and runs it against a real Windows service.

Usage (on the VM):
  pwsh -NoProfile -File .\setup-agent.ps1 `
      -BackendUrl "https://pulseops-backend-xxxxx-uc.a.run.app" `
      -RepoZipUrl "https://github.com/<owner>/pulseops/archive/refs/heads/main.zip" `
      -DeviceId CLOUD-VM-01 -MonitoredService Spooler

The DeviceId must match the dashboard default (deploy-frontend-firebase.ps1 -DeviceId).
#>

param(
    [Parameter(Mandatory = $true)][string]$BackendUrl,
    [Parameter(Mandatory = $true)][string]$RepoZipUrl,
    [string]$MonitoredService = 'Spooler',
    [string]$DeviceId = 'CLOUD-VM-01',
    [string]$WorkDir = 'C:\pulseops',
    [string]$GoVersion = '1.23.4'
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# 1) Install Go if missing.
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Go $GoVersion..." -ForegroundColor Cyan
    $goMsi = Join-Path $env:TEMP "go$GoVersion.msi"
    Invoke-WebRequest "https://go.dev/dl/go$GoVersion.windows-amd64.msi" -OutFile $goMsi
    Start-Process msiexec.exe -ArgumentList "/i `"$goMsi`" /quiet /norestart" -Wait
    $env:Path = "C:\Program Files\Go\bin;$env:Path"
}
& go version
if ($LASTEXITCODE -ne 0) { throw "Go is not available on PATH after install" }

# 2) Download + extract the repo (zip; no git required).
Write-Host "Downloading repo zip..." -ForegroundColor Cyan
if (Test-Path $WorkDir) { Remove-Item $WorkDir -Recurse -Force }
$zip = Join-Path $env:TEMP 'pulseops.zip'
Invoke-WebRequest $RepoZipUrl -OutFile $zip
$extract = Join-Path $env:TEMP 'pulseops-extract'
if (Test-Path $extract) { Remove-Item $extract -Recurse -Force }
Expand-Archive $zip -DestinationPath $extract -Force
$inner = Get-ChildItem $extract -Directory | Select-Object -First 1
if (-not $inner) { throw "repo zip did not contain a top-level folder" }
Move-Item $inner.FullName $WorkDir

# 3) Write agent/.env (backup) — real env vars below take precedence anyway.
$agentDir = Join-Path $WorkDir 'agent'
$agentEnv = @(
    "APP_ENV=production",
    "AGENT_DEVICE_ID=$DeviceId",
    "MONITORED_SERVICE_NAME=$MonitoredService",
    "BACKEND_BASE_URL=$($BackendUrl.TrimEnd('/'))",
    "AGENT_HEARTBEAT_INTERVAL_SEC=10"
) -join "`n"
Set-Content -Path (Join-Path $agentDir '.env') -Value $agentEnv -Encoding utf8

# 4) Build the agent.
Write-Host "Building agent..." -ForegroundColor Cyan
$exe = Join-Path $agentDir 'pulseops-agent.exe'
Push-Location $agentDir
try {
    & go build -o $exe ./cmd/agent
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
}
finally {
    Pop-Location
}

# 5) Run it (foreground). Env vars are set in-session for reliability.
$env:APP_ENV = 'production'
$env:AGENT_DEVICE_ID = $DeviceId
$env:MONITORED_SERVICE_NAME = $MonitoredService
$env:BACKEND_BASE_URL = $BackendUrl.TrimEnd('/')

Write-Host ""
Write-Host "== Starting agent ==" -ForegroundColor Green
Write-Host "device=$DeviceId service=$MonitoredService -> $BackendUrl" -ForegroundColor Yellow
Write-Host "Leave this window open. To trigger an incident from another admin shell:" -ForegroundColor Gray
Write-Host "  Stop-Service $MonitoredService -Force" -ForegroundColor Gray
& $exe
