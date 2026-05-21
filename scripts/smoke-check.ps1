$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'
$agentDir = Join-Path $repoRoot 'agent'
$frontendDir = Join-Path $repoRoot 'frontend'
$backendUrl = 'http://localhost:8080/healthz'

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

Write-Host '[smoke-check] Checking backend health endpoint...'
$backendProc = Start-Process go -WorkingDirectory $backendDir -ArgumentList 'run', './cmd/server' -PassThru

try {
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

    Write-Host '[smoke-check] PASS: build checks succeeded and backend /healthz returned ok.'
} finally {
    if ($backendProc -and -not $backendProc.HasExited) {
        Stop-Process -Id $backendProc.Id -Force
    }
}
