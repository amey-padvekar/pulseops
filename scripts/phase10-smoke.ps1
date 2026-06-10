<#
Phase 10 smoke-check script

Goal:
- Produce repeatable, observable evidence that recovery is CONFIRMED by fresh telemetry
  before an incident closes, and that a non-recovering endpoint deterministically fails
  validation instead of silently closing. Artifacts land under
  artifacts/phase10-smoke/<timestamp>.

This drives the Phase 10 proof path end to end against the REAL backend validation engine.
Phase 9 already proves real agent execution; Phase 10 validation is purely backend
telemetry-driven, so this script stays host-independent by simulating the agent's HTTP
contract (command fetch + result report) rather than launching the agent binary or
requiring a restartable OS service:

  POSITIVE (recovery proven):
    detect (stopped telemetry) -> investigate -> awaiting_approval -> approve
      -> GET /devices/{id}/commands         (dispatch; incident -> executing)
      -> POST /remediation/results succeeded (incident -> validating)
      -> POST 2 healthy telemetry cycles     (running + heartbeat)
      -> incident -> resolved                (validationStatus=succeeded)

  NEGATIVE (recovery not proven):
    same path to validating, then the service never recovers
      -> POST unhealthy telemetry cycles     (still stopped)
      -> validation window elapses
      -> incident -> failed                  (validationStatus=failed, reason recorded)

The backend runs with AGENT_BUILDER_FALLBACK_MODE=local_stub_actions so the approval gate
can be exercised offline with a deterministic catalog-valid recommendation.

Usage:
    pwsh -NoProfile -File .\scripts\phase10-smoke.ps1

Optional:
    -BackendUrl http://localhost:8080
    -TimeoutSeconds 180            (per-step wait budget)
    -ValidationWaitSeconds 100     (max wait for the negative-path timeout failure)
    -PollIntervalSeconds 2
    -ApprovedBy demo.operator
    -ServiceName OpenVPNService
    -SkipFrontend
    -KeepProcesses

Artifacts produced (per scenario, under green/ and red/):
- incident snapshots at awaiting_approval, executing, validating, and terminal
- telemetry post responses for each injected cycle
- summary.json with pass/fail checks for both scenarios
#>

