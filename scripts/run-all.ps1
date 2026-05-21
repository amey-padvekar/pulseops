$ErrorActionPreference = 'Stop'

$scriptDir = $PSScriptRoot

Write-Host '[run-all] Opening three terminals (backend, agent, frontend)...'

Start-Process powershell -ArgumentList @('-NoExit', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $scriptDir 'run-backend.ps1')) | Out-Null
Start-Process powershell -ArgumentList @('-NoExit', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $scriptDir 'run-agent.ps1')) | Out-Null
Start-Process powershell -ArgumentList @('-NoExit', '-ExecutionPolicy', 'Bypass', '-File', (Join-Path $scriptDir 'run-frontend.ps1')) | Out-Null

Write-Host '[run-all] Terminals launched.'
