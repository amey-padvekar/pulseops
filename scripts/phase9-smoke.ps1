<#
Phase 9 smoke-check script

Goal:
- Produce repeatable evidence that an approved incident is dispatched to the real Go
  agent, executed as a bounded platform action, and reported back to the backend, with
  the execution outcome (status, per-action stdout/stderr/exit code, timeline) visible
  on the incident the dashboard renders. Artifacts land under
  artifacts/phase9-smoke/<timestamp>.

This drives the full Phase 9 proof path end to end, using the REAL agent binary:
  detect -> investigate -> awaiting_approval -> approve
        -> backend queues command
        -> agent polls GET /devices/{deviceId}/commands (dispatch; incident -> executing)
        -> agent maps the approved action to a platform op and executes it
        -> agent POSTs the ExecutionResult to /remediation/results
        -> backend persists the result and advances the incident (validating | failed)

To stay self-contained offline, the backend runs with
AGENT_BUILDER_FALLBACK_MODE=local_stub_actions, which synthesizes a deterministic,
catalog-valid recommendation (restart_service) and promotes the incident to
awaiting_approval. The approved action is restart_service on the monitored service.

Reliability note (task 4.12.3): the action list is intentionally a single catalog
action. The remediation LOOP always completes regardless of whether the underlying OS
command succeeds: on a host without the monitored service, restart_service fails and the
incident moves to `failed` with the captured stderr -- still real execution with logs.
On a host where the service exists, it succeeds and the incident moves to `validating`.
Set -ServiceName to a restartable service for a green-path demo.

Usage:
    pwsh -NoProfile -File .\scripts\phase9-smoke.ps1

Optional:
    -BackendUrl http://localhost:8080
    -TimeoutSeconds 180
    -PollIntervalSeconds 3
    -ApprovedBy demo.operator
    -ServiceName OpenVPNService
    -SkipFrontend
    -KeepProcesses

Artifacts produced:
- backend/agent/frontend stdout and stderr logs
- baseline incidents snapshot
- telemetry trigger response snapshot
- incident snapshot at awaiting_approval (pre-approval)
- approval request and response snapshots
- incident snapshot after approval (command queued)
- incident snapshot after execution (the dashboard-visible execution result + timeline)
- parsed backend dispatch + result-ingestion log evidence
- parsed agent receipt + execution log evidence
- summary.json with pass/fail checks
#>

