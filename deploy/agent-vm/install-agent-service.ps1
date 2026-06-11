<#
Build the PulseOps monitoring agent ON a Windows VM and install it as a real
Windows SERVICE (via NSSM). Unlike setup-agent.ps1 (which runs the agent in the
foreground and dies on RDP logoff), this:
  - runs as LocalSystem (admin) so Restart-Service remediation works,
  - survives logoff and auto-starts on reboot,
  - captures stdout/stderr to a log file you can tail.

RUN THIS ON THE VM, in an ELEVATED (Administrator) PowerShell, FROM A FOLDER THAT
IS *NOT* -WorkDir. Put the script in e.g. C:\agentsetup and run it from there;
the script wipes/rebuilds -WorkDir (default C:\pulseops), so it must not live
inside it (that was the "Cannot remove ... because it is in use" error).

Get the script onto the VM (after pushing it to GitHub):
  New-Item -ItemType Directory -Force C:\agentsetup | Out-Null
  Set-Location C:\agentsetup
  iwr https://raw.githubusercontent.com/<owner>/pulseops/main/deploy/agent-vm/install-agent-service.ps1 -OutFile install-agent-service.ps1

Usage (on the VM):
  powershell -File .\install-agent-service.ps1 `
      -BackendUrl "https://pulseops-backend-xxxxx-uc.a.run.app" `
      -RepoZipUrl "https://github.com/<owner>/pulseops/archive/refs/heads/main.zip" `
      -DeviceId CLOUD-VM-01 -MonitoredService Spooler

Re-running updates the agent: it stops + removes the old service (freeing the exe
lock), rebuilds from the latest zip, and reinstalls. Idempotent.

Manage later:
  Get-Service PulseOpsAgent
  Get-Content C:\pulseops\agent\agent.log -Wait        # watch live
  Stop-Service Spooler -Force                           # trigger a demo incident
Uninstall:
  & "$env:TEMP\nssm-2.24\nssm-2.24\win64\nssm.exe" stop PulseOpsAgent
  & "$env:TEMP\nssm-2.24\nssm-2.24\win64\nssm.exe" remove PulseOpsAgent confirm

The DeviceId must match the device you select in the dashboard dropdown.
#>

param(
    [Parameter(Mandatory = $true)][string]$BackendUrl,
    [Parameter(Mandatory = $true)][string]$RepoZipUrl,
    [string]$MonitoredService = 'Spooler',
    [string]$DeviceId = 'CLOUD-VM-01',
    [string]$WorkDir = 'C:\pulseops',
    [string]$ServiceName = 'PulseOpsAgent',
    [string]$GoVersion = '1.23.4',
    [string]$NssmVersion = '2.24'
)

$ErrorActionPreference = 'Stop'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$workRoot = $WorkDir.TrimEnd('\')
$agentDir = Join-Path $workRoot 'agent'
$exe = Join-Path $agentDir 'pulseops-agent.exe'
$backend = $BackendUrl.TrimEnd('/')

# --- 0) Guards -------------------------------------------------------------
$isAdmin = ([Security.Principal.WindowsPrincipal]`
        [Security.Principal.WindowsIdentity]::GetCurrent()
).IsInRole([Security.Principal.WindowsBuiltinRole]::Administrator)
if (-not $isAdmin) {
    throw "Run this in an ELEVATED (Administrator) PowerShell - Restart-Service needs admin."
}

