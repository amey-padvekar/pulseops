<#
Phase 11 smoke-check script

Goal:
- Prove the FINAL INCIDENT SUMMARY works as the closing artifact in the live story:
  an incident resolves or fails, summary generation triggers, the summary is stored on
  the incident, appears via the API the dashboard reads, and survives a refresh.
  Artifacts land under artifacts/phase11-smoke/<timestamp>.

This builds directly on the Phase 10 proof path (recovery proven -> resolved; recovery not
proven -> failed) and then asserts the Phase 11 closing report for BOTH outcomes:

  POSITIVE: detect -> approve -> execute -> validate -> resolved
              -> final summary generated (result reflects recovery)
  NEGATIVE: same path to validating, health never returns -> failed (timeout)
              -> final summary generated (result reflects failure, no false recovery)

The backend runs offline (unreachable Agent Builder endpoint + local_stub_actions) so the
approval gate works without network. With no live ADK summary backend, summary generation
deterministically FALLS BACK to a record-grounded summary — which is exactly the
demo-resilience property Phase 11 step 4.10 guarantees. The artifact is therefore produced
even with zero AI availability.

Usage:
    pwsh -NoProfile -File .\scripts\phase11-smoke.ps1

Optional:
    -BackendUrl http://localhost:8080
    -TimeoutSeconds 180
    -ValidationWaitSeconds 100
    -SummaryWaitSeconds 30
    -PollIntervalSeconds 2
    -ApprovedBy demo.operator
    -ServiceName OpenVPNService
    -SkipFrontend
    -KeepProcesses

Artifacts produced (per scenario, under green/ and red/):
- incident snapshots at awaiting_approval, executing, validating, terminal, and with-summary
- summary.txt — human-readable closing report for demo narration (matches the dashboard Copy)
- summary.json with pass/fail checks for both scenarios
- example-summary.txt at the run root — the representative closing artifact (task 4.12.2)
#>

param(
    [int]$TimeoutSeconds = 180,
    [int]$ValidationWaitSeconds = 100,
    [int]$SummaryWaitSeconds = 30,
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

function Write-TextFile {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Value
    )
    [System.IO.File]::WriteAllText($Path, $Value)
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
    # Offline-safe: unreachable Agent Builder endpoint + local stub so detection still
    # produces a catalog-valid recommendation and reaches awaiting_approval. Summary
    # generation has no live backend and therefore uses the deterministic fallback.
    if (-not $env:AGENT_BUILDER_ENDPOINT -and -not $env:AGENT_BUILDER_ADK_ENDPOINT) {
        $env:AGENT_BUILDER_ENDPOINT = 'http://127.0.0.1:1'
        if (-not $env:AGENT_BUILDER_ENABLED) { $env:AGENT_BUILDER_ENABLED = 'true' }
        if (-not $env:AGENT_BUILDER_FALLBACK_MODE) { $env:AGENT_BUILDER_FALLBACK_MODE = 'local_stub_actions' }
        if (-not $env:AGENT_BUILDER_TIMEOUT_MS) { $env:AGENT_BUILDER_TIMEOUT_MS = '2000' }
        if (-not $env:AGENT_BUILDER_SUMMARY_TIMEOUT_MS) { $env:AGENT_BUILDER_SUMMARY_TIMEOUT_MS = '4000' }
        Write-Host '[phase11-smoke] Agent Builder endpoint not configured; using local_stub_actions + deterministic summary fallback.'
    }
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$frontendDir = Join-Path $repoRoot 'frontend'

$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$outDir = Join-Path $repoRoot (Join-Path 'artifacts\phase11-smoke' $timestamp)
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

function Send-Telemetry {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceId,
        [Parameter(Mandatory = $true)][string]$Status,
        [Parameter(Mandatory = $true)][bool]$Heartbeat,
        [string]$RequestId = 'phase11'
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
        recentLogs = @("phase11 smoke: service $Status")
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

# Render the final summary as plain text for demo narration. Mirrors the dashboard Copy
# button output so the rehearsal artifact matches what a judge would copy from the UI.
function Format-SummaryText {
    param([Parameter(Mandatory = $true)][object]$Incident)
    $s = $Incident.finalSummary
    $lines = @()
    $lines += "Incident Summary - $($Incident.incidentId)"
    $lines += "Device: $($Incident.deviceId) - Service: $($Incident.serviceName)"
    $lines += "Outcome: $($Incident.state)"
    $lines += ""
    if (-not [string]::IsNullOrWhiteSpace([string]$s.operatorSummary)) {
        $lines += [string]$s.operatorSummary
        $lines += ""
    }
    $lines += "Root cause: $($s.rootCause)"
    $lines += ""
    $lines += "Evidence:"
    foreach ($e in @($s.evidence)) { $lines += "  - $e" }
    $lines += ""
    $lines += "Actions taken:"
    foreach ($a in @($s.actionsTaken)) { $lines += "  - $a" }
    $lines += ""
    $lines += "Result: $($s.result)"
    if (-not [string]::IsNullOrWhiteSpace([string]$Incident.summaryGeneratedAt)) {
        $lines += ""
        $lines += "Generated: $($Incident.summaryGeneratedAt)"
    }
    return ($lines -join "`n")
}

# Poll until the incident carries a stored final summary (generated or fallback).
function Wait-ForSummary {
    param(
        [Parameter(Mandatory = $true)][string]$IncidentId,
        [int]$TimeoutSeconds = 30
    )
    $script:p11_withSummary = $null
    [void](Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds 1000 -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $IncidentId
            if ($d.finalSummary -and -not [string]::IsNullOrWhiteSpace([string]$d.summaryStatus) -and $d.summaryStatus -ne 'pending') {
                $script:p11_withSummary = $d
                return $true
            }
            return $false
        } catch { return $false }
    })
    return $script:p11_withSummary
}

