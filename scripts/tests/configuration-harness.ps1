param([string]$FixtureRoot,[string]$Scenario)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Configure-Windows.ps1')
$script:fixtureRoot=$FixtureRoot
$script:scenario=$Scenario
function Get-InstallerElevationState {[pscustomobject]@{IsAdmin=$true}}
function Get-ScheduledTask {[CmdletBinding()]param($TaskName);[pscustomobject]@{TaskName=$TaskName}}
function Disable-ScheduledTask {[CmdletBinding()]param($TaskName);Set-Content (Join-Path $script:fixtureRoot 'disabled.txt') $TaskName}
function Get-NetIPAddress {[CmdletBinding()]param($AddressFamily);@([pscustomobject]@{IPAddress='10.42.8.9';InterfaceIndex=9},[pscustomobject]@{IPAddress='10.42.9.246';InterfaceIndex=9},[pscustomobject]@{IPAddress='10.42.9.247';InterfaceIndex=9})}
function Invoke-NNNetworkSetup {param($Plan);if(-not $Plan.network){throw 'Missing network plan'};[pscustomobject]@{}}
function Get-NetConnectionProfile {[CmdletBinding()]param($InterfaceIndex);[pscustomobject]@{InterfaceAlias="Fixture LAN";NetworkCategory=$(if($script:scenario -eq 'public'){'Public'}else{'Private'})}}
function Get-NetFirewallRule {[CmdletBinding()]param($Group)}
function Remove-NetFirewallRule {[CmdletBinding()]param([Parameter(ValueFromPipeline)]$InputObject)}
function New-NetFirewallRule {[CmdletBinding()]param($Name,$DisplayName,$Group,$Direction,$Action,$Program,$Protocol,$Profile,$RemoteAddress,$LocalPort,$LocalAddress,$InterfaceAlias);@{Program=$Program;Protocol=$Protocol;Profile=$Profile;RemoteAddress=$RemoteAddress;LocalAddress=$LocalAddress;LocalPort=$LocalPort;Action=$Action}|ConvertTo-Json -Compress|Add-Content (Join-Path $script:fixtureRoot 'firewall.jsonl')}
function New-ScheduledTaskAction {param($Execute,$Argument,$WorkingDirectory);[pscustomobject]@{Execute=$Execute;Argument=$Argument;WorkingDirectory=$WorkingDirectory}}
function New-ScheduledTaskSettingsSet {param([switch]$AllowStartIfOnBatteries,[switch]$DontStopIfGoingOnBatteries,[switch]$StartWhenAvailable,$MultipleInstances,$ExecutionTimeLimit,$RestartCount,$RestartInterval,[switch]$Disable);[pscustomobject]@{Disabled=$Disable.IsPresent}}
function New-ScheduledTaskPrincipal {param($UserId,$LogonType,$RunLevel);[pscustomobject]@{UserId=$UserId;LogonType=$LogonType;RunLevel=$RunLevel}}
function New-ScheduledTaskTrigger {param([switch]$AtLogOn,$User,[switch]$AtStartup);[pscustomobject]@{User=$User;AtLogOn=$AtLogOn.IsPresent;AtStartup=$AtStartup.IsPresent}}
function New-ScheduledTask {param($Action,$Trigger,$Settings,$Principal);[pscustomobject]@{Action=$Action;Trigger=$Trigger;Settings=$Settings;Principal=$Principal}}
function Register-ScheduledTask {param($TaskName,$InputObject,[switch]$Force);$InputObject|ConvertTo-Json -Depth 8|Set-Content (Join-Path $script:fixtureRoot 'task.json')}
function Enable-ScheduledTask {[CmdletBinding()]param($TaskName);Set-Content (Join-Path $script:fixtureRoot 'enabled.txt') $TaskName}
function Unregister-ScheduledTask {[CmdletBinding(SupportsShouldProcess)]param($TaskName)}
function Test-Path {
    [CmdletBinding()]param([string]$LiteralPath,[string]$PathType)
    if ($LiteralPath -like ($env:SystemRoot+'\System32\*.dll')) {return $true}
    Microsoft.PowerShell.Management\Test-Path @PSBoundParameters
}
Invoke-MinecraftWindowsSetup -PlanPath (Join-Path $FixtureRoot 'plan.json')




