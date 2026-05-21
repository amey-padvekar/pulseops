$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot

Write-Host '[install-deps] Installing Go and frontend dependencies...'

Push-Location (Join-Path $repoRoot 'agent')
go mod tidy
Pop-Location

Push-Location (Join-Path $repoRoot 'backend')
go mod tidy
Pop-Location

Push-Location (Join-Path $repoRoot 'frontend')
npm install
Pop-Location

Write-Host '[install-deps] Done.'