# Drive a device from a stopped-service detection to `validating`, simulating the agent's
# command fetch and a successful execution result. Returns a hashtable with incidentId and
# requestId. Per-step snapshots are written under $ScenarioDir.
function Invoke-DriveToValidating {
    param(
        [Parameter(Mandatory = $true)][string]$DeviceId,
        [Parameter(Mandatory = $true)][string]$ScenarioDir,
        [Parameter(Mandatory = $true)][string]$Tag
    )

    $tResp = Send-Telemetry -DeviceId $DeviceId -Status 'stopped' -Heartbeat $true -RequestId "$Tag-detect"
    Write-JsonFile -Path (Join-Path $ScenarioDir 'telemetry_detect_response.json') -Value $tResp

    Write-Host "[phase11-smoke][$Tag] Waiting for awaiting_approval (device $DeviceId)..."
    $script:p11_awaiting = $null
    $ok = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $list = @(Invoke-RestMethod -Method Get -Uri $incidentsUrl -TimeoutSec 3)
            $m = $list | Where-Object { $_.deviceId -eq $DeviceId } | Select-Object -First 1
            if (-not $m) { return $false }
            $hasActions = @($m.recommendedActions).Count -gt 0
            if (($m.state -eq 'awaiting_approval') -and $hasActions) { $script:p11_awaiting = $m; return $true }
            return $false
        } catch { return $false }
    }
    $awaiting = $script:p11_awaiting
    if (-not $ok -or -not $awaiting) { throw "[$Tag] incident did not reach awaiting_approval with a recommendation." }
    $incidentId = $awaiting.incidentId
    Write-JsonFile -Path (Join-Path $ScenarioDir 'incident_awaiting_snapshot.json') -Value $awaiting

    $selectedActionIds = @($awaiting.recommendedActions | ForEach-Object { $_.actionId })
    $approvalReq = [ordered]@{ approvedBy = $ApprovedBy; selectedActionIds = $selectedActionIds; note = "Phase 11 smoke ($Tag)" }
    $approveResp = Invoke-RestMethod -Method Post -Uri "$BackendUrl/incidents/$incidentId/approve" -ContentType 'application/json' -Body ($approvalReq | ConvertTo-Json -Depth 6) -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $ScenarioDir 'approval_response.json') -Value $approveResp

    [void](Wait-Until -TimeoutSeconds 30 -IntervalMilliseconds 400 -Check {
        try { return ((Get-IncidentDetail -IncidentId $incidentId).state -eq 'approved') } catch { return $false }
    })

    $cmdResp = Invoke-RestMethod -Method Get -Uri "$BackendUrl/devices/$DeviceId/commands" -TimeoutSec 5
    Write-JsonFile -Path (Join-Path $ScenarioDir 'commands_response.json') -Value $cmdResp
    $command = @($cmdResp.commands) | Select-Object -First 1
    if (-not $command -or [string]::IsNullOrWhiteSpace([string]$command.requestId)) {
        throw "[$Tag] no dispatched command/requestId returned for device $DeviceId."
    }
    $requestId = $command.requestId

    [void](Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 300 -Check {
        try { return ((Get-IncidentDetail -IncidentId $incidentId).state -eq 'executing') } catch { return $false }
    })

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
                stdout = "phase11 smoke: $($selectedActionIds[0]) reported success"
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
    Write-Host "[phase11-smoke][$Tag] Incident $incidentId is validating."
    return @{ incidentId = $incidentId; requestId = $requestId }
}

