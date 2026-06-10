<#
W6 - Create a Windows Compute Engine VM to run the PulseOps monitoring agent.

A Windows VM runs the Go agent UNCHANGED (it uses sc.exe / Restart-Service), so
the whole detect->remediate loop runs in the cloud against a real service
(e.g., Spooler) on the VM. The agent makes OUTBOUND calls to the Cloud Run
backend, so no inbound firewall rule is needed (RDP uses the network default).

Prerequisites: gcloud auth login ; billing enabled.

Usage:
  pwsh -NoProfile -File .\scripts\create-agent-vm.ps1 -Project pulseops-agent

After it finishes: set a Windows password, RDP in, then run
deploy/agent-vm/setup-agent.ps1 on the VM (see docs/HOSTING.md).
#>

param(
    [string]$Project = 'pulseops-agent',
    [string]$Zone = 'us-central1-a',
    [string]$Name = 'pulseops-agent-vm',
    [string]$MachineType = 'e2-medium',
    [string]$ImageFamily = 'windows-2022',
    [string]$RdpUser = 'pulseops'
)

$ErrorActionPreference = 'Stop'

function Invoke-Gcloud {
    param([Parameter(Mandatory = $true)][string[]]$GcloudArgs)
    Write-Host "> gcloud $($GcloudArgs -join ' ')" -ForegroundColor Cyan
    & gcloud @GcloudArgs
    if ($LASTEXITCODE -ne 0) { throw "gcloud failed (exit $LASTEXITCODE): $($GcloudArgs -join ' ')" }
}

Invoke-Gcloud @('services', 'enable', 'compute.googleapis.com', '--project', $Project)

Invoke-Gcloud @(
    'compute', 'instances', 'create', $Name,
    '--project', $Project, '--zone', $Zone,
    '--machine-type', $MachineType,
    '--image-family', $ImageFamily, '--image-project', 'windows-cloud',
    '--boot-disk-size', '50GB'
)

Write-Host ""
Write-Host "== VM created: $Name ($Zone) ==" -ForegroundColor Green
Write-Host "1) Set a Windows password (copy the output securely):" -ForegroundColor Yellow
Write-Host "   gcloud compute reset-windows-password $Name --zone $Zone --project $Project --user $RdpUser" -ForegroundColor Gray
Write-Host "2) RDP to the VM's external IP with that user/password:" -ForegroundColor Yellow
Write-Host "   gcloud compute instances describe $Name --zone $Zone --project $Project --format='value(networkInterfaces[0].accessConfigs[0].natIP)'" -ForegroundColor Gray
Write-Host "3) In an ADMIN PowerShell on the VM, run deploy/agent-vm/setup-agent.ps1 (see docs/HOSTING.md)." -ForegroundColor Yellow
