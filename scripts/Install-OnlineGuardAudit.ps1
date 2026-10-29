[CmdletBinding()]
param([string]$Root = 'C:\MinecraftServers\MultiServer')
$ErrorActionPreference = 'Stop'

$taskName = 'NetherNetOnlineGuardAudit'
$scriptPath = Join-Path $Root 'Refresh-OnlineGuardEvidence.ps1'
if (-not (Test-Path -LiteralPath $scriptPath -PathType Leaf)) { throw "Audit script is missing: $scriptPath" }

$command = "`"$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe`" -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File `"$scriptPath`""
& schtasks.exe /Create /TN $taskName /TR $command /SC MINUTE /MO 1 /RU SYSTEM /RL HIGHEST /F | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Windows could not register the NetherNet Online audit task.' }
& schtasks.exe /Run /TN $taskName | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Windows could not start the NetherNet Online audit task.' }
Write-Output 'NetherNet Online Guard Audit installed and started.'