param(
    [int]$TimeoutSeconds = 180,
    [int]$PollIntervalSeconds = 3,
    [string]$BackendUrl = "http://localhost:8080",
    [string]$ApprovedBy = "demo.operator",
    [string]$ServiceName = "OpenVPNService",
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
        Write-Host '[phase9-smoke] Agent Builder endpoint not configured; using local_stub_actions fallback.'
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'

$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$outDir = Join-Path $repoRoot (Join-Path 'artifacts\phase9-smoke' $timestamp)
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
$backendPort = ([uri]$BackendUrl).Port

$procs = @()

Write-Host "[phase9-smoke] Artifacts directory: $outDir"

try {
    # Pre-flight: refuse to run against a stale backend already holding the port.
    if (@(Get-NetTCPConnection -LocalPort $backendPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw "Port $backendPort is already in use. Stop the existing backend before running this smoke check."
    }

    # Build backend and agent binaries so the tracked processes ARE the services (no
    # go-run orphans that leak the port and swallow redirected logs).
    $serverExe = Join-Path $outDir 'pulseops-server.exe'
    $agentExe = Join-Path $outDir 'pulseops-agent.exe'

    Write-Host '[phase9-smoke] Building backend...'
    Push-Location $backendDir
    try {
        & go build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "backend go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Write-Host '[phase9-smoke] Building agent...'
    Push-Location $agentDir
    try {
        & go build -o $agentExe ./cmd/agent
        if ($LASTEXITCODE -ne 0) { throw "agent go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Set-AgentBuilderEnvDefaults

    Write-Host '[phase9-smoke] Starting backend...'
    $backendProc = Start-Process $serverExe -WorkingDirectory $backendDir -RedirectStandardOutput $backendOut -RedirectStandardError $backendErr -PassThru
    $procs += $backendProc

    if (-not $SkipFrontend) {
        Write-Host '[phase9-smoke] Starting frontend...'
        try {
            $frontendProc = Start-Process npm.cmd -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendOut -RedirectStandardError $frontendErr -PassThru
            $procs += $frontendProc
        } catch {
            Write-Warning "[phase9-smoke] Frontend failed to start (non-fatal): $($_.Exception.Message)"
        }
    }

    Write-Host '[phase9-smoke] Waiting for backend health...'
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

    # Unique device id so the proof reflects this run only. The real agent is launched
    # with the SAME device id so it fetches this incident's command.
    $deviceId = "phase9-smoke-$timestamp"
    $requestId = "phase9-$timestamp-1"

    # 1) Inject telemetry (service stopped) to drive detection -> investigation.
    $telemetryPayloadObject = @{
        schemaVersion = '1.0.0'
        deviceId = $deviceId
        timestamp = (Get-Date).ToString('o')
        heartbeat = $true
        serviceName = $ServiceName
        serviceStatus = 'stopped'
        networkReachable = $true
        cpuUsage = 14.2
        memoryUsage = 62.7
        recentLogs = @(
            'phase9 smoke: service stopped while heartbeat present',
            'phase9 smoke: restart recommended by reasoning'
        )
    }
    $telemetryPayloadJson = $telemetryPayloadObject | ConvertTo-Json -Depth 8
    $telemetryResponse = Invoke-RestMethod -Method Post -Uri $telemetryUrl -Headers @{
        'X-PulseOps-Request-ID' = $requestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $deviceId
    } -ContentType 'application/json' -Body $telemetryPayloadJson -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $outDir 'telemetry_post_response.json') -Value $telemetryResponse

    # 2) Wait for awaiting_approval with a recommendation.
    Write-Host "[phase9-smoke] Waiting for incident to reach awaiting_approval for device $deviceId ..."
    $awaiting = $null
    $awaitingReady = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $list = Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3
            if (-not ($list -is [System.Array])) { return $false }
            $match = $list | Where-Object { $_.deviceId -eq $deviceId } | Select-Object -First 1
            if (-not $match) { return $false }
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
    Write-Host "[phase9-smoke] Incident awaiting approval: $incidentId"
    Write-JsonFile -Path (Join-Path $outDir 'incident_awaiting_snapshot.json') -Value $awaiting

    # 3) Approve, strictly from the attached recommendation.
    $selectedActionIds = @($awaiting.recommendedActions | ForEach-Object { $_.actionId })
    $approvalRequestObject = [ordered]@{
        approvedBy = $ApprovedBy
        selectedActionIds = $selectedActionIds
        note = 'Phase 9 smoke approval'
    }
    Write-JsonFile -Path (Join-Path $outDir 'approval_request.json') -Value $approvalRequestObject
    $approveUrl = "$BackendUrl/incidents/$incidentId/approve"
    Write-Host "[phase9-smoke] Submitting approval to $approveUrl ..."
    $approvalResponse = Invoke-RestMethod -Method Post -Uri $approveUrl -ContentType 'application/json' -Body ($approvalRequestObject | ConvertTo-Json -Depth 6) -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $outDir 'approval_response.json') -Value $approvalResponse

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

    # 4) Start the REAL agent so it discovers, executes, and reports the command. A high
    # heartbeat interval keeps the agent's own telemetry from adding noise; the fast
    # remediation poll picks up the queued command quickly. The agent device id matches
    # the incident device so the command routes to it.
    $env:APP_ENV = 'development'
    $env:AGENT_DEVICE_ID = $deviceId
    $env:MONITORED_SERVICE_NAME = $ServiceName
    $env:BACKEND_BASE_URL = $BackendUrl
    $env:AGENT_REMEDIATION_POLL_INTERVAL_SEC = '2'
    $env:AGENT_HEARTBEAT_INTERVAL_SEC = '60'
    $env:AGENT_REQUEST_TIMEOUT_MS = '5000'
    $env:ENABLE_SIMULATED_LOGS = 'true'

    Write-Host "[phase9-smoke] Starting agent (device=$deviceId, service=$ServiceName)..."
    $agentProc = Start-Process $agentExe -WorkingDirectory $agentDir -RedirectStandardOutput $agentOut -RedirectStandardError $agentErr -PassThru
    $procs += $agentProc

    # Best-effort capture of the executing state right after dispatch (may be brief).
    [void](Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 300 -Check {
        try {
            $detail = Invoke-RestMethod -Method Get -Uri $incidentDetailUrl -TimeoutSec 3
            if ($detail.state -eq 'executing') {
                Write-JsonFile -Path (Join-Path $outDir 'incident_executing_snapshot.json') -Value $detail
                return $true
            }
            return $false
        } catch {
            return $false
        }
    })

    # 5) Wait for the execution result to land on the incident: a remediation status, a
    # finished timeline event, and a terminal-for-Phase-9 state (validating | failed).
    Write-Host '[phase9-smoke] Waiting for the agent to execute and report the result...'
    $executed = $null
    $executedReady = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $detail = Invoke-RestMethod -Method Get -Uri $incidentDetailUrl -TimeoutSec 3
            $script:executed = $detail
            $hasStatus = -not [string]::IsNullOrWhiteSpace([string]$detail.remediationStatus)
            $hasFinished = ($detail.timeline -is [System.Array]) -and (@($detail.timeline | Where-Object { $_.type -eq 'command_finished' }).Count -gt 0)
            $terminal = ($detail.state -eq 'validating') -or ($detail.state -eq 'failed')
            return ($hasStatus -and $hasFinished -and $terminal)
        } catch {
            return $false
        }
    }
    if (-not $executedReady -or -not $executed) {
        throw 'Agent did not execute and report a remediation result within timeout.'
    }
    Write-JsonFile -Path (Join-Path $outDir 'incident_executed_snapshot.json') -Value $executed

    # Stop processes to flush the redirected log streams, then mine evidence lines.
    Write-Host '[phase9-smoke] Stopping processes to flush logs...'
    foreach ($p in @($agentProc, $backendProc)) {
        if ($p -and -not $p.HasExited) {
            Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue
        }
    }

    $dispatchLine = $null
    [void](Wait-Until -TimeoutSeconds 10 -IntervalMilliseconds 500 -Check {
        $line = Select-LatestLogLine -Path $backendErr -Pattern 'remediation command dispatched'
        if ($line) { $script:dispatchLine = $line; return $true }
        return $false
    })
    if ($dispatchLine) {
        Write-JsonFile -Path (Join-Path $outDir 'backend_dispatch_log.json') -Value (Parse-KeyValueLogLine -Line $dispatchLine)
    }

    $ingestLine = Select-LatestLogLine -Path $backendErr -Pattern 'remediation result ingested'
    if ($ingestLine) {
        Write-JsonFile -Path (Join-Path $outDir 'backend_result_ingested_log.json') -Value (Parse-KeyValueLogLine -Line $ingestLine)
    }

    $agentReceiveLine = Select-LatestLogLine -Path $agentErr -Pattern 'remediation command received'
    if ($agentReceiveLine) {
        Write-JsonFile -Path (Join-Path $outDir 'agent_command_received_log.json') -Value (Parse-KeyValueLogLine -Line $agentReceiveLine)
    }
    $agentExecLine = Select-LatestLogLine -Path $agentErr -Pattern 'remediation execution finished'
    if ($agentExecLine) {
        Write-JsonFile -Path (Join-Path $outDir 'agent_execution_finished_log.json') -Value (Parse-KeyValueLogLine -Line $agentExecLine)
    }

    # 6) Checks.
    $approvedReached = ($approved.state -eq 'approved')
    $dispatched = ($executed.timeline -is [System.Array]) -and (@($executed.timeline | Where-Object { $_.type -eq 'command_dispatched' }).Count -gt 0)
    $resultReceived = -not [string]::IsNullOrWhiteSpace([string]$executed.remediationStatus)
    $results = @($executed.remediationResults)
    $executionDetailPresent = ($results.Count -gt 0) -and (-not [string]::IsNullOrWhiteSpace([string]$results[0].actionId)) -and (-not [string]::IsNullOrWhiteSpace([string]$results[0].status))
    $finishedRecorded = ($executed.timeline -is [System.Array]) -and (@($executed.timeline | Where-Object { $_.type -eq 'command_finished' }).Count -gt 0)
    $terminalState = ($executed.state -eq 'validating') -or ($executed.state -eq 'failed')
    $actionSucceeded = ($executed.remediationStatus -eq 'succeeded')

    $loopComplete = $approvedReached -and $dispatched -and $resultReceived -and $executionDetailPresent -and $finishedRecorded -and $terminalState

    $summary = [ordered]@{
        runId = $timestamp
        backendUrl = $BackendUrl
        requestId = $requestId
        deviceId = $deviceId
        serviceName = $ServiceName
        incidentId = $incidentId
        approvedBy = $approved.approvedBy
        selectedActionIds = $selectedActionIds
        finalState = $executed.state
        remediationStatus = $executed.remediationStatus
        remediationRequestId = $executed.remediationRequestId
        actionSucceeded = $actionSucceeded
        checks = [ordered]@{
            approvedReached = $approvedReached
            commandDispatched = $dispatched
            resultReceived = $resultReceived
            executionDetailPresent = $executionDetailPresent
            finishedTimelineRecorded = $finishedRecorded
            terminalState = $terminalState
            loopComplete = $loopComplete
        }
        artifactFiles = @(
            'backend.log',
            'backend.err.log',
            'agent.log',
            'agent.err.log',
            'frontend.log',
            'frontend.err.log',
            'baseline_incidents.json',
            'telemetry_post_response.json',
            'incident_awaiting_snapshot.json',
            'approval_request.json',
            'approval_response.json',
            'incident_approved_snapshot.json',
            'incident_executing_snapshot.json',
            'incident_executed_snapshot.json',
            'backend_dispatch_log.json',
            'backend_result_ingested_log.json',
            'agent_command_received_log.json',
            'agent_execution_finished_log.json'
        )
        note = 'incident_executed_snapshot.json holds the execution status, per-action stdout/stderr/exitCode, and timeline the dashboard execution panel renders. Optionally capture a dashboard screenshot as dashboard.png in this directory.'
    }
    Write-JsonFile -Path (Join-Path $outDir 'summary.json') -Value $summary

    if ($loopComplete) {
        $outcome = if ($actionSucceeded) { 'succeeded' } else { "completed with status '$($executed.remediationStatus)' (final state $($executed.state))" }
        Write-Host "[phase9-smoke] PASS: dispatch -> agent execution -> result ingestion loop $outcome. Execution detail captured for the dashboard."
    } else {
        Write-Warning '[phase9-smoke] Incomplete execution proof. Check summary.json and incident snapshots.'
    }

    Write-Host "[phase9-smoke] Evidence captured at: $outDir"
}
finally {
    if (-not $KeepProcesses) {
        Write-Host '[phase9-smoke] Stopping started processes...'
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
        Write-Host '[phase9-smoke] KeepProcesses specified; leaving services running.'
    }

    # The built binaries are rebuildable scaffolding, not evidence; drop them so the
    # artifact directory stays lean (best effort, only once processes are stopped).
    if (-not $KeepProcesses) {
        foreach ($exe in @($serverExe, $agentExe)) {
            if ($exe -and (Test-Path $exe)) {
                Remove-Item -Path $exe -Force -ErrorAction SilentlyContinue
            }
        }
    }
}