param(
    [int]$TimeoutSeconds = 180,
    [int]$ValidationWaitSeconds = 100,
    [int]$PollIntervalSeconds = 2,
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

function Set-AgentBuilderEnvDefaults {
    # Offline-safe: point at an unreachable Agent Builder endpoint and use the local stub
    # so detection still produces a catalog-valid recommendation and reaches awaiting_approval.
    if (-not $env:AGENT_BUILDER_ENDPOINT -and -not $env:AGENT_BUILDER_ADK_ENDPOINT) {
        $env:AGENT_BUILDER_ENDPOINT = 'http://127.0.0.1:1'
        if (-not $env:AGENT_BUILDER_ENABLED) { $env:AGENT_BUILDER_ENABLED = 'true' }
        if (-not $env:AGENT_BUILDER_FALLBACK_MODE) { $env:AGENT_BUILDER_FALLBACK_MODE = 'local_stub_actions' }
        if (-not $env:AGENT_BUILDER_TIMEOUT_MS) { $env:AGENT_BUILDER_TIMEOUT_MS = '2000' }
        Write-Host '[phase10-smoke] Agent Builder endpoint not configured; using local_stub_actions fallback.'
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$frontendDir = Join-Path $repoRoot 'frontend'

$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$outDir = Join-Path $repoRoot (Join-Path 'artifacts\phase10-smoke' $timestamp)
New-Item -ItemType Directory -Path $outDir -Force | Out-Null
$greenDir = Join-Path $outDir 'green'
$redDir = Join-Path $outDir 'red'
New-Item -ItemType Directory -Path $greenDir -Force | Out-Null
New-Item -ItemType Directory -Path $redDir -Force | Out-Null

$backendOut = Join-Path $outDir 'backend.log'
$backendErr = Join-Path $outDir 'backend.err.log'
$frontendOut = Join-Path $outDir 'frontend.log'
$frontendErr = Join-Path $outDir 'frontend.err.log'

$healthUrl = "$BackendUrl/healthz"
$telemetryUrl = "$BackendUrl/telemetry"
$incidentsUrl = "$BackendUrl/incidents?active=true"
$backendPort = ([uri]$BackendUrl).Port

$procs = @()
$serverExe = $null

# Post one telemetry cycle for a device. $Status is the serviceStatus ('stopped'|'running').
function Send-Telemetry {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceId,
        [Parameter(Mandatory = $true)][string]$Status,
        [Parameter(Mandatory = $true)][bool]$Heartbeat,
        [string]$RequestId = 'phase10'
    )
    $payload = @{
        schemaVersion = '1.0.0'
        deviceId = $DeviceId
        timestamp = (Get-Date).ToString('o')
        heartbeat = $Heartbeat
        serviceName = $ServiceName
        serviceStatus = $Status
        networkReachable = $true
        cpuUsage = 11.0
        memoryUsage = 41.0
        recentLogs = @("phase10 smoke: service $Status")
    }
    return Invoke-RestMethod -Method Post -Uri $telemetryUrl -Headers @{
        'X-PulseOps-Request-ID' = $RequestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $DeviceId
    } -ContentType 'application/json' -Body ($payload | ConvertTo-Json -Depth 8) -TimeoutSec 5
}

function Get-IncidentDetail {
    param([Parameter(Mandatory = $true)][string]$IncidentId)
    return Invoke-RestMethod -Method Get -Uri "$BackendUrl/incidents/$IncidentId" -TimeoutSec 3
}

# Drive a device from a stopped-service detection all the way to `validating`, simulating
# the agent's command fetch and a successful execution result. Returns a hashtable with
# incidentId and requestId. Per-step snapshots are written under $ScenarioDir.
function Invoke-DriveToValidating {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceId,
        [Parameter(Mandatory = $true)][string]$ScenarioDir,
        [Parameter(Mandatory = $true)][string]$Tag
    )

    # 1) Inject stopped telemetry -> detection -> investigation -> awaiting_approval.
    $tResp = Send-Telemetry -DeviceId $DeviceId -Status 'stopped' -Heartbeat $true -RequestId "$Tag-detect"
    Write-JsonFile -Path (Join-Path $ScenarioDir 'telemetry_detect_response.json') -Value $tResp

    Write-Host "[phase10-smoke][$Tag] Waiting for awaiting_approval (device $DeviceId)..."
    # Capture via $script: scope so the value survives the Wait-Until scriptblock (a child
    # scope) and is readable here; a function-local would stay $null.
    $script:p10_awaiting = $null
    $ok = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            # @() forces an array: Invoke-RestMethod unwraps a single-element JSON array
            # into a scalar, which would otherwise hide a lone incident from the filter.
            $list = @(Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3)
            $m = $list | Where-Object { $_.deviceId -eq $DeviceId } | Select-Object -First 1
            if (-not $m) { return $false }
            $hasActions = @($m.recommendedActions).Count -gt 0
            if (($m.state -eq 'awaiting_approval') -and $hasActions) { $script:p10_awaiting = $m; return $true }
            return $false
        } catch { return $false }
    }
    $awaiting = $script:p10_awaiting
    if (-not $ok -or -not $awaiting) { throw "[$Tag] incident did not reach awaiting_approval with a recommendation." }
    $incidentId = $awaiting.incidentId
    Write-JsonFile -Path (Join-Path $ScenarioDir 'incident_awaiting_snapshot.json') -Value $awaiting

    # 2) Approve strictly from the attached recommendation.
    $selectedActionIds = @($awaiting.recommendedActions | ForEach-Object { $_.actionId })
    $approvalReq = [ordered]@{ approvedBy = $ApprovedBy; selectedActionIds = $selectedActionIds; note = "Phase 10 smoke ($Tag)" }
    $approveResp = Invoke-RestMethod -Method Post -Uri "$BackendUrl/incidents/$incidentId/approve" -ContentType 'application/json' -Body ($approvalReq | ConvertTo-Json -Depth 6) -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $ScenarioDir 'approval_response.json') -Value $approveResp

    [void](Wait-Until -TimeoutSeconds 30 -IntervalMilliseconds 400 -Check {
        try { return ((Get-IncidentDetail -IncidentId $incidentId).state -eq 'approved') } catch { return $false }
    })

    # 3) Simulate the agent fetching its command: GET /devices/{id}/commands dispatches the
    #    queued command (incident -> executing) and returns a fresh requestId.
    $cmdResp = Invoke-RestMethod -Method Get -Uri "$BackendUrl/devices/$DeviceId/commands" -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $ScenarioDir 'commands_response.json') -Value $cmdResp
    $command = @($cmdResp.commands) | Select-Object -First 1
    if (-not $command -or [string]::IsNullOrWhiteSpace([string]$command.requestId)) {
        throw "[$Tag] no dispatched command/requestId returned for device $DeviceId."
    }
    $requestId = $command.requestId

    $script:p10_executing = $null
    [void](Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 300 -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $incidentId
            if ($d.state -eq 'executing') { $script:p10_executing = $d; return $true }
            return $false
        } catch { return $false }
    })
    if ($script:p10_executing) { Write-JsonFile -Path (Join-Path $ScenarioDir 'incident_executing_snapshot.json') -Value $script:p10_executing }

    # 4) Simulate the agent reporting a SUCCESSFUL execution result -> incident -> validating.
    #    Phase 10 deliberately does NOT resolve on command success alone; validation follows.
    $now = Get-Date
    $resultBody = [ordered]@{
        incidentId = $incidentId
        deviceId = $DeviceId
        requestId = $requestId
        status = 'succeeded'
        startedAt = $now.AddSeconds(-2).ToString('o')
        finishedAt = $now.ToString('o')
        results = @(
            [ordered]@{
                actionId = $selectedActionIds[0]
                target = $ServiceName
                status = 'succeeded'
                stdout = "phase10 smoke: $($selectedActionIds[0]) reported success"
                stderr = ''
                exitCode = 0
                durationMs = 1200
            }
        )
    }
    $resultResp = Invoke-RestMethod -Method Post -Uri "$BackendUrl/remediation/results" -ContentType 'application/json' -Body ($resultBody | ConvertTo-Json -Depth 8) -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $ScenarioDir 'incident_validating_snapshot.json') -Value $resultResp

    if ($resultResp.state -ne 'validating') {
        throw "[$Tag] expected incident to enter validating after a successful result, got '$($resultResp.state)'."
    }
    Write-Host "[phase10-smoke][$Tag] Incident $incidentId is validating (recovery proof pending)."
    return @{ incidentId = $incidentId; requestId = $requestId }
}

