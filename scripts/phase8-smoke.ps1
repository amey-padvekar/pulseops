<#
Phase 8 smoke-check script

Goal:
- Produce repeatable evidence that an active incident with an AI recommendation can be
  approved by a human operator, transitioning the incident to `approved` with durable
  approver identity and timestamp. Artifacts land under artifacts/phase8-smoke/<timestamp>.

This drives the full Phase 8 proof path:
  detect -> investigate (recommendation) -> awaiting_approval -> POST /approve -> approved

To stay self-contained offline, the backend runs with
AGENT_BUILDER_FALLBACK_MODE=local_stub_actions, which synthesizes a deterministic,
catalog-valid recommendation (restart_service) and promotes the incident to
awaiting_approval. With a real Agent Builder/ADK endpoint configured, the same path runs
against the live recommendation instead.

Usage:
    pwsh -NoProfile -File .\scripts\phase8-smoke.ps1

Optional:
    -BackendUrl http://localhost:8080
    -TimeoutSeconds 180
    -PollIntervalSeconds 3
    -ApprovedBy demo.operator
    -SkipFrontend
    -KeepProcesses

Artifacts produced:
- backend/agent/frontend stdout and stderr logs
- baseline incidents snapshot
- telemetry trigger response snapshot
- incident snapshot at awaiting_approval (pre-approval)
- approval request and approval response snapshots
- incident snapshot after approval (post-approval)
- approval audit log line snapshot (redacted key/value form)
- summary.json with pass/fail checks
#>

