$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$backendDir = Join-Path $repoRoot 'backend'

Write-Host '[run-backend] Starting backend...'
Push-Location $backendDir
go run ./cmd/server
Pop-Location
