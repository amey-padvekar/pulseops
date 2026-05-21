$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'
$backendUrl = 'http://localhost:8080/healthz'
$evidenceRootDir = Join-Path $repoRoot 'artifacts\phase2-smoke'
$runId = Get-Date -Format 'yyyyMMdd-HHmmss'
$evidenceDir = Join-Path $evidenceRootDir $runId
$backendLog = Join-Path $evidenceDir 'backend.log'
$backendErrLog = Join-Path $evidenceDir 'backend.err.log'
$agentLog = Join-Path $evidenceDir 'agent.log'
$agentErrLog = Join-Path $evidenceDir 'agent.err.log'

New-Item -ItemType Directory -Path $evidenceDir -Force | Out-Null

Write-Host '[smoke-check] Building agent...'
Push-Location $agentDir
go build ./...
Pop-Location

Write-Host '[smoke-check] Building backend...'
Push-Location $backendDir
go build ./...
Pop-Location

Write-Host '[smoke-check] Building frontend...'
Push-Location $frontendDir
npm run build | Out-Null
Pop-Location

Write-Host '[smoke-check] Starting backend and agent for telemetry verification...'
$backendProc = Start-Process go -WorkingDirectory $backendDir -ArgumentList 'run', './cmd/server' -RedirectStandardOutput $backendLog -RedirectStandardError $backendErrLog -PassThru
$agentProc = Start-Process go -WorkingDirectory $agentDir -ArgumentList 'run', './cmd/agent' -RedirectStandardOutput $agentLog -RedirectStandardError $agentErrLog -PassThru

try {
    $usingExistingBackend = $false
    $healthy = $false
    for ($i = 0; $i -lt 20; $i++) {
        try {
            $resp = Invoke-RestMethod -Uri $backendUrl -Method Get -TimeoutSec 2
            if ($resp.status -eq 'ok') {
                $healthy = $true
                break
            }
        } catch {
            Start-Sleep -Milliseconds 500
        }
    }

    if (-not $healthy) {
        throw 'Backend /healthz did not return status=ok within timeout.'
    }

    if ($backendProc.HasExited) {
        $usingExistingBackend = $true
        Write-Host '[smoke-check] NOTE: Started backend process exited early. Continuing against existing backend on port 8080.'
    }

    $backendTelemetrySeen = $false
    $agentDeliverySeen = $false
    for ($i = 0; $i -lt 30; $i++) {
        $backendContent = ''
        if (Test-Path $backendLog) {
            $backendContent += (Get-Content $backendLog -Raw)
        }
        if (Test-Path $backendErrLog) {
            $backendContent += (Get-Content $backendErrLog -Raw)
        }

        $agentContent = ''
        if (Test-Path $agentLog) {
            $agentContent += (Get-Content $agentLog -Raw)
        }
        if (Test-Path $agentErrLog) {
            $agentContent += (Get-Content $agentErrLog -Raw)
        }

        if ($backendContent -match 'telemetry received') {
            $backendTelemetrySeen = $true
        }
        if ($agentContent -match 'heartbeat delivery succeeded' -and $agentContent -match 'status_code=20[0-9]') {
            $agentDeliverySeen = $true
        }

        if ($backendTelemetrySeen -or $agentDeliverySeen) {
            break
        }
        Start-Sleep -Milliseconds 500
    }

    if (-not $backendTelemetrySeen -and -not $agentDeliverySeen) {
        throw "Telemetry was not observed in backend logs. Check $backendLog, $backendErrLog, $agentLog, and $agentErrLog"
    }

    if ($backendTelemetrySeen) {
        Write-Host '[smoke-check] PASS: build checks succeeded, backend health is ok, and backend telemetry ingestion was observed.'
    } elseif ($agentDeliverySeen) {
        Write-Host '[smoke-check] PASS: build checks succeeded, backend health is ok, and agent confirmed successful telemetry delivery.'
    }
    if ($usingExistingBackend) {
        Write-Host '[smoke-check] NOTE: Evidence may reflect an already-running backend process outside this script.'
    }
    Write-Host "[smoke-check] Evidence logs written to: $evidenceDir"
} finally {
    if ($agentProc -and -not $agentProc.HasExited) {
        Stop-Process -Id $agentProc.Id -Force
    }
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force
    }
}