param(
    [int]$TimeoutSeconds = 180,
    [int]$PollIntervalSeconds = 3,
    [string]$BackendUrl = "http://localhost:8080",
    [string]$ApprovedBy = "demo.operator",
    [switch]$SkipFrontend,
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

function Set-AgentBuilderEnvDefaults {
    # When no real Agent Builder/ADK endpoint is configured, point at an unreachable
    # address and use local_stub_actions, which yields a deterministic, catalog-valid
    # recommendation so the approval gate can be exercised offline. Env vars set here are
    # inherited by the launched backend; godotenv does not override already-set vars.
    if (-not $env:AGENT_BUILDER_ENDPOINT -and -not $env:AGENT_BUILDER_ADK_ENDPOINT) {
        $env:AGENT_BUILDER_ENDPOINT = 'http://127.0.0.1:1'
        if (-not $env:AGENT_BUILDER_ENABLED) { $env:AGENT_BUILDER_ENABLED = 'true' }
        if (-not $env:AGENT_BUILDER_FALLBACK_MODE) { $env:AGENT_BUILDER_FALLBACK_MODE = 'local_stub_actions' }
        if (-not $env:AGENT_BUILDER_TIMEOUT_MS) { $env:AGENT_BUILDER_TIMEOUT_MS = '2000' }
        Write-Host '[phase8-smoke] Agent Builder endpoint not configured; using local_stub_actions fallback.'
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$frontendDir = Join-Path $repoRoot 'frontend'

$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$outDir = Join-Path $repoRoot (Join-Path 'artifacts\phase8-smoke' $timestamp)
New-Item -ItemType Directory -Path $outDir -Force | Out-Null

$backendOut = Join-Path $outDir 'backend.log'
$backendErr = Join-Path $outDir 'backend.err.log'
$frontendOut = Join-Path $outDir 'frontend.log'
$frontendErr = Join-Path $outDir 'frontend.err.log'

$healthUrl = "$BackendUrl/healthz"
$telemetryUrl = "$BackendUrl/telemetry"
$incidentsUrl = "$BackendUrl/incidents?active=true"
$backendPort = ([uri]$BackendUrl).Port

$procs = @()

Write-Host "[phase8-smoke] Artifacts directory: $outDir"

try {
    # Pre-flight: refuse to run against a stale backend already holding the port, so the
    # proof always reflects the backend this script starts (go-run orphans are a known trap).
    if (@(Get-NetTCPConnection -LocalPort $backendPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw "Port $backendPort is already in use. Stop the existing backend before running this smoke check."
    }

    # Build a server binary and launch it directly so the tracked process IS the server.
    # This avoids `go run` orphaning the child (which leaks the port) and lets Stop-Process
    # flush the redirected stderr where the approval_audit line is written.
    $serverExe = Join-Path $outDir 'pulseops-server.exe'
    Write-Host '[phase8-smoke] Building backend...'
    Push-Location $backendDir
    try {
        & go build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Set-AgentBuilderEnvDefaults

    Write-Host '[phase8-smoke] Starting backend...'
    $backendProc = Start-Process $serverExe -WorkingDirectory $backendDir -RedirectStandardOutput $backendOut -RedirectStandardError $backendErr -PassThru
    $procs += $backendProc

    # Telemetry is injected directly by this script (below), so the device agent is not
    # required for the approval proof and is intentionally not started.

    if (-not $SkipFrontend) {
        Write-Host '[phase8-smoke] Starting frontend...'
        try {
            $frontendProc = Start-Process npm.cmd -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendOut -RedirectStandardError $frontendErr -PassThru
            $procs += $frontendProc
        } catch {
            Write-Warning "[phase8-smoke] Frontend failed to start (non-fatal): $($_.Exception.Message)"
        }
    }

    Write-Host '[phase8-smoke] Waiting for backend health...'
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

    $deviceId = if ($env:AGENT_DEVICE_ID) { $env:AGENT_DEVICE_ID } else { "phase8-smoke-$timestamp" }
    $serviceName = if ($env:MONITORED_SERVICE_NAME) { $env:MONITORED_SERVICE_NAME } else { 'OpenVPNService' }
    $requestId = "phase8-$timestamp-1"

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
            'phase8 smoke: service stopped while heartbeat present',
            'phase8 smoke: restart recommended by reasoning'
        )
    }
    $telemetryPayloadJson = $telemetryPayloadObject | ConvertTo-Json -Depth 8

    $telemetryResponse = Invoke-RestMethod -Method Post -Uri $telemetryUrl -Headers @{
        'X-PulseOps-Request-ID' = $requestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $deviceId
    } -ContentType 'application/json' -Body $telemetryPayloadJson -TimeoutSec 5

    Write-JsonFile -Path (Join-Path $outDir 'telemetry_post_response.json') -Value $telemetryResponse

    Write-Host "[phase8-smoke] Waiting for incident to reach awaiting_approval for device $deviceId ..."
    $awaiting = $null
    $awaitingReady = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $list = Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3
            if (-not ($list -is [System.Array])) {
                return $false
            }
            $match = $list | Where-Object { $_.deviceId -eq $deviceId } | Select-Object -First 1
            if (-not $match) {
                return $false
            }
            $hasActions = ($match.recommendedActions -is [System.Array]) -and ($match.recommendedActions.Count -gt 0)
            if (($match.state -eq 'awaiting_approval') -and $hasActions) {
                $script:awaiting = $match
                return $true
            }
            return $false
        } catch {
            return $false
        }
    }

    if (-not $awaitingReady -or -not $awaiting) {
        throw 'Incident did not reach awaiting_approval with a recommendation within timeout.'
    }

    $incidentId = $awaiting.incidentId
    Write-Host "[phase8-smoke] Incident awaiting approval: $incidentId"
    Write-JsonFile -Path (Join-Path $outDir 'incident_awaiting_snapshot.json') -Value $awaiting

    # Build the approval request strictly from the recommendation attached to the incident.
    $selectedActionIds = @($awaiting.recommendedActions | ForEach-Object { $_.actionId })
    $approvalRequestObject = [ordered]@{
        approvedBy = $ApprovedBy
        selectedActionIds = $selectedActionIds
        note = 'Phase 8 smoke approval'
    }
    Write-JsonFile -Path (Join-Path $outDir 'approval_request.json') -Value $approvalRequestObject
    $approvalRequestJson = $approvalRequestObject | ConvertTo-Json -Depth 6

    $approveUrl = "$BackendUrl/incidents/$incidentId/approve"
    Write-Host "[phase8-smoke] Submitting approval to $approveUrl ..."
    $approvalResponse = Invoke-RestMethod -Method Post -Uri $approveUrl -ContentType 'application/json' -Body $approvalRequestJson -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $outDir 'approval_response.json') -Value $approvalResponse

    # Confirm durability via a fresh REST read (hard-refresh path).
    $incidentDetailUrl = "$BackendUrl/incidents/$incidentId"
    $approved = $null
    $approvedReady = Wait-Until -TimeoutSeconds 30 -IntervalMilliseconds 500 -Check {
        try {
            $detail = Invoke-RestMethod -Method Get -Uri $incidentDetailUrl -TimeoutSec 3
            $script:approved = $detail
            return ($detail.state -eq 'approved')
        } catch {
            return $false
        }
    }

    if (-not $approvedReady -or -not $approved) {
        throw 'Incident did not reach approved state after approval submission.'
    }

    Write-JsonFile -Path (Join-Path $outDir 'incident_approved_snapshot.json') -Value $approved

    # The Go log package writes to stderr. Start-Process buffers the redirected stream,
    # so the audit line only lands in backend.err.log once the backend's stderr handle
    # closes. The approval is already complete and verified, so stop the backend now to
    # flush, then read the structured approval_audit evidence line.
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force -ErrorAction SilentlyContinue
    }
    $auditLine = $null
    [void](Wait-Until -TimeoutSeconds 10 -IntervalMilliseconds 500 -Check {
        $line = Select-LatestLogLine -Path $backendErr -Pattern 'approval_audit'
        if ($line) {
            $script:auditLine = $line
            return $true
        }
        return $false
    })
    if ($auditLine) {
        Write-JsonFile -Path (Join-Path $outDir 'approval_audit.redacted.json') -Value (Parse-KeyValueLogLine -Line $auditLine)
    } else {
        Write-Warning '[phase8-smoke] approval_audit log line not found in backend.err.log.'
    }

    # Checks.
    $recommendationVisible = ($awaiting.state -eq 'awaiting_approval') -and ($selectedActionIds.Count -gt 0)
    $stateApproved = ($approved.state -eq 'approved')
    $approverRecorded = -not [string]::IsNullOrWhiteSpace([string]$approved.approvedBy)
    $timestampRecorded = -not [string]::IsNullOrWhiteSpace([string]$approved.approvedAt)
    $queuedActionsPresent = ($approvalResponse.queuedActions -is [System.Array]) -and ($approvalResponse.queuedActions.Count -gt 0)
    $approvedActionsMatch = $true
    foreach ($id in $selectedActionIds) {
        if (@($approved.approvedActions) -notcontains $id) {
            $approvedActionsMatch = $false
        }
    }

    $summary = [ordered]@{
        runId = $timestamp
        backendUrl = $BackendUrl
        requestId = $requestId
        deviceId = $deviceId
        serviceName = $serviceName
        incidentId = $incidentId
        approvedBy = $approved.approvedBy
        approvedAt = $approved.approvedAt
        selectedActionIds = $selectedActionIds
        finalState = $approved.state
        checks = [ordered]@{
            recommendationVisible = $recommendationVisible
            approvalAccepted = $stateApproved
            stateApproved = $stateApproved
            approverRecorded = $approverRecorded
            timestampRecorded = $timestampRecorded
            queuedActionsPresent = $queuedActionsPresent
            approvedActionsMatchSelection = $approvedActionsMatch
        }
        artifactFiles = @(
            'backend.log',
            'backend.err.log',
            'frontend.log',
            'frontend.err.log',
            'baseline_incidents.json',
            'telemetry_post_response.json',
            'incident_awaiting_snapshot.json',
            'approval_request.json',
            'approval_response.json',
            'incident_approved_snapshot.json',
            'approval_audit.redacted.json'
        )
        note = 'Optional: capture a dashboard screenshot of the approved card and save it as dashboard.png in this directory.'
    }

    Write-JsonFile -Path (Join-Path $outDir 'summary.json') -Value $summary

    if ($recommendationVisible -and $stateApproved -and $approverRecorded -and $timestampRecorded -and $queuedActionsPresent -and $approvedActionsMatch) {
        Write-Host '[phase8-smoke] PASS: Recommendation approved; incident is approved with approver, timestamp, and queued actions.'
    } else {
        Write-Warning '[phase8-smoke] Incomplete approval proof. Check summary.json and incident snapshots.'
    }

    Write-Host "[phase8-smoke] Evidence captured at: $outDir"
}
finally {
    if (-not $KeepProcesses) {
        Write-Host '[phase8-smoke] Stopping started processes...'
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
        Write-Host '[phase8-smoke] KeepProcesses specified; leaving services running.'
    }
}
