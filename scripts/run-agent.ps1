$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$agentDir = Join-Path $repoRoot 'agent'

Write-Host '[run-agent] Starting agent...'
Push-Location $agentDir
go run ./cmd/agent
Pop-Location
