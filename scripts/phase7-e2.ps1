<#
Phase E2 observability controlled test

Purpose:
- Prove the Phase E2 metric set and per-request structured log are wired
  correctly in the Cloud Run adapter.
- Drive deterministic traffic against a locally-run adapter and assert the
  Prometheus /metrics counters move as expected.
- Optionally deploy the Cloud Monitoring log-based metrics, alert policies,
  and dashboard.

Outputs:
- artifacts/phase7-smoke/<timestamp>-e2/
  - logs/*.log
  - metrics.txt
  - summary.json

Usage:
    pwsh -NoProfile -File .\scripts\phase7-e2.ps1

Options:
    -Port 8085             Local port for the adapter under test
    -DeployMonitoring      Apply gcloud log-based metrics, alerts, dashboard
    -Project pulseops-agent  GCP project for -DeployMonitoring
    -ChannelId <id>        Notification channel id for alert policies
#>

param(
    [int]$Port = 8085,
    [switch]$DeployMonitoring,
    [string]$Project = 'pulseops-agent',
    [string]$ChannelId
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

function Get-MetricValue {
    param(
        [Parameter(Mandatory = $true)][string]$Text,
        [Parameter(Mandatory = $true)][string]$Name
    )
    $pattern = '(?m)^' + [regex]::Escape($Name) + '\s+([0-9.eE+\-]+)\s*$'
    $m = [regex]::Match($Text, $pattern)
    if (-not $m.Success) {
        return $null
    }
    return [double]$m.Groups[1].Value
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$artifactRoot = Join-Path $repoRoot (Join-Path 'artifacts\phase7-smoke' ($timestamp + '-e2'))
$logsDir = Join-Path $artifactRoot 'logs'
New-Item -ItemType Directory -Path $logsDir -Force | Out-Null

$adapterDir = Join-Path $repoRoot 'google-agent-service\cloudrun-adapter'
$authToken = 'phase7-e2-token'
$baseUrl = "http://127.0.0.1:$Port"

$checks = @()
$adapterProcess = $null

Write-Host "[phase7-e2] Artifacts: $artifactRoot"

try {
    # 1. Unit proofs: metric math + per-request log fields.
    $unitLog = Join-Path $logsDir 'obs-unit.log'
    $unitErr = "$unitLog.err"
    $u = Start-Process -FilePath 'go' -ArgumentList @('test', './internal/obs', './internal/httpapi', '-count=1') `
        -WorkingDirectory $adapterDir -RedirectStandardOutput $unitLog -RedirectStandardError $unitErr `
        -PassThru -NoNewWindow -Wait
    if (Test-Path $unitErr) {
        Add-Content -Path $unitLog -Value (Get-Content $unitErr -Raw)
        Remove-Item $unitErr -Force -ErrorAction SilentlyContinue
    }
    $checks += [ordered]@{ name = 'obs-and-handler-unit-tests'; passed = ($u.ExitCode -eq 0); detail = "exit=$($u.ExitCode)" }

    # 2. Build adapter binary.
    $adapterExe = Join-Path $artifactRoot 'adapter-e2.exe'
    $buildLog = Join-Path $logsDir 'build.log'
    $buildErr = "$buildLog.err"
    $b = Start-Process -FilePath 'go' -ArgumentList @('build', '-o', $adapterExe, './cmd/server') `
        -WorkingDirectory $adapterDir -RedirectStandardOutput $buildLog -RedirectStandardError $buildErr `
        -PassThru -NoNewWindow -Wait
    if (Test-Path $buildErr) {
        Add-Content -Path $buildLog -Value (Get-Content $buildErr -Raw)
        Remove-Item $buildErr -Force -ErrorAction SilentlyContinue
    }
    $checks += [ordered]@{ name = 'adapter-build'; passed = ($b.ExitCode -eq 0); detail = "exit=$($b.ExitCode)" }
    if ($b.ExitCode -ne 0) {
        throw 'adapter build failed'
    }

    # 3. Start adapter with auth + JSON logs captured to artifact.
    $adapterLog = Join-Path $logsDir 'adapter.log'
    $env:INBOUND_AUTH_TOKEN = $authToken
    $env:ADDR = ":$Port"
    $env:ELASTIC_MCP_ENABLED = 'true'
    $adapterProcess = Start-Process -FilePath $adapterExe -WorkingDirectory $adapterDir `
        -RedirectStandardOutput $adapterLog -RedirectStandardError "$adapterLog.err" -PassThru -NoNewWindow

    # Wait for readiness.
    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $h = Invoke-WebRequest -Uri "$baseUrl/health" -UseBasicParsing -TimeoutSec 2
            if ($h.StatusCode -eq 200) { $ready = $true; break }
        } catch {
            Start-Sleep -Milliseconds 300
        }
    }
    $checks += [ordered]@{ name = 'adapter-ready'; passed = $ready; detail = "$baseUrl/health" }
    if (-not $ready) {
        throw 'adapter did not become ready'
    }

    $headers = @{ Authorization = "Bearer $authToken" }
    $validBody = '{"task":"phase7_investigation","prompt":"investigate","metadata":{"incident_id":"inc-e2","device_id":"dev-e2","request_id":"req-e2"},"elastic_context_hints":{"deviceId":"dev-e2","serviceName":"OpenVPNService"},"available_actions":[{"actionId":"restart_service","target":"service"}],"evidence_summary":"serviceStatus=stopped\nheartbeat=true"}'

    # 4. Drive deterministic traffic.
    $successCount = 5
    for ($i = 0; $i -lt $successCount; $i++) {
        Invoke-WebRequest -Uri "$baseUrl/investigate" -Method Post -Headers $headers -ContentType 'application/json' -Body $validBody -UseBasicParsing -TimeoutSec 5 | Out-Null
    }
    $validationFailCount = 2
    for ($i = 0; $i -lt $validationFailCount; $i++) {
        try {
            Invoke-WebRequest -Uri "$baseUrl/investigate" -Method Post -Headers $headers -ContentType 'application/json' -Body '{"task":' -UseBasicParsing -TimeoutSec 5 | Out-Null
        } catch {
            # 400 expected; ignored.
        }
    }
    # One unauthorized request (must NOT be counted as a request outcome).
    try {
        Invoke-WebRequest -Uri "$baseUrl/investigate" -Method Post -ContentType 'application/json' -Body $validBody -UseBasicParsing -TimeoutSec 5 | Out-Null
    } catch {
        # 401 expected; ignored.
    }

    # 5. Scrape metrics.
    $metricsText = (Invoke-WebRequest -Uri "$baseUrl/metrics" -UseBasicParsing -TimeoutSec 5).Content
    [System.IO.File]::WriteAllText((Join-Path $artifactRoot 'metrics.txt'), $metricsText)

    $requests = Get-MetricValue -Text $metricsText -Name 'investigate_requests_total'
    $success = Get-MetricValue -Text $metricsText -Name 'investigate_success_total'
    $fail = Get-MetricValue -Text $metricsText -Name 'investigate_fail_total'
    $validationFail = Get-MetricValue -Text $metricsText -Name 'investigate_validation_fail_total'

    $checks += [ordered]@{ name = 'requests_total'; passed = ($requests -eq ($successCount + $validationFailCount)); detail = "got=$requests want=$($successCount + $validationFailCount)" }
    $checks += [ordered]@{ name = 'success_total'; passed = ($success -eq $successCount); detail = "got=$success want=$successCount" }
    $checks += [ordered]@{ name = 'validation_fail_total'; passed = ($validationFail -eq $validationFailCount); detail = "got=$validationFail want=$validationFailCount" }
    $checks += [ordered]@{ name = 'fail_total'; passed = ($fail -eq $validationFailCount); detail = "got=$fail want=$validationFailCount" }

    # 6. Confirm structured log carries required fields.
    $logRaw = ''
    if (Test-Path $adapterLog) { $logRaw = Get-Content $adapterLog -Raw }
    $requiredFields = @('request_id', 'incident_id', 'device_id', 'trace_id', 'status_transport', 'status_workflow', 'latency_ms', 'confidence', 'action_ids', 'enrichment_used', 'evidence_lines')
    $logLine = ($logRaw -split "`n" | Where-Object { $_ -match 'investigate_request' } | Select-Object -First 1)
    $missing = @()
    foreach ($f in $requiredFields) {
        if (-not ($logLine -match ('"' + $f + '"'))) { $missing += $f }
    }
    $checks += [ordered]@{ name = 'structured-log-fields'; passed = ($missing.Count -eq 0); detail = "missing=$($missing -join ',')" }
}
finally {
    if ($adapterProcess -and -not $adapterProcess.HasExited) {
        Stop-Process -Id $adapterProcess.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:INBOUND_AUTH_TOKEN -ErrorAction SilentlyContinue
    Remove-Item Env:ADDR -ErrorAction SilentlyContinue
    Remove-Item Env:ELASTIC_MCP_ENABLED -ErrorAction SilentlyContinue
}

# 7. Optional monitoring deploy.
$deploy = [ordered]@{ attempted = [bool]$DeployMonitoring; steps = @() }
if ($DeployMonitoring) {
    $monDir = Join-Path $repoRoot 'google-agent-service\deploy\monitoring'
    Write-Host "[phase7-e2] Deploying monitoring resources to $Project"
    Write-Host "[phase7-e2] See $monDir\README.md for the log-based metric creation commands."
    $dashArgs = @('monitoring', 'dashboards', 'create', "--project=$Project", "--config-from-file=$(Join-Path $monDir 'dashboard.json')")
    $d = Start-Process -FilePath 'gcloud' -ArgumentList $dashArgs -NoNewWindow -PassThru -Wait
    $deploy.steps += [ordered]@{ name = 'dashboard'; exitCode = $d.ExitCode }
    if ($ChannelId) {
        foreach ($policy in @('alert-error-rate.json', 'alert-p95-latency.json')) {
            $pArgs = @('alpha', 'monitoring', 'policies', 'create', "--project=$Project", "--policy-from-file=$(Join-Path $monDir $policy)", "--notification-channels=$ChannelId")
            $p = Start-Process -FilePath 'gcloud' -ArgumentList $pArgs -NoNewWindow -PassThru -Wait
            $deploy.steps += [ordered]@{ name = $policy; exitCode = $p.ExitCode }
        }
    } else {
        Write-Host "[phase7-e2] -ChannelId not supplied; skipping alert policy creation."
    }
}

$allPassed = ($checks | Where-Object { -not $_.passed }).Count -eq 0
$summary = [ordered]@{
    runId = $timestamp
    startedAt = (Get-Date).ToString('o')
    artifactDir = $artifactRoot
    status = if ($allPassed) { 'passed' } else { 'failed' }
    checks = $checks
    monitoringDeploy = $deploy
}
Write-JsonFile -Path (Join-Path $artifactRoot 'summary.json') -Value $summary

Write-Host ""
foreach ($c in $checks) {
    $mark = if ($c.passed) { 'PASS' } else { 'FAIL' }
    Write-Host ("  [{0}] {1} ({2})" -f $mark, $c.name, $c.detail)
}
Write-Host ""
Write-Host "[phase7-e2] Status: $($summary.status)"
Write-Host "[phase7-e2] Summary: $(Join-Path $artifactRoot 'summary.json')"

if (-not $allPassed) {
    exit 1
}
