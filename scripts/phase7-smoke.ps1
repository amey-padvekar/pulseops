<#
Phase 7 smoke-check script

Goal:
- Produce repeatable evidence that an active incident gets an AI diagnosis and
  remediation recommendation, with artifacts under artifacts/phase7-smoke/<timestamp>.

Usage:
    pwsh -NoProfile -File .\scripts\phase7-smoke.ps1

Optional:
    -BackendUrl http://localhost:8080
    -TimeoutSeconds 180
    -PollIntervalSeconds 3
    -KeepProcesses

Artifacts produced:
- backend/agent/frontend stdout and stderr logs
- baseline incidents snapshot
- telemetry trigger request/response snapshots
- incident snapshot (full)
- incident investigation snapshot (redacted fields relevant to Phase 7)
- adk request metadata and response metadata snapshots (from backend logs)
- investigation trace summary line snapshot
- summary.json with pass/fail checks
-- optional dashboard screenshot placeholder note
#>

param(
    [int]$TimeoutSeconds = 180,
    [int]$PollIntervalSeconds = 3,
    [string]$BackendUrl = "http://localhost:8080",
    [switch]$KeepProcesses
)

$ErrorActionPreference = 'Stop'

function Write-JsonFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][object]$Value
    )
    $json = $Value | ConvertTo-Json -Depth 12
    [System.IO.File]::WriteAllText($Path, $json)
}

function Wait-Until {
    param(
        [Parameter(Mandatory = $true)][scriptblock]$Check,
        [int]$TimeoutSeconds = 30,
        [int]$IntervalMilliseconds = 500
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        if (& $Check) {
            return $true
        }
        Start-Sleep -Milliseconds $IntervalMilliseconds
    }

    return $false
}

function Select-LatestLogLine {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Pattern
    )
    if (-not (Test-Path $Path)) {
        return $null
    }

    $match = Select-String -Path $Path -Pattern $Pattern -SimpleMatch -ErrorAction SilentlyContinue | Select-Object -Last 1
    if (-not $match) {
        return $null
    }

    return $match.Line
}

function Parse-KeyValueLogLine {
    param(
        [Parameter(Mandatory = $true)][string]$Line
    )

    $obj = [ordered]@{}
    $pairs = [regex]::Matches($Line, '(?<k>[a-zA-Z0-9_\-]+)=(?<v>"[^"]*"|\S+)')
    foreach ($m in $pairs) {
        $k = $m.Groups['k'].Value
        $v = $m.Groups['v'].Value.Trim('"')
        $obj[$k] = $v
    }
    return $obj
}

