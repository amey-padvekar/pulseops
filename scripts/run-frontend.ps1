$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent $PSScriptRoot
$frontendDir = Join-Path $repoRoot 'frontend'

Write-Host '[run-frontend] Starting frontend...'
Push-Location $frontendDir
npm run dev
Pop-Location