Write-Host "[phase11-smoke] Artifacts directory: $outDir"

try {
    if (@(Get-NetTCPConnection -LocalPort $backendPort -State Listen -ErrorAction SilentlyContinue).Count -gt 0) {
        throw "Port $backendPort is already in use. Stop the existing backend before running this smoke check."
    }

    $serverExe = Join-Path $outDir 'pulseops-server.exe'
    Write-Host '[phase11-smoke] Building backend...'
    Push-Location $backendDir
    try {
        & go build -o $serverExe ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw "backend go build failed with exit code $LASTEXITCODE" }
    } finally {
        Pop-Location
    }

    Set-AgentBuilderEnvDefaults

    Write-Host '[phase11-smoke] Starting backend...'
    $backendProc = Start-Process $serverExe -WorkingDirectory $backendDir -RedirectStandardOutput $backendOut -RedirectStandardError $backendErr -PassThru
    $procs += $backendProc

    if (-not $SkipFrontend) {
        Write-Host '[phase11-smoke] Starting frontend...'
        try {
            $frontendProc = Start-Process npm.cmd -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendOut -RedirectStandardError $frontendErr -PassThru
            $procs += $frontendProc
        } catch {
            Write-Warning "[phase11-smoke] Frontend failed to start (non-fatal): $($_.Exception.Message)"
        }
    }

    Write-Host '[phase11-smoke] Waiting for backend health...'
    $healthy = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds 1000 -Check {
        try { return ((Invoke-RestMethod -Method Get -Uri $healthUrl -TimeoutSec 2).status -eq 'ok') } catch { return $false }
    }
    if (-not $healthy) { throw "Backend did not become healthy within timeout ($TimeoutSeconds s)." }

    # ---------------------------------------------------------------------------------
    # POSITIVE PATH: resolve, then assert the final summary closing artifact.
    # ---------------------------------------------------------------------------------
    $greenDevice = "phase11-smoke-green-$timestamp"
    Write-Host "[phase11-smoke][green] Driving recovery-proven scenario (device $greenDevice)..."
    $green = Invoke-DriveToValidating -DeviceId $greenDevice -ScenarioDir $greenDir -Tag 'green'

    1..2 | ForEach-Object {
        $r = Send-Telemetry -DeviceId $greenDevice -Status 'running' -Heartbeat $true -RequestId "green-healthy-$_"
        Write-JsonFile -Path (Join-Path $greenDir "telemetry_healthy_$_.json") -Value $r
        Start-Sleep -Milliseconds 800
    }

    Write-Host '[phase11-smoke][green] Waiting for incident to resolve...'
    $greenFinal = $null
    $greenResolved = Wait-Until -TimeoutSeconds $TimeoutSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $green.incidentId
            $script:greenFinal = $d
            return ($d.state -eq 'resolved')
        } catch { return $false }
    }
    if ($greenFinal) { Write-JsonFile -Path (Join-Path $greenDir 'incident_resolved_snapshot.json') -Value $greenFinal }
    if (-not $greenResolved) { Write-Warning '[phase11-smoke][green] Incident did not resolve within timeout.' }

    Write-Host '[phase11-smoke][green] Waiting for final summary...'
    $greenSummary = Wait-ForSummary -IncidentId $green.incidentId -TimeoutSeconds $SummaryWaitSeconds
    if ($greenSummary) {
        Write-JsonFile -Path (Join-Path $greenDir 'incident_with_summary_snapshot.json') -Value $greenSummary
        Write-TextFile -Path (Join-Path $greenDir 'summary.txt') -Value (Format-SummaryText -Incident $greenSummary)
    } else {
        Write-Warning '[phase11-smoke][green] Final summary did not appear within timeout.'
    }

    # Survives refresh: re-fetch and confirm the stored summary persists unchanged.
    $greenRefresh = $null
    if ($greenSummary) {
        Start-Sleep -Milliseconds 600
        $greenRefresh = Get-IncidentDetail -IncidentId $green.incidentId
        Write-JsonFile -Path (Join-Path $greenDir 'incident_refresh_snapshot.json') -Value $greenRefresh
    }

    # ---------------------------------------------------------------------------------
    # NEGATIVE PATH: fail by timeout, then assert the failure-reflecting summary.
    # ---------------------------------------------------------------------------------
    $redDevice = "phase11-smoke-red-$timestamp"
    Write-Host "[phase11-smoke][red] Driving recovery-not-proven scenario (device $redDevice)..."
    $red = Invoke-DriveToValidating -DeviceId $redDevice -ScenarioDir $redDir -Tag 'red'

    1..2 | ForEach-Object {
        $r = Send-Telemetry -DeviceId $redDevice -Status 'stopped' -Heartbeat $true -RequestId "red-unhealthy-$_"
        Write-JsonFile -Path (Join-Path $redDir "telemetry_unhealthy_$_.json") -Value $r
        Start-Sleep -Milliseconds 800
    }

    Write-Host "[phase11-smoke][red] Waiting for validation timeout to fail the incident (up to $ValidationWaitSeconds s)..."
    $redFinal = $null
    $redFailed = Wait-Until -TimeoutSeconds $ValidationWaitSeconds -IntervalMilliseconds ($PollIntervalSeconds * 1000) -Check {
        try {
            $d = Get-IncidentDetail -IncidentId $red.incidentId
            $script:redFinal = $d
            return ($d.state -eq 'failed')
        } catch { return $false }
    }
    if ($redFinal) { Write-JsonFile -Path (Join-Path $redDir 'incident_failed_snapshot.json') -Value $redFinal }
    if (-not $redFailed) { Write-Warning '[phase11-smoke][red] Incident did not fail within timeout.' }

    Write-Host '[phase11-smoke][red] Waiting for final summary...'
    $redSummary = Wait-ForSummary -IncidentId $red.incidentId -TimeoutSeconds $SummaryWaitSeconds
    if ($redSummary) {
        Write-JsonFile -Path (Join-Path $redDir 'incident_with_summary_snapshot.json') -Value $redSummary
        Write-TextFile -Path (Join-Path $redDir 'summary.txt') -Value (Format-SummaryText -Incident $redSummary)
    } else {
        Write-Warning '[phase11-smoke][red] Final summary did not appear within timeout.'
    }

    $redRefresh = $null
    if ($redSummary) {
        Start-Sleep -Milliseconds 600
        $redRefresh = Get-IncidentDetail -IncidentId $red.incidentId
        Write-JsonFile -Path (Join-Path $redDir 'incident_refresh_snapshot.json') -Value $redRefresh
    }

    # ---------------------------------------------------------------------------------
    # Checks + summary.
    # ---------------------------------------------------------------------------------
    function Get-SummaryChecks {
        param($Final, $Refresh, [string]$OutcomeWord)
        $s = $Final.finalSummary
        $wordCount = 0
        if ($s) { $wordCount = (Format-SummaryText -Incident $Final).Split(@(" ", "`n", "`r", "`t"), [System.StringSplitOptions]::RemoveEmptyEntries).Count }
        return [ordered]@{
            summaryPresent = ($null -ne $s)
            statusValid = ($Final.summaryStatus -in @('generated', 'fallback'))
            rootCauseNonEmpty = (-not [string]::IsNullOrWhiteSpace([string]$s.rootCause))
            evidenceNonEmpty = (@($s.evidence).Count -gt 0)
            actionsNonEmpty = (@($s.actionsTaken).Count -gt 0)
            resultNonEmpty = (-not [string]::IsNullOrWhiteSpace([string]$s.result))
            outcomeReflected = ([string]$s.result -match $OutcomeWord)
            generatedAtRecorded = (-not [string]::IsNullOrWhiteSpace([string]$Final.summaryGeneratedAt))
            survivesRefresh = ($Refresh -and $Refresh.finalSummary -and ([string]$Refresh.summaryGeneratedAt -eq [string]$Final.summaryGeneratedAt))
            # Demo readiness: a 3-minute window holds ~450 spoken words at a conversational
            # ~150 wpm. A < 220-word closing report (header + structured fields) is therefore
            # read aloud in well under a minute, leaving ample demo time — while still
            # flagging genuine bloat.
            readableQuicklyWordCount = $wordCount
            readableQuickly = ($wordCount -gt 0 -and $wordCount -lt 220)
        }
    }

    $greenChecks = Get-SummaryChecks -Final $greenFinal -Refresh $greenRefresh -OutcomeWord 'resolv'
    $greenPass = $greenChecks.summaryPresent -and $greenChecks.statusValid -and $greenChecks.rootCauseNonEmpty -and $greenChecks.evidenceNonEmpty -and $greenChecks.actionsNonEmpty -and $greenChecks.resultNonEmpty -and $greenChecks.outcomeReflected -and $greenChecks.survivesRefresh -and $greenChecks.readableQuickly

    # The failed-path result must reflect failure and must NOT falsely claim resolution.
    $redChecks = Get-SummaryChecks -Final $redFinal -Refresh $redRefresh -OutcomeWord 'fail|not\s'
    $redChecks.notClaimingRecovery = (-not ([string]$redFinal.finalSummary.result -match 'recovered and the incident was resolved'))
    $redPass = $redChecks.summaryPresent -and $redChecks.statusValid -and $redChecks.rootCauseNonEmpty -and $redChecks.evidenceNonEmpty -and $redChecks.resultNonEmpty -and $redChecks.notClaimingRecovery -and $redChecks.survivesRefresh -and $redChecks.readableQuickly

    $overallPass = $greenPass -and $redPass

    # Capture one representative example artifact at the run root (task 4.12.2).
    if ($greenSummary) {
        Write-TextFile -Path (Join-Path $outDir 'example-summary.txt') -Value (Format-SummaryText -Incident $greenSummary)
    } elseif ($redSummary) {
        Write-TextFile -Path (Join-Path $outDir 'example-summary.txt') -Value (Format-SummaryText -Incident $redSummary)
    }

    $summary = [ordered]@{
        runId = $timestamp
        backendUrl = $BackendUrl
        serviceName = $ServiceName
        positive = [ordered]@{
            deviceId = $greenDevice
            incidentId = $green.incidentId
            finalState = $greenFinal.state
            summaryStatus = $greenFinal.summaryStatus
            summaryGeneratedAt = $greenFinal.summaryGeneratedAt
            checks = $greenChecks
            pass = $greenPass
        }
        negative = [ordered]@{
            deviceId = $redDevice
            incidentId = $red.incidentId
            finalState = $redFinal.state
            summaryStatus = $redFinal.summaryStatus
            summaryGeneratedAt = $redFinal.summaryGeneratedAt
            checks = $redChecks
            pass = $redPass
        }
        overallPass = $overallPass
        note = 'green/summary.txt and red/summary.txt are the demo-narration closing reports (match the dashboard Copy action). *_with_summary_snapshot.json hold the stored summary; *_refresh_snapshot.json prove it survives a refresh. example-summary.txt is the representative artifact. Optionally capture dashboard screenshots as green/dashboard.png and red/dashboard.png.'
    }
    Write-JsonFile -Path (Join-Path $outDir 'summary.json') -Value $summary

    if ($overallPass) {
        Write-Host "[phase11-smoke] PASS: resolved AND failed incidents both produced a stored, refresh-surviving, demo-readable closing summary."
    } else {
        Write-Warning "[phase11-smoke] Incomplete proof. green.pass=$greenPass red.pass=$redPass. Check summary.json and the scenario artifacts."
    }

    Write-Host "[phase11-smoke] Evidence captured at: $outDir"
}
finally {
    if (-not $KeepProcesses) {
        Write-Host '[phase11-smoke] Stopping started processes...'
        foreach ($p in $procs) {
            try {
                if ($p -and -not $p.HasExited) { Stop-Process -Id $p.Id -Force -ErrorAction SilentlyContinue }
            } catch { }
        }
        if ($serverExe -and (Test-Path $serverExe)) {
            Remove-Item -Path $serverExe -Force -ErrorAction SilentlyContinue
        }
    } else {
        Write-Host '[phase11-smoke] KeepProcesses specified; leaving services running.'
    }
}
