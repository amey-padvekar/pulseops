param(
    [switch]$SkipBuild,
    [ValidateSet('phase3', 'phase4')]
    [string]$Phase = 'phase3',
    [switch]$NoFrontend
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'

$healthzUrl = 'http://localhost:8080/healthz'
$devicesUrl = 'http://localhost:8080/devices'
$telemetryUrl = 'http://localhost:8080/telemetry'
$incidentsUrl = 'http://localhost:8080/incidents'
$frontendUrl = 'http://localhost:5173'
$expectedDeviceId = if ($env:AGENT_DEVICE_ID) { $env:AGENT_DEVICE_ID } else { 'DEV-AGENT-01' }
$monitoredServiceName = if ($env:MONITORED_SERVICE_NAME) { $env:MONITORED_SERVICE_NAME } else { 'OpenVPNService' }

$evidenceRootDir = Join-Path $repoRoot "artifacts\$Phase-smoke"
$runId = Get-Date -Format 'yyyyMMdd-HHmmss'
$evidenceDir = Join-Path $evidenceRootDir $runId
$backendLog = Join-Path $evidenceDir 'backend.log'
$backendErrLog = Join-Path $evidenceDir 'backend.err.log'
$agentLog = Join-Path $evidenceDir 'agent.log'
$agentErrLog = Join-Path $evidenceDir 'agent.err.log'
$frontendLog = Join-Path $evidenceDir 'frontend.log'
$frontendErrLog = Join-Path $evidenceDir 'frontend.err.log'

New-Item -ItemType Directory -Path $evidenceDir -Force | Out-Null

function Wait-Until {
    param(
        [scriptblock]$Check,
        [int]$TimeoutSeconds = 20,
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

function Write-JsonEvidence {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Path,
        [Parameter(Mandatory = $true)]
        [object]$Value
    )

    $json = $Value | ConvertTo-Json -Depth 10
    [System.IO.File]::WriteAllText($Path, $json)
}

function New-FailureTelemetry {
    param(
        [Parameter(Mandatory = $true)]
        [string]$DeviceId,
        [Parameter(Mandatory = $true)]
        [string]$ServiceName,
        [Parameter(Mandatory = $true)]
        [datetime]$TimestampUtc,
        [Parameter(Mandatory = $true)]
        [string[]]$RecentLogs
    )

    return @{
        schemaVersion = '1.0.0'
        deviceId = $DeviceId
        timestamp = $TimestampUtc.ToString('o')
        heartbeat = $true
        serviceName = $ServiceName
        serviceStatus = 'stopped'
        networkReachable = $true
        cpuUsage = 17.4
        memoryUsage = 62.1
        recentLogs = $RecentLogs
    }
}

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

Write-Host "[smoke-check] Starting backend and agent for $Phase verification..."
$backendProc = Start-Process go -WorkingDirectory $backendDir -ArgumentList 'run', './cmd/server' -RedirectStandardOutput $backendLog -RedirectStandardError $backendErrLog -PassThru
$agentProc = Start-Process go -WorkingDirectory $agentDir -ArgumentList 'run', './cmd/agent' -RedirectStandardOutput $agentLog -RedirectStandardError $agentErrLog -PassThru
$frontendProc = $null

if ($Phase -eq 'phase4' -and -not $NoFrontend) {
    Write-Host '[smoke-check] Starting frontend for Phase 4 panel verification...'
    $frontendProc = Start-Process npm -WorkingDirectory $frontendDir -ArgumentList 'run', 'dev', '--', '--host', '127.0.0.1', '--port', '5173' -RedirectStandardOutput $frontendLog -RedirectStandardError $frontendErrLog -PassThru
}

try {
    $healthReady = Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 500 -Check {
        try {
            $resp = Invoke-RestMethod -Uri $healthzUrl -Method Get -TimeoutSec 2
            return $resp.status -eq 'ok'
        } catch {
            return $false
        }
    }

    if (-not $healthReady) {
        throw 'Backend /healthz did not return status=ok within timeout.'
    }

    if ($Phase -eq 'phase4' -and $frontendProc) {
        $frontendReady = Wait-Until -TimeoutSeconds 25 -IntervalMilliseconds 500 -Check {
            try {
                $resp = Invoke-WebRequest -Uri $frontendUrl -Method Get -UseBasicParsing -TimeoutSec 2
                return $resp.StatusCode -ge 200 -and $resp.StatusCode -lt 500
            } catch {
                return $false
            }
        }

        Write-JsonEvidence -Path (Join-Path $evidenceDir 'frontend_status.json') -Value @{
            started = $true
            reachable = $frontendReady
            url = $frontendUrl
        }
    }

    if ($Phase -eq 'phase3') {
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
        return
    }

    # Phase 4 deterministic smoke path.
    $baselineIncidents = Invoke-RestMethod -Uri "${incidentsUrl}?active=true" -Method Get -TimeoutSec 2
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'baseline_incidents.json') -Value $baselineIncidents
    if ($baselineIncidents -is [System.Array] -and $baselineIncidents.Count -gt 0) {
        throw 'Expected no active incidents at baseline for Phase 4 smoke check.'
    }

    $firstFailureTimestamp = (Get-Date).ToUniversalTime()
    $firstTelemetry = New-FailureTelemetry -DeviceId $expectedDeviceId -ServiceName $monitoredServiceName -TimestampUtc $firstFailureTimestamp -RecentLogs @('phase4 smoke: service stopped while heartbeat true')
    $firstRequestId = "phase4-$runId-1"
    $firstTelemetryResponse = Invoke-RestMethod -Uri $telemetryUrl -Method Post -Headers @{
        'X-PulseOps-Request-ID' = $firstRequestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $expectedDeviceId
    } -ContentType 'application/json' -Body ($firstTelemetry | ConvertTo-Json -Depth 5) -TimeoutSec 3
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'telemetry_post_1_response.json') -Value $firstTelemetryResponse

    $incidentReady = Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 500 -Check {
        try {
            $active = Invoke-RestMethod -Uri "${incidentsUrl}?active=true&deviceId=$([uri]::EscapeDataString($expectedDeviceId))" -Method Get -TimeoutSec 2
            if (-not ($active -is [System.Array]) -or $active.Count -lt 1) {
                return $false
            }
            return $active[0].state -eq 'investigating'
        } catch {
            return $false
        }
    }
    if (-not $incidentReady) {
        throw 'Active incident did not reach investigating state within timeout.'
    }

    $activeAfterFirst = Invoke-RestMethod -Uri "${incidentsUrl}?active=true&deviceId=$([uri]::EscapeDataString($expectedDeviceId))" -Method Get -TimeoutSec 2
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'incidents_after_first_failure.json') -Value $activeAfterFirst
    if ($activeAfterFirst.Count -ne 1) {
        throw "Expected exactly 1 active incident after first failure; found $($activeAfterFirst.Count)."
    }

    $firstIncidentId = $activeAfterFirst[0].incidentId
    $firstLastSeenAt = Get-Date $activeAfterFirst[0].lastSeenAt

    $secondTelemetry = New-FailureTelemetry -DeviceId $expectedDeviceId -ServiceName $monitoredServiceName -TimestampUtc ($firstFailureTimestamp.AddSeconds(30)) -RecentLogs @('phase4 smoke: repeated failing heartbeat, no duplicate expected')
    $secondRequestId = "phase4-$runId-2"
    $secondTelemetryResponse = Invoke-RestMethod -Uri $telemetryUrl -Method Post -Headers @{
        'X-PulseOps-Request-ID' = $secondRequestId
        'X-PulseOps-Request-Attempt' = '1'
        'X-PulseOps-Device-ID' = $expectedDeviceId
    } -ContentType 'application/json' -Body ($secondTelemetry | ConvertTo-Json -Depth 5) -TimeoutSec 3
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'telemetry_post_2_response.json') -Value $secondTelemetryResponse

    $dedupeVerified = Wait-Until -TimeoutSeconds 20 -IntervalMilliseconds 500 -Check {
        try {
            $active = Invoke-RestMethod -Uri "${incidentsUrl}?active=true&deviceId=$([uri]::EscapeDataString($expectedDeviceId))" -Method Get -TimeoutSec 2
            if (-not ($active -is [System.Array]) -or $active.Count -ne 1) {
                return $false
            }
            if ($active[0].incidentId -ne $firstIncidentId) {
                return $false
            }
            $nextLastSeenAt = Get-Date $active[0].lastSeenAt
            return $nextLastSeenAt -gt $firstLastSeenAt
        } catch {
            return $false
        }
    }
    if (-not $dedupeVerified) {
        throw 'Repeated failing heartbeat did not preserve single active incident with refreshed lastSeenAt.'
    }

    $activeAfterSecond = Invoke-RestMethod -Uri "${incidentsUrl}?active=true&deviceId=$([uri]::EscapeDataString($expectedDeviceId))" -Method Get -TimeoutSec 2
    $detailAfterSecond = Invoke-RestMethod -Uri "${incidentsUrl}/$firstIncidentId" -Method Get -TimeoutSec 2
    $allIncidents = Invoke-RestMethod -Uri $incidentsUrl -Method Get -TimeoutSec 2

    Write-JsonEvidence -Path (Join-Path $evidenceDir 'incidents_after_second_failure.json') -Value $activeAfterSecond
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'incident_detail.json') -Value $detailAfterSecond
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'incidents_snapshot.json') -Value $allIncidents

    $summary = @{
        phase = $Phase
        runId = $runId
        deviceId = $expectedDeviceId
        serviceName = $monitoredServiceName
        firstRequestId = $firstRequestId
        secondRequestId = $secondRequestId
        activeIncidentId = $firstIncidentId
        activeIncidentCountAfterSecondFailure = $activeAfterSecond.Count
        activeIncidentState = $activeAfterSecond[0].state
        activeIncidentSeverity = $activeAfterSecond[0].severity
        frontendStarted = [bool]$frontendProc
        evidenceDir = $evidenceDir
    }
    Write-JsonEvidence -Path (Join-Path $evidenceDir 'summary.json') -Value $summary

    Write-Host '[smoke-check] PASS: Phase 4 incident smoke check succeeded.'
    Write-Host "[smoke-check] Active incident id: $firstIncidentId"
    Write-Host "[smoke-check] State: $($activeAfterSecond[0].state) | Severity: $($activeAfterSecond[0].severity)"
    Write-Host "[smoke-check] Evidence written to: $evidenceDir"
    Write-Host ''
    Write-Host '[smoke-check] Optional UI evidence:'
    Write-Host '  1) Open http://localhost:5173'
    Write-Host '  2) Verify Incident Timeline card is red with active incident details'
    Write-Host '  3) Save screenshot to artifacts\phase4-smoke\<timestamp>\dashboard.png'
} finally {
    if ($frontendProc -and -not $frontendProc.HasExited) {
        Stop-Process -Id $frontendProc.Id -Force
    }
    if ($agentProc -and -not $agentProc.HasExited) {
        Stop-Process -Id $agentProc.Id -Force
    }
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force
    }
}
