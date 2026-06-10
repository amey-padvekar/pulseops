<#
Phase E3 security validation and rollback drill

Purpose:
- Validate the adapter security posture locally (auth enforcement, production
  startup gate, secret redaction, no raw evidence/prompt dumps).
- Optionally verify least-privilege IAM on the runtime service account.
- Optionally execute the timed Cloud Run rollback drill with a contract smoke
  before and after (exit criteria: rollback < 5 minutes, no contract break).

Outputs:
- artifacts/phase7-smoke/<timestamp>-e3/
  - logs/*.log
  - summary.json

Usage:
    pwsh -NoProfile -File .\scripts\phase7-e3.ps1

Options:
    -Port 8088               Local port for the adapter under test
    -GcpChecks               Verify runtime SA least-privilege IAM
    -RollbackDrill           Execute the timed Cloud Run rollback drill
    -Project pulseops-agent  GCP project
    -Region us-central1      Cloud Run region
    -Service <name>          Cloud Run service name
    -ServiceAccount <email>  Runtime SA email (defaults to pulseops-agent-svc@<project>)
    -AuthToken <token>       Inbound bearer token for the rollback contract smoke
#>

param(
    [int]$Port = 8088,
    [switch]$GcpChecks,
    [switch]$RollbackDrill,
    [string]$Project = 'pulseops-agent',
    [string]$Region = 'us-central1',
    [string]$Service = 'pulseops-google-agent-adapter',
    [string]$ServiceAccount,
    [string]$AuthToken
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

$expectedRoles = @(
    'roles/aiplatform.user',
    'roles/secretmanager.secretAccessor',
    'roles/logging.logWriter',
    'roles/monitoring.metricWriter'
)

$repoRoot = Split-Path -Parent $PSScriptRoot
$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$artifactRoot = Join-Path $repoRoot (Join-Path 'artifacts\phase7-smoke' ($timestamp + '-e3'))
$logsDir = Join-Path $artifactRoot 'logs'
New-Item -ItemType Directory -Path $logsDir -Force | Out-Null

$adapterDir = Join-Path $repoRoot 'google-agent-service\cloudrun-adapter'
$localToken = 'phase7-e3-secret-token'
$baseUrl = "http://127.0.0.1:$Port"
$checks = @()
$adapterProcess = $null

Write-Host "[phase7-e3] Artifacts: $artifactRoot"

try {
    # 1. Security-focused unit proofs.
    $unitLog = Join-Path $logsDir 'security-unit.log'
    $unitErr = "$unitLog.err"
    $u = Start-Process -FilePath 'go' -ArgumentList @('test', './internal/obs', './internal/safety', './internal/httpapi', '-count=1') `
        -WorkingDirectory $adapterDir -RedirectStandardOutput $unitLog -RedirectStandardError $unitErr `
        -PassThru -NoNewWindow -Wait
    if (Test-Path $unitErr) {
        Add-Content -Path $unitLog -Value (Get-Content $unitErr -Raw)
        Remove-Item $unitErr -Force -ErrorAction SilentlyContinue
    }
    $checks += [ordered]@{ name = 'security-unit-tests'; passed = ($u.ExitCode -eq 0); detail = "exit=$($u.ExitCode)" }

    # 2. Build adapter binary.
    $adapterExe = Join-Path $artifactRoot 'adapter-e3.exe'
    $buildLog = Join-Path $logsDir 'build.log'
    $b = Start-Process -FilePath 'go' -ArgumentList @('build', '-o', $adapterExe, './cmd/server') `
        -WorkingDirectory $adapterDir -RedirectStandardOutput $buildLog -RedirectStandardError "$buildLog.err" `
        -PassThru -NoNewWindow -Wait
    $checks += [ordered]@{ name = 'adapter-build'; passed = ($b.ExitCode -eq 0); detail = "exit=$($b.ExitCode)" }
    if ($b.ExitCode -ne 0) { throw 'adapter build failed' }

    # 3. Production startup gate: no token in production must refuse to start.
    $gateLog = Join-Path $logsDir 'prod-gate.log'
    $env:APP_ENV = 'production'
    Remove-Item Env:INBOUND_AUTH_TOKEN -ErrorAction SilentlyContinue
    $env:ADDR = ":$([int]$Port + 1)"
    $gate = Start-Process -FilePath $adapterExe -WorkingDirectory $adapterDir `
        -RedirectStandardOutput $gateLog -RedirectStandardError "$gateLog.err" -PassThru -NoNewWindow
    $exitedFast = $gate.WaitForExit(8000)
    if (-not $exitedFast) {
        Stop-Process -Id $gate.Id -Force -ErrorAction SilentlyContinue
    }
    $gateRefused = $exitedFast -and ($gate.ExitCode -ne 0)
    $checks += [ordered]@{ name = 'prod-startup-gate-refuses-no-auth'; passed = $gateRefused; detail = "exited=$exitedFast exit=$($gate.ExitCode)" }
    Remove-Item Env:APP_ENV -ErrorAction SilentlyContinue

    # 4. Start adapter with auth for live security checks.
    $adapterLog = Join-Path $logsDir 'adapter.log'
    $env:INBOUND_AUTH_TOKEN = $localToken
    $env:ADDR = ":$Port"
    $env:ELASTIC_MCP_ENABLED = 'true'
    $adapterProcess = Start-Process -FilePath $adapterExe -WorkingDirectory $adapterDir `
        -RedirectStandardOutput $adapterLog -RedirectStandardError "$adapterLog.err" -PassThru -NoNewWindow

    $ready = $false
    for ($i = 0; $i -lt 30; $i++) {
        try {
            $h = Invoke-WebRequest -Uri "$baseUrl/health" -UseBasicParsing -TimeoutSec 2
            if ($h.StatusCode -eq 200) { $ready = $true; break }
        } catch { Start-Sleep -Milliseconds 300 }
    }
    $checks += [ordered]@{ name = 'adapter-ready'; passed = $ready; detail = "$baseUrl/health" }
    if (-not $ready) { throw 'adapter did not become ready' }

    $validBody = '{"task":"phase7_investigation","prompt":"investigate","metadata":{"incident_id":"inc-e3","device_id":"dev-e3","request_id":"req-e3"},"available_actions":[{"actionId":"restart_service","target":"service"}],"evidence_summary":"SENSITIVE_EVIDENCE_E3"}'

    # 4a. Auth enforced: no token -> 401.
    $unauthorizedStatus = 0
    try {
        Invoke-WebRequest -Uri "$baseUrl/investigate" -Method Post -ContentType 'application/json' -Body $validBody -UseBasicParsing -TimeoutSec 5 | Out-Null
    } catch {
        $unauthorizedStatus = [int]$_.Exception.Response.StatusCode.value__
    }
    $checks += [ordered]@{ name = 'auth-enforced-401'; passed = ($unauthorizedStatus -eq 401); detail = "status=$unauthorizedStatus" }

    # 4b. Valid token -> 200, with an Authorization header carrying the secret.
    $authedStatus = 0
    try {
        $resp = Invoke-WebRequest -Uri "$baseUrl/investigate" -Method Post -Headers @{ Authorization = "Bearer $localToken" } `
            -ContentType 'application/json' -Body $validBody -UseBasicParsing -TimeoutSec 5
        $authedStatus = [int]$resp.StatusCode
    } catch {
        $authedStatus = [int]$_.Exception.Response.StatusCode.value__
    }
    $checks += [ordered]@{ name = 'auth-valid-200'; passed = ($authedStatus -eq 200); detail = "status=$authedStatus" }
}
finally {
    if ($adapterProcess -and -not $adapterProcess.HasExited) {
        Stop-Process -Id $adapterProcess.Id -Force -ErrorAction SilentlyContinue
    }
    Remove-Item Env:INBOUND_AUTH_TOKEN -ErrorAction SilentlyContinue
    Remove-Item Env:ADDR -ErrorAction SilentlyContinue
    Remove-Item Env:ELASTIC_MCP_ENABLED -ErrorAction SilentlyContinue
    Remove-Item Env:APP_ENV -ErrorAction SilentlyContinue
}

# 5. Log hygiene: neither the secret token nor raw evidence may appear in logs.
$adapterLogPath = Join-Path $logsDir 'adapter.log'
$logRaw = ''
if (Test-Path $adapterLogPath) { $logRaw = Get-Content $adapterLogPath -Raw }
$tokenLeaked = $logRaw -match [regex]::Escape($localToken)
$evidenceLeaked = $logRaw -match 'SENSITIVE_EVIDENCE_E3'
$checks += [ordered]@{ name = 'no-secret-in-logs'; passed = (-not $tokenLeaked); detail = "leaked=$tokenLeaked" }
$checks += [ordered]@{ name = 'no-raw-evidence-in-logs'; passed = (-not $evidenceLeaked); detail = "leaked=$evidenceLeaked" }

# 6. Optional: least-privilege IAM verification.
$iam = [ordered]@{ attempted = [bool]$GcpChecks }
if ($GcpChecks) {
    if (-not $ServiceAccount) { $ServiceAccount = "pulseops-agent-svc@$Project.iam.gserviceaccount.com" }
    $iamLog = Join-Path $logsDir 'iam.log'
    $rolesRaw = & gcloud projects get-iam-policy $Project --flatten="bindings[].members" `
        --filter="bindings.members:serviceAccount:$ServiceAccount" --format="value(bindings.role)" 2>&1
    Set-Content -Path $iamLog -Value $rolesRaw
    $actualRoles = @($rolesRaw | Where-Object { $_ -match '^roles/' })
    $extra = @($actualRoles | Where-Object { $expectedRoles -notcontains $_ })
    $missing = @($expectedRoles | Where-Object { $actualRoles -notcontains $_ })
    $iam.serviceAccount = $ServiceAccount
    $iam.actualRoles = $actualRoles
    $iam.extraRoles = $extra
    $iam.missingRoles = $missing
    $checks += [ordered]@{ name = 'iam-least-privilege'; passed = ($extra.Count -eq 0 -and $missing.Count -eq 0); detail = "extra=$($extra -join ',') missing=$($missing -join ',')" }
}

# 7. Optional: timed rollback drill with contract smoke before/after.
$rollback = [ordered]@{ attempted = [bool]$RollbackDrill }
if ($RollbackDrill) {
    $fixturePath = Join-Path $repoRoot 'docs\contracts\adk_request_fixture.json'
    $fixtureBody = Get-Content $fixturePath -Raw
    $headers = @{}
    if ($AuthToken) { $headers = @{ Authorization = "Bearer $AuthToken" } }

    function Invoke-ContractSmoke {
        param([string]$Url)
        try {
            $r = Invoke-WebRequest -Uri "$Url/investigate" -Method Post -Headers $headers `
                -ContentType 'application/json' -Body $fixtureBody -UseBasicParsing -TimeoutSec 15
            $obj = $r.Content | ConvertFrom-Json
            $ok = ($null -ne $obj.request_id) -and ($null -ne $obj.trace_id) -and ($null -ne $obj.status.transport) -and ($null -ne $obj.status.workflow)
            return $ok
        } catch {
            return $false
        }
    }

    $url = (& gcloud run services describe $Service --region $Region --project $Project --format='value(status.url)').Trim()
    $stable = (& gcloud run services describe $Service --region $Region --project $Project --format='value(status.traffic[0].revisionName)').Trim()
    $rollback.serviceUrl = $url
    $rollback.stableRevision = $stable

    $smokeBefore = Invoke-ContractSmoke -Url $url
    $checks += [ordered]@{ name = 'rollback-contract-smoke-before'; passed = $smokeBefore; detail = "url=$url" }

    # Tag stable, promote candidate (latest), then time the rollback to stable.
    & gcloud run services update-traffic $Service --region $Region --project $Project --set-tags "stable=$stable" | Out-Null
    & gcloud run services update-traffic $Service --region $Region --project $Project --to-latest | Out-Null

    $rollbackStart = Get-Date
    & gcloud run services update-traffic $Service --region $Region --project $Project --to-revisions "$stable=100" | Out-Null
    $rollbackSeconds = [Math]::Round(((Get-Date) - $rollbackStart).TotalSeconds, 1)
    $rollback.rollbackSeconds = $rollbackSeconds

    $smokeAfter = Invoke-ContractSmoke -Url $url
    $checks += [ordered]@{ name = 'rollback-contract-smoke-after'; passed = $smokeAfter; detail = "seconds=$rollbackSeconds" }
    $checks += [ordered]@{ name = 'rollback-under-5min'; passed = ($rollbackSeconds -lt 300); detail = "seconds=$rollbackSeconds" }
}

$allPassed = ($checks | Where-Object { -not $_.passed }).Count -eq 0
$summary = [ordered]@{
    runId = $timestamp
    startedAt = (Get-Date).ToString('o')
    artifactDir = $artifactRoot
    status = if ($allPassed) { 'passed' } else { 'failed' }
    checks = $checks
    iam = $iam
    rollback = $rollback
}
Write-JsonFile -Path (Join-Path $artifactRoot 'summary.json') -Value $summary

Write-Host ""
foreach ($c in $checks) {
    $mark = if ($c.passed) { 'PASS' } else { 'FAIL' }
    Write-Host ("  [{0}] {1} ({2})" -f $mark, $c.name, $c.detail)
}
Write-Host ""
Write-Host "[phase7-e3] Status: $($summary.status)"
Write-Host "[phase7-e3] Summary: $(Join-Path $artifactRoot 'summary.json')"

if (-not $allPassed) { exit 1 }
