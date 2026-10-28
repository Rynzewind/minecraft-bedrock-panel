param([string]$FixtureRoot,[ValidateSet('retain','remove')][string]$Scenario)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Decommission-NetherNet.ps1')
$script:fixture=$FixtureRoot;$script:taskPresent=$true;$script:removedTask=''
$root=Join-Path $FixtureRoot 'installation';$report=Join-Path $FixtureRoot 'report';$outside=Join-Path $FixtureRoot 'unrelated.txt'
New-Item -ItemType Directory -Force (Join-Path $root 'Creative\worlds\World'),(Join-Path $root 'MultiServer'),$report|Out-Null
[IO.File]::WriteAllText($outside,'unrelated');$world=Join-Path $root 'Creative\worlds\World\data';[IO.File]::WriteAllText($world,'world-data')
[IO.File]::WriteAllText((Join-Path $root 'MultiServer\config.json'),'{"servers":[]}')
$script:root=$root
function Get-NNInventory {param($Root)
 [pscustomobject]@{root=$script:root;attention=@();tasks=@($(if($script:taskPresent){[pscustomobject]@{name='owned-task';path='\'}}));services=@();processes=@();startup=@();firewall=@();aliases=@();uninstall=@();directories=@((Join-Path $script:root 'Creative'),(Join-Path $script:root 'MultiServer'));root_files=@()}
}
function Disable-ScheduledTask {param($TaskName,$TaskPath);if($TaskName -ne 'owned-task'){throw 'wrong task'}}
function Stop-ScheduledTask {[CmdletBinding()]param($TaskName,$TaskPath)}
function Unregister-ScheduledTask {[CmdletBinding(SupportsShouldProcess)]param($TaskName,$TaskPath);$script:removedTask=$TaskName;$script:taskPresent=$false}
function Get-CimInstance {param($ClassName)}
function Get-NetTCPConnection {[CmdletBinding()]param($State)}
$inventory=Get-NNInventory $root
$result=Invoke-NNDecommission $inventory ($Scenario -eq 'remove') $report
if($script:removedTask -ne 'owned-task'){throw 'owned startup task was not removed'}
if([IO.File]::ReadAllText($outside) -ne 'unrelated'){throw 'unrelated file changed'}
if($Scenario -eq 'retain'){if([IO.File]::ReadAllText($world) -ne 'world-data'){throw 'retain mode changed world'}}
else{if(Test-Path -LiteralPath $root){throw 'complete removal left project marker/data in an otherwise empty root'}}
if($result.reboot_verification -ne 'pending'){throw 'unobserved reboot claimed verified'}

