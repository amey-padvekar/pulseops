<#
Phase E1 test matrix runner

Purpose:
- Execute the Phase E1 verification matrix (unit, integration, contract, smoke)
- Capture category logs and a single pass/fail summary artifact

Outputs:
- artifacts/phase7-smoke/<timestamp>-e1/
  - logs/*.log
  - summary.json

Usage:
    pwsh -NoProfile -File .\scripts\phase7-e1.ps1

Options:
    -SkipSmoke               Skip end-to-end smoke run
    -SmokeTimeoutSeconds 180 Timeout passed to phase7-smoke.ps1
    -SmokeKeepProcesses      Pass -KeepProcesses to phase7-smoke.ps1
#>

param(
    [switch]$SkipSmoke,
    [int]$SmokeTimeoutSeconds = 180,
    [switch]$SmokeKeepProcesses
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

function Invoke-ExternalStep {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][string]$FilePath,
        [string[]]$Arguments,
        [Parameter(Mandatory = $true)][string]$WorkingDirectory,
        [Parameter(Mandatory = $true)][string]$LogPath
    )

    $startedAt = Get-Date
    $stderrPath = "$LogPath.err"
    $p = Start-Process -FilePath $FilePath -ArgumentList $Arguments -WorkingDirectory $WorkingDirectory -RedirectStandardOutput $LogPath -RedirectStandardError $stderrPath -PassThru -NoNewWindow -Wait
    $endedAt = Get-Date

    if (Test-Path $stderrPath) {
        $stderrText = Get-Content $stderrPath -Raw
        if (-not [string]::IsNullOrWhiteSpace($stderrText)) {
            Add-Content -Path $LogPath -Value "`n--- STDERR ---`n"
            Add-Content -Path $LogPath -Value $stderrText
        }
        Remove-Item -Path $stderrPath -Force -ErrorAction SilentlyContinue
    }

    return [ordered]@{
        name = $Name
        file = $FilePath
        args = $Arguments
        cwd = $WorkingDirectory
        startedAt = $startedAt.ToString('o')
        endedAt = $endedAt.ToString('o')
        durationSeconds = [Math]::Round(($endedAt - $startedAt).TotalSeconds, 3)
        exitCode = $p.ExitCode
        passed = ($p.ExitCode -eq 0)
        log = $LogPath
    }
}

function Get-LatestPhase7SmokeRun {
    param(
        [Parameter(Mandatory = $true)][string]$RootPath,
        [Parameter(Mandatory = $true)][datetime]$NotBefore
    )

    if (-not (Test-Path $RootPath)) {
        return $null
    }

    $dirs = Get-ChildItem -Path $RootPath -Directory | Where-Object { $_.LastWriteTime -ge $NotBefore } | Sort-Object LastWriteTime -Descending
    if (-not $dirs -or $dirs.Count -eq 0) {
        return $null
    }

    return $dirs[0].FullName
}

$repoRoot = Split-Path -Parent $PSScriptRoot
$timestamp = (Get-Date).ToString('yyyyMMdd-HHmmss')
$artifactRoot = Join-Path $repoRoot (Join-Path 'artifacts\phase7-smoke' ($timestamp + '-e1'))
$logsDir = Join-Path $artifactRoot 'logs'

New-Item -ItemType Directory -Path $logsDir -Force | Out-Null

$backendDir = Join-Path $repoRoot 'backend'
$adapterDir = Join-Path $repoRoot 'google-agent-service\cloudrun-adapter'
$agentServiceDir = Join-Path $repoRoot 'google-agent-service\agent'
$googleAgentServiceRoot = Join-Path $repoRoot 'google-agent-service'
$phase7SmokeScript = Join-Path $repoRoot 'scripts\phase7-smoke.ps1'
$phase7SmokeRoot = Join-Path $repoRoot 'artifacts\phase7-smoke'

$results = [ordered]@{
    runId = $timestamp
    startedAt = (Get-Date).ToString('o')
    artifactDir = $artifactRoot
    categories = [ordered]@{
        unit = @()
        integration = @()
        contract = @()
        smoke = @()
    }
}

Write-Host "[phase7-e1] Artifacts: $artifactRoot"

# Unit tests
$results.categories.unit += Invoke-ExternalStep -Name 'backend-agentbuilder-unit' -FilePath 'go' -Arguments @('test', './internal/agentbuilder', '-count=1') -WorkingDirectory $backendDir -LogPath (Join-Path $logsDir 'unit-backend-agentbuilder.log')
$results.categories.unit += Invoke-ExternalStep -Name 'adapter-request-envelope-unit' -FilePath 'go' -Arguments @('test', './internal/httpapi', './internal/safety', '-count=1') -WorkingDirectory $adapterDir -LogPath (Join-Path $logsDir 'unit-adapter-httpapi-safety.log')
$results.categories.unit += Invoke-ExternalStep -Name 'agent-schema-validator-unit' -FilePath 'python' -Arguments @('-m', 'pytest', 'agent/tests/test_schema_validator.py', 'agent/tests/test_c2_schema_policy_validator.py') -WorkingDirectory $googleAgentServiceRoot -LogPath (Join-Path $logsDir 'unit-agent-schema.log')

# Integration tests
$results.categories.integration += Invoke-ExternalStep -Name 'adapter-mocked-adk-integration' -FilePath 'go' -Arguments @('test', './internal/httpapi', '-count=1') -WorkingDirectory $adapterDir -LogPath (Join-Path $logsDir 'integration-adapter-httpapi.log')
$results.categories.integration += Invoke-ExternalStep -Name 'agent-mocked-tools-integration' -FilePath 'python' -Arguments @('-m', 'pytest', 'agent/tests/test_c1_evidence_pipeline.py', 'agent/tests/test_c1_investigation_workflow.py') -WorkingDirectory $googleAgentServiceRoot -LogPath (Join-Path $logsDir 'integration-agent-workflow.log')
$results.categories.integration += Invoke-ExternalStep -Name 'backend-timeout-retry-fallback-integration' -FilePath 'go' -Arguments @('test', './cmd/server', '-run', 'TestSubmitAgentBuilderRequest_Fallback', '-count=1') -WorkingDirectory $backendDir -LogPath (Join-Path $logsDir 'integration-backend-fallback.log')

# Contract tests
$results.categories.contract += Invoke-ExternalStep -Name 'backend-request-builder-contract' -FilePath 'go' -Arguments @('test', './internal/agentbuilder', '-run', 'TestBuildADKRequestPayload_IncludesTraceMetadata', '-count=1') -WorkingDirectory $backendDir -LogPath (Join-Path $logsDir 'contract-backend-request-builder.log')
$results.categories.contract += Invoke-ExternalStep -Name 'adapter-fixture-roundtrip-contract' -FilePath 'go' -Arguments @('test', './internal/httpapi', '-run', 'TestInvestigateRoundTripWithFrozenFixture', '-count=1') -WorkingDirectory $adapterDir -LogPath (Join-Path $logsDir 'contract-adapter-roundtrip.log')
$results.categories.contract += Invoke-ExternalStep -Name 'backend-parser-contract' -FilePath 'go' -Arguments @('test', './internal/agentbuilder', '-run', 'TestParseInvestigationResult_Valid|TestParseInvestigationResult_InvalidActionID|TestParseInvestigationResult_MalformedJSON', '-count=1') -WorkingDirectory $backendDir -LogPath (Join-Path $logsDir 'contract-backend-parser.log')

# Smoke test
if ($SkipSmoke) {
    $results.categories.smoke += [ordered]@{
        name = 'phase7-smoke'
        skipped = $true
        reason = 'SkipSmoke switch provided'
    }
} else {
    $smokeStartedAt = Get-Date
    $smokeShell = 'pwsh'
    if (-not (Get-Command pwsh -ErrorAction SilentlyContinue)) {
        $smokeShell = 'powershell'
    }

    $smokeArgs = @('-NoProfile', '-File', $phase7SmokeScript, '-TimeoutSeconds', [string]$SmokeTimeoutSeconds)
    if ($SmokeKeepProcesses) {
        $smokeArgs += '-KeepProcesses'
    }

    $smokeRun = Invoke-ExternalStep -Name 'phase7-smoke' -FilePath $smokeShell -Arguments $smokeArgs -WorkingDirectory $repoRoot -LogPath (Join-Path $logsDir 'smoke-phase7.log')
    $latestSmokeDir = Get-LatestPhase7SmokeRun -RootPath $phase7SmokeRoot -NotBefore $smokeStartedAt

    if ($latestSmokeDir) {
        $smokeRun['artifactDir'] = $latestSmokeDir
        $smokeSummaryPath = Join-Path $latestSmokeDir 'summary.json'
        if (Test-Path $smokeSummaryPath) {
            try {
                $smokeSummary = Get-Content $smokeSummaryPath -Raw | ConvertFrom-Json
                $smokeRun['checks'] = $smokeSummary.checks
            } catch {
                $smokeRun['checksReadError'] = $_.Exception.Message
            }
        }
    }

    $results.categories.smoke += $smokeRun
}

$allSteps = @()
$allSteps += $results.categories.unit
$allSteps += $results.categories.integration
$allSteps += $results.categories.contract
$allSteps += $results.categories.smoke

$hasKey = {
    param($obj, [string]$key)
    return ($null -ne $obj -and $null -ne $obj.Keys -and ($obj.Keys -contains $key))
}

$failed = @($allSteps | Where-Object {
    if (& $hasKey $_ 'skipped' -and $_.skipped) {
        return $false
    }
    if (& $hasKey $_ 'passed') {
        return -not $_.passed
    }
    return $false
})

$results['endedAt'] = (Get-Date).ToString('o')
$results['status'] = if ($failed.Count -eq 0) { 'passed' } else { 'failed' }
$results['failedStepCount'] = $failed.Count
$results['failedSteps'] = @($failed | ForEach-Object { $_.name })

$summaryPath = Join-Path $artifactRoot 'summary.json'
Write-JsonFile -Path $summaryPath -Value $results

if ($results.status -eq 'passed') {
    Write-Host "[phase7-e1] PASS: all selected matrix steps passed. Summary: $summaryPath"
    exit 0
}

Write-Warning "[phase7-e1] FAIL: one or more matrix steps failed. Summary: $summaryPath"
exit 1
