param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'

$healthzUrl = 'http://localhost:8080/healthz'
$devicesUrl = 'http://localhost:8080/devices'
$expectedDeviceId = if ($env:AGENT_DEVICE_ID) { $env:AGENT_DEVICE_ID } else { 'DEV-AGENT-01' }

$evidenceRootDir = Join-Path $repoRoot 'artifacts\phase3-smoke'
$runId = Get-Date -Format 'yyyyMMdd-HHmmss'
$evidenceDir = Join-Path $evidenceRootDir $runId
$backendLog = Join-Path $evidenceDir 'backend.log'
$backendErrLog = Join-Path $evidenceDir 'backend.err.log'
$agentLog = Join-Path $evidenceDir 'agent.log'
$agentErrLog = Join-Path $evidenceDir 'agent.err.log'

New-Item -ItemType Directory -Path $evidenceDir -Force | Out-Null

if (-not $SkipBuild) {
    Write-Host '[smoke-check] Building backend...'
    Push-Location $backendDir
    go build ./...
    Pop-Location

    Write-Host '[smoke-check] Building agent...'
    Push-Location $agentDir
    go build ./...
    Pop-Location

    Write-Host '[smoke-check] Building frontend...'
    Push-Location $frontendDir
    npm run build | Out-Null
    Pop-Location
}

Write-Host '[smoke-check] Starting backend and agent for Phase 3 verification...'
$backendProc = Start-Process go -WorkingDirectory $backendDir -ArgumentList 'run', './cmd/server' -RedirectStandardOutput $backendLog -RedirectStandardError $backendErrLog -PassThru
$agentProc = Start-Process go -WorkingDirectory $agentDir -ArgumentList 'run', './cmd/agent' -RedirectStandardOutput $agentLog -RedirectStandardError $agentErrLog -PassThru

try {
    $healthReady = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $resp = Invoke-RestMethod -Uri $healthzUrl -Method Get -TimeoutSec 2
            if ($resp.status -eq 'ok') {
                $healthReady = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }
    if (-not $healthReady) {
        throw 'Backend /healthz did not return status=ok within timeout.'
    }

    $devicesList = $null
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $candidate = Invoke-RestMethod -Uri $devicesUrl -Method Get -TimeoutSec 2
            if ($candidate -is [System.Array] -and $candidate.Count -gt 0) {
                $devicesList = $candidate
                break
            }
        } catch {
            # ignore transient startup failures
        }
        Start-Sleep -Milliseconds 500
    }
    if (-not $devicesList) {
        throw "No device states were returned from $devicesUrl. Check $backendLog and $agentLog"
    }

    $deviceState = $null
    try {
        $deviceState = Invoke-RestMethod -Uri ("$devicesUrl/$expectedDeviceId") -Method Get -TimeoutSec 2
    } catch {
        # Fallback to first observed device if expected id is different.
        $expectedDeviceId = $devicesList[0].deviceId
        $deviceState = Invoke-RestMethod -Uri ("$devicesUrl/$expectedDeviceId") -Method Get -TimeoutSec 2
    }

    if (-not $deviceState.deviceId) {
        throw 'Device state response is missing deviceId.'
    }
    if (-not $deviceState.serviceStatus) {
        throw 'Device state response is missing serviceStatus.'
    }
    if (-not $deviceState.timestamp) {
        throw 'Device state response is missing timestamp.'
    }

    Write-Host '[smoke-check] PASS: backend health, telemetry ingestion, and /devices endpoints are working.'
    Write-Host "[smoke-check] Verified device: $expectedDeviceId"
    Write-Host "[smoke-check] Current serviceStatus: $($deviceState.serviceStatus)"
    Write-Host "[smoke-check] Evidence logs written to: $evidenceDir"

    Write-Host ''
    Write-Host '[smoke-check] Manual UI follow-up for Phase 3:'
    Write-Host '  1) Start frontend with scripts/run-frontend.ps1 and open http://localhost:5173'
    Write-Host '  2) Confirm Endpoint Health shows connected + live CPU/memory values'
    Write-Host '  3) Stop monitored service and confirm card turns red within one heartbeat interval'
    Write-Host '  4) Restart service and confirm card returns green'
} finally {
    if ($agentProc -and -not $agentProc.HasExited) {
        Stop-Process -Id $agentProc.Id -Force
    }
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force
    }
}