# Never let the script delete the folder it is running from / living in.
$scriptDir = $PSScriptRoot
if ($scriptDir) {
    $sd = $scriptDir.TrimEnd('\')
    if ($sd -ieq $workRoot -or $sd -like "$workRoot\*") {
        throw "This script is inside -WorkDir ($workRoot), which it must wipe. Move it to e.g. C:\agentsetup and run it from there."
    }
}
if ((Get-Location).Path.TrimEnd('\') -like "$workRoot*") {
    throw "Your current directory is inside -WorkDir ($workRoot). cd to e.g. C:\agentsetup first."
}

Write-Host "== Install PulseOps agent as a Windows service ==" -ForegroundColor Green
Write-Host "service=$ServiceName device=$DeviceId monitored=$MonitoredService -> $backend" -ForegroundColor Yellow

# --- 1) NSSM (service wrapper) --------------------------------------------
$nssmRoot = Join-Path $env:TEMP "nssm-$NssmVersion"
$nssm = Join-Path $nssmRoot "nssm-$NssmVersion\win64\nssm.exe"
if (-not (Test-Path $nssm)) {
    Write-Host "Downloading NSSM $NssmVersion..." -ForegroundColor Cyan
    $nssmZip = Join-Path $env:TEMP "nssm-$NssmVersion.zip"
    Invoke-WebRequest "https://nssm.cc/release/nssm-$NssmVersion.zip" -OutFile $nssmZip
    if (Test-Path $nssmRoot) { Remove-Item $nssmRoot -Recurse -Force }
    Expand-Archive $nssmZip -DestinationPath $nssmRoot -Force
}
if (-not (Test-Path $nssm)) { throw "NSSM not found at $nssm after download" }

# --- 2) Remove any existing service FIRST (it holds the .exe open) ---------
if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Stopping + removing existing service $ServiceName..." -ForegroundColor Cyan
    & $nssm stop $ServiceName
    & $nssm remove $ServiceName confirm
    Start-Sleep -Seconds 2
}

# --- 3) Install Go if missing ---------------------------------------------
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Go $GoVersion..." -ForegroundColor Cyan
    $goMsi = Join-Path $env:TEMP "go$GoVersion.msi"
    Invoke-WebRequest "https://go.dev/dl/go$GoVersion.windows-amd64.msi" -OutFile $goMsi
    Start-Process msiexec.exe -ArgumentList "/i `"$goMsi`" /quiet /norestart" -Wait
    $env:Path = "C:\Program Files\Go\bin;$env:Path"
}
& go version
if ($LASTEXITCODE -ne 0) { throw "Go is not available on PATH after install" }

# --- 4) Download + extract the repo to WorkDir (safe now: service is gone) -
Write-Host "Downloading repo zip..." -ForegroundColor Cyan
if (Test-Path $workRoot) { Remove-Item $workRoot -Recurse -Force }
$zip = Join-Path $env:TEMP 'pulseops.zip'
Invoke-WebRequest $RepoZipUrl -OutFile $zip
$extract = Join-Path $env:TEMP 'pulseops-extract'
if (Test-Path $extract) { Remove-Item $extract -Recurse -Force }
Expand-Archive $zip -DestinationPath $extract -Force
$inner = Get-ChildItem $extract -Directory | Select-Object -First 1
if (-not $inner) { throw "repo zip did not contain a top-level folder" }
Move-Item $inner.FullName $workRoot

# --- 5) Write agent/.env (backup; NSSM env below is authoritative) --------
$agentEnv = @(
    "APP_ENV=production",
    "AGENT_DEVICE_ID=$DeviceId",
    "MONITORED_SERVICE_NAME=$MonitoredService",
    "BACKEND_BASE_URL=$backend",
    "AGENT_HEARTBEAT_INTERVAL_SEC=10"
) -join "`n"
Set-Content -Path (Join-Path $agentDir '.env') -Value $agentEnv -Encoding ascii

# --- 6) Build the agent ----------------------------------------------------
Write-Host "Building agent..." -ForegroundColor Cyan
Push-Location $agentDir
try {
    & go build -o $exe ./cmd/agent
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }
}
finally {
    Pop-Location
}
if (-not (Test-Path $exe)) { throw "build did not produce $exe" }

# --- 7) Install + configure the service -----------------------------------
Write-Host "Installing service $ServiceName..." -ForegroundColor Cyan
$logFile = Join-Path $agentDir 'agent.log'
& $nssm install $ServiceName $exe
& $nssm set $ServiceName AppDirectory $agentDir
& $nssm set $ServiceName DisplayName "PulseOps Monitoring Agent"
& $nssm set $ServiceName Description "Reports $MonitoredService health to PulseOps and runs operator-approved remediations."
& $nssm set $ServiceName Start SERVICE_AUTO_START
& $nssm set $ServiceName AppStdout $logFile
& $nssm set $ServiceName AppStderr $logFile
& $nssm set $ServiceName AppRotateFiles 1
& $nssm set $ServiceName AppRotateBytes 1048576
# Env vars take precedence over .env and are independent of the working directory.
& $nssm set $ServiceName AppEnvironmentExtra `
    "APP_ENV=production" `
    "AGENT_DEVICE_ID=$DeviceId" `
    "MONITORED_SERVICE_NAME=$MonitoredService" `
    "BACKEND_BASE_URL=$backend" `
    "AGENT_HEARTBEAT_INTERVAL_SEC=10"

& $nssm start $ServiceName
Start-Sleep -Seconds 2

# --- 8) Report -------------------------------------------------------------
$svc = Get-Service $ServiceName -ErrorAction SilentlyContinue
Write-Host ""
Write-Host "== Done ==" -ForegroundColor Green
Write-Host "Service : $ServiceName  ($([string]$svc.Status))" -ForegroundColor Yellow
Write-Host "Exe     : $exe" -ForegroundColor Gray
Write-Host "Log     : $logFile" -ForegroundColor Gray
Write-Host ""
Write-Host "Watch it:        Get-Content `"$logFile`" -Wait" -ForegroundColor Gray
Write-Host "Trigger a demo:  Stop-Service $MonitoredService -Force   (then Approve in the dashboard)" -ForegroundColor Gray
Write-Host "In the dashboard, select device '$DeviceId'." -ForegroundColor Gray