function New-BackendCommand {
    param(
        [Parameter(Mandatory = $true)][string]$BackendDir
    )

    $lines = @(
        "Set-Location '$BackendDir'"
    )

    if (-not $env:AGENT_BUILDER_ENDPOINT -and -not $env:AGENT_BUILDER_ADK_ENDPOINT) {
        $lines += '$env:AGENT_BUILDER_ENDPOINT = ''http://127.0.0.1:1'''
        if (-not $env:AGENT_BUILDER_ENABLED) {
            $lines += '$env:AGENT_BUILDER_ENABLED = ''true'''
        }
        if (-not $env:AGENT_BUILDER_FALLBACK_MODE) {
            $lines += '$env:AGENT_BUILDER_FALLBACK_MODE = ''local_stub'''
        }
        if (-not $env:AGENT_BUILDER_TIMEOUT_MS) {
            $lines += '$env:AGENT_BUILDER_TIMEOUT_MS = ''2000'''
        }
        $lines += "Write-Host '[phase7-smoke] Agent Builder endpoint not configured; using local fallback mode.'"
    }

    $lines += 'go run ./cmd/server'
    return ($lines -join '; ')
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'

$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$outDir = Join-Path $repoRoot (Join-Path 'artifacts\phase7-smoke' $timestamp)
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

$backendOut = Join-Path $outDir 'backend.log'
$backendErr = Join-Path $outDir 'backend.err.log'
$agentOut = Join-Path $outDir 'agent.log'
$agentErr = Join-Path $outDir 'agent.err.log'
$frontendOut = Join-Path $outDir 'frontend.log'
$frontendErr = Join-Path $outDir 'frontend.err.log'

$healthUrl = "$BackendUrl/healthz"
$telemetryUrl = "$BackendUrl/telemetry"
$incidentsUrl = "$BackendUrl/incidents?active=true"

$procs = @()

Write-Host "[phase7-smoke] Artifacts directory: $outDir"

try {
    Write-Host '[phase7-smoke] Starting backend...'
    $backendProc = Start-Process powershell.exe -WorkingDirectory $backendDir -ArgumentList '-NoProfile', '-Command', (New-BackendCommand -BackendDir $backendDir) -RedirectStandardOutput $backendOut -RedirectStandardError $backendErr -PassThru
    $procs += $backendProc

    Write-Host '[phase7-smoke] Starting agent...'
    $agentProc = Start-Process go -WorkingDirectory $agentDir -ArgumentList 'run', './cmd/agent' -RedirectStandardOutput $agentOut -RedirectStandardError $agentErr -PassThru
    $procs += $agentProc

    Write-Host '[phase7-smoke] Starting frontend...'
    $frontendProc = Start-Process npm.cmd -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendOut -RedirectStandardError $frontendErr -PassThru
    $procs += $frontendProc

    Write-Host '[phase7-smoke] Waiting for backend health...'
    $healthy = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds 1000 -Check {
        try {
            $resp = Invoke-RestMethod -Method Get -Uri $healthUrl -TimeoutSec 2
            return ($resp.status -eq 'ok')
        } catch {
            return $false
        }
    }

    if (-not $healthy) {
        throw "Backend did not become healthy within timeout ($TimeoutSeconds s)."
    }

    $baseline = Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3
    Write-JsonFile -Path (Join-Path $outDir 'baseline_incidents.json') -Value $baseline

    $deviceId = if ($env:AGENT_DEVICE_ID) { $env:AGENT_DEVICE_ID } else { "phase7-smoke-$timestamp" }
    $serviceName = if ($env:MONITORED_SERVICE_NAME) { $env:MONITORED_SERVICE_NAME } else { 'OpenVPNService' }
    $requestId = "phase7-$timestamp-1"

    $telemetryPayloadObject = @{
        schemaVersion = '1.0.0'
        deviceId = $deviceId
        timestamp = (Get-Date).ToString('o')
        heartbeat = $true
        serviceName = $serviceName
        serviceStatus = 'stopped'
        networkReachable = $true
        cpuUsage = 14.2
        memoryUsage = 62.7
        recentLogs = @(
            'phase7 smoke: service stopped while heartbeat present',
            'phase7 smoke: restart recommended by reasoning'
        )
    }
    $telemetryPayloadJson = $telemetryPayloadObject | ConvertTo-Json -Depth 8

    Write-JsonFile -Path (Join-Path $outDir 'investigation_request.redacted.json') -Value $telemetryPayloadObject

    $telemetryResponse = Invoke-RestMethod -Method Post -Uri $telemetryUrl -Headers @{
        'X-PulseOps-Request-ID' = $requestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $deviceId
    } -ContentType 'application/json' -Body $telemetryPayloadJson -TimeoutSec 5

    Write-JsonFile -Path (Join-Path $outDir 'telemetry_post_response.json') -Value $telemetryResponse

    Write-Host "[phase7-smoke] Waiting for active incident for device $deviceId ..."
    $incident = $null
    $incidentReady = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $list = Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3
            if (-not ($list -is [System.Array])) {
                return $false
            }
            $match = $list | Where-Object { $_.deviceId -eq $deviceId } | Select-Object -First 1
            if (-not $match) {
                return $false
            }
            $script:incident = $match
            return $true
        } catch {
            return $false
        }
    }

    if (-not $incidentReady -or -not $incident) {
        throw 'No active incident found for smoke device within timeout.'
    }

    $incidentId = $incident.incidentId
    Write-Host "[phase7-smoke] Found incident: $incidentId"

    $incidentDetailUrl = "$BackendUrl/incidents/$incidentId"
    $finalIncident = $null
    $investigationReady = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $detail = Invoke-RestMethod -Method Get -Uri $incidentDetailUrl -TimeoutSec 3
            $script:finalIncident = $detail
            $hasStatus = -not [string]::IsNullOrWhiteSpace([string]$detail.investigationStatus)
            $hasResult = -not [string]::IsNullOrWhiteSpace([string]$detail.probableCause)
            return ($hasStatus -or $hasResult)
        } catch {
            return $false
        }
    }

    if (-not $investigationReady -or -not $finalIncident) {
        throw 'Investigation did not produce a status/result within timeout.'
    }

    Write-JsonFile -Path (Join-Path $outDir 'incident_api_snapshot.json') -Value $finalIncident

    $investigationResponseSnapshot = [ordered]@{
        incidentId = $finalIncident.incidentId
        deviceId = $finalIncident.deviceId
        investigationStatus = $finalIncident.investigationStatus
        probableCause = $finalIncident.probableCause
        confidence = $finalIncident.confidence
        recommendedActions = $finalIncident.recommendedActions
        validationSteps = $finalIncident.validationSteps
        summary = $finalIncident.summary
        agentBuilderTraceId = $finalIncident.agentBuilderTraceId
        investigatedAt = $finalIncident.investigatedAt
    }
    Write-JsonFile -Path (Join-Path $outDir 'investigation_response.redacted.json') -Value $investigationResponseSnapshot

    $requestLine = Select-LatestLogLine -Path $backendOut -Pattern 'agent_builder_request'
    $responseLine = Select-LatestLogLine -Path $backendOut -Pattern 'agent_builder_response'
    $traceLine = Select-LatestLogLine -Path $backendOut -Pattern 'agent_builder_trace'

    if ($requestLine) {
        Write-JsonFile -Path (Join-Path $outDir 'adk_request_metadata.redacted.json') -Value (Parse-KeyValueLogLine -Line $requestLine)
    }
    if ($responseLine) {
        Write-JsonFile -Path (Join-Path $outDir 'adk_response_metadata.redacted.json') -Value (Parse-KeyValueLogLine -Line $responseLine)
    }
    if ($traceLine) {
        Write-JsonFile -Path (Join-Path $outDir 'investigation_trace.redacted.json') -Value (Parse-KeyValueLogLine -Line $traceLine)
    }

    $allowedActionIds = @('restart_service', 'flush_dns', 'reconnect_vpn')
    $recommendedActions = @()
    if ($finalIncident.recommendedActions -is [System.Array]) {
        $recommendedActions = $finalIncident.recommendedActions
    }

    $invalidActions = @($recommendedActions | Where-Object { $allowedActionIds -notcontains $_.actionId })
    $hasProbableCause = -not [string]::IsNullOrWhiteSpace([string]$finalIncident.probableCause)
    $hasValidationSteps = ($finalIncident.validationSteps -is [System.Array]) -and ($finalIncident.validationSteps.Count -gt 0)
    $hasAllowedActions = ($recommendedActions.Count -gt 0) -and ($invalidActions.Count -eq 0)

    $summary = [ordered]@{
        runId = $timestamp
        backendUrl = $BackendUrl
        requestId = $requestId
        deviceId = $deviceId
        serviceName = $serviceName
        incidentId = $incidentId
        investigationStatus = $finalIncident.investigationStatus
        checks = [ordered]@{
            probableCausePresent = $hasProbableCause
            validationStepsPresent = $hasValidationSteps
            recommendedActionsAllowed = $hasAllowedActions
        }
        invalidActionIds = @($invalidActions | ForEach-Object { $_.actionId })
        artifactFiles = @(
            'backend.log',
            'backend.err.log',
            'agent.log',
            'agent.err.log',
            'frontend.log',
            'frontend.err.log',
            'adk_request_metadata.redacted.json',
            'adk_response_metadata.redacted.json',
            'investigation_request.redacted.json',
            'investigation_response.redacted.json',
            'incident_api_snapshot.json',
            'investigation_trace.redacted.json'
        )
        note = 'Optional: capture a dashboard screenshot and save it as dashboard.png in this directory.'
    }

    Write-JsonFile -Path (Join-Path $outDir 'summary.json') -Value $summary

    if ($hasProbableCause -and $hasValidationSteps -and $hasAllowedActions) {
        Write-Host '[phase7-smoke] PASS: Investigation produced probableCause, allowed recommendedActions, and validationSteps.'
    } else {
        Write-Warning '[phase7-smoke] Incomplete investigation output. Check summary.json and incident snapshot.'
    }

    Write-Host "[phase7-smoke] Evidence captured at: $outDir"
}
finally {
    if (-not $KeepProcesses) {
        Write-Host '[phase7-smoke] Stopping started processes...'
        foreach ($p in $procs) {
            try {
                if ($p -and -not $p.HasExited) {
                    Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
                }
            } catch {
                # best effort
            }
        }
    } else {
        Write-Host '[phase7-smoke] KeepProcesses specified; leaving services running.'
    }
}