Write-Host "[phase10-smoke] Artifacts directory: $outDir"

try {
    if (@(Get-NetTCPConnection -LocalPort $backendPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw "Port $backendPort is already in use. Stop the existing backend before running this smoke check."
    }

    $serverExe = Join-Path $outDir 'pulseops-server.exe'
    Write-Host '[phase10-smoke] Building backend...'
    Push-Location $backendDir
    try {
        & go build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "backend go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Set-AgentBuilderEnvDefaults

    Write-Host '[phase10-smoke] Starting backend...'
    $backendProc = Start-Process $serverExe -WorkingDirectory $backendDir -RedirectStandardOutput $backendOut -RedirectStandardError $backendErr -PassThru
    $procs += $backendProc

    if (-not $SkipFrontend) {
        Write-Host '[phase10-smoke] Starting frontend...'
        try {
            $frontendProc = Start-Process npm.cmd -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendOut -RedirectStandardError $frontendErr -PassThru
            $procs += $frontendProc
        } catch {
            Write-Warning "[phase10-smoke] Frontend failed to start (non-fatal): $($_.Exception.Message)"
        }
    }

    Write-Host '[phase10-smoke] Waiting for backend health...'
    $healthy = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds 1000 -Check {
        try { return ((Invoke-RestMethod -Method Get -Uri $healthUrl -TimeoutSec 2).status -eq 'ok') } catch { return $false }
    }
    if (-not $healthy) { throw "Backend did not become healthy within timeout ($TimeoutSeconds s)." }

    # ---------------------------------------------------------------------------------
    # POSITIVE PATH: healthy telemetry after remediation -> incident resolves.
    # ---------------------------------------------------------------------------------
    $greenDevice = "phase10-smoke-green-$timestamp"
    Write-Host "[phase10-smoke][green] Driving recovery-proven scenario (device $greenDevice)..."
    $green = Invoke-DriveToValidating -DeviceId $greenDevice -ScenarioDir $greenDir -Tag 'green'

    # Two fresh HEALTHY telemetry cycles (service running). Each is post-remediation evidence
    # newer than the validation boundary; two consecutive healthy cycles satisfy the default
    # requiredHealthyCycles and resolve the incident.
    1..2 | ForEach-Object {
        $r = Send-Telemetry -DeviceId $greenDevice -Status 'running' -Heartbeat $true -RequestId "green-healthy-$_"
        Write-JsonFile -Path (Join-Path $greenDir "telemetry_healthy_$_.json") -Value $r
        Start-Sleep -Milliseconds 800
    }

    Write-Host '[phase10-smoke][green] Waiting for incident to resolve...'
    $greenFinal = $null
    $greenResolved = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $green.incidentId
            $script:greenFinal = $d
            return ($d.state -eq 'resolved')
        } catch { return $false }
    }
    if ($greenFinal) { Write-JsonFile -Path (Join-Path $greenDir 'incident_resolved_snapshot.json') -Value $greenFinal }
    if (-not $greenResolved) { Write-Warning '[phase10-smoke][green] Incident did not resolve within timeout.' }

    # ---------------------------------------------------------------------------------
    # NEGATIVE PATH: health does not return -> incident fails validation by timeout.
    # ---------------------------------------------------------------------------------
    $redDevice = "phase10-smoke-red-$timestamp"
    Write-Host "[phase10-smoke][red] Driving recovery-not-proven scenario (device $redDevice)..."
    $red = Invoke-DriveToValidating -DeviceId $redDevice -ScenarioDir $redDir -Tag 'red'

    # Inject a couple of UNHEALTHY cycles (service still stopped) to actively show that
    # health did not return, then let the validation window elapse to fail the incident.
    1..2 | ForEach-Object {
        $r = Send-Telemetry -DeviceId $redDevice -Status 'stopped' -Heartbeat $true -RequestId "red-unhealthy-$_"
        Write-JsonFile -Path (Join-Path $redDir "telemetry_unhealthy_$_.json") -Value $r
        Start-Sleep -Milliseconds 800
    }

    Write-Host "[phase10-smoke][red] Waiting for validation timeout to fail the incident (up to $ValidationWaitSeconds s)..."
    $redFinal = $null
    $redFailed = Wait-Until -TimeoutSeconds $ValidationWaitSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $red.incidentId
            $script:redFinal = $d
            return ($d.state -eq 'failed')
        } catch { return $false }
    }
    if ($redFinal) { Write-JsonFile -Path (Join-Path $redDir 'incident_failed_snapshot.json') -Value $redFinal }
    if (-not $redFailed) { Write-Warning '[phase10-smoke][red] Incident did not fail within timeout.' }

    # ---------------------------------------------------------------------------------
    # Checks + summary.
    # ---------------------------------------------------------------------------------
    $greenChecks = [ordered]@{
        reachedValidating = $true
        resolved = ($greenFinal.state -eq 'resolved')
        validationSucceeded = ($greenFinal.validationStatus -eq 'succeeded')
        validatedAtRecorded = -not [string]::IsNullOrWhiteSpace([string]$greenFinal.validatedAt)
        healthyCyclesMet = ([int]$greenFinal.healthyCycleCount -ge [int]$greenFinal.requiredHealthyCycles -and [int]$greenFinal.requiredHealthyCycles -gt 0)
        deactivated = (-not $greenFinal.active)
    }
    $greenPass = $greenChecks.resolved -and $greenChecks.validationSucceeded -and $greenChecks.validatedAtRecorded -and $greenChecks.deactivated

    $redChecks = [ordered]@{
        reachedValidating = $true
        failed = ($redFinal.state -eq 'failed')
        validationFailed = ($redFinal.validationStatus -eq 'failed')
        failureReasonRecorded = -not [string]::IsNullOrWhiteSpace([string]$redFinal.validationFailureReason)
        notResolved = ($redFinal.state -ne 'resolved')
        deactivated = (-not $redFinal.active)
    }
    $redPass = $redChecks.failed -and $redChecks.validationFailed -and $redChecks.failureReasonRecorded

    $overallPass = $greenPass -and $redPass

    $summary = [ordered]@{
        runId = $timestamp
        backendUrl = $BackendUrl
        serviceName = $ServiceName
        positive = [ordered]@{
            deviceId = $greenDevice
            incidentId = $green.incidentId
            finalState = $greenFinal.state
            validationStatus = $greenFinal.validationStatus
            healthyCycleCount = $greenFinal.healthyCycleCount
            requiredHealthyCycles = $greenFinal.requiredHealthyCycles
            validatedAt = $greenFinal.validatedAt
            checks = $greenChecks
            pass = $greenPass
        }
        negative = [ordered]@{
            deviceId = $redDevice
            incidentId = $red.incidentId
            finalState = $redFinal.state
            validationStatus = $redFinal.validationStatus
            validationFailureReason = $redFinal.validationFailureReason
            checks = $redChecks
            pass = $redPass
        }
        overallPass = $overallPass
        note = 'green/incident_resolved_snapshot.json and red/incident_failed_snapshot.json hold the dashboard-visible validation evidence (validationStatus, healthy cycle counts, snapshot, and failure reason). Optionally capture dashboard screenshots as green/dashboard.png and red/dashboard.png.'
    }
    Write-JsonFile -Path (Join-Path $outDir 'summary.json') -Value $summary

    if ($overallPass) {
        Write-Host "[phase10-smoke] PASS: recovery proven (resolved) AND non-recovery fails validation (failed). Observable transitions captured."
    } else {
        Write-Warning "[phase10-smoke] Incomplete proof. green.pass=$greenPass red.pass=$redPass. Check summary.json and the scenario snapshots."
    }

    Write-Host "[phase10-smoke] Evidence captured at: $outDir"
}
finally {
    if (-not $KeepProcesses) {
        Write-Host '[phase10-smoke] Stopping started processes...'
        foreach ($p in $procs) {
            try {
                if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
            } catch { }
        }
        if ($serverExe -and (Test-Path $serverExe)) {
            Remove-Item -Path $serverExe -Force -ErrorAction SilentlyContinue
        }
    } else {
        Write-Host '[phase10-smoke] KeepProcesses specified; leaving services running.'
    }
}
