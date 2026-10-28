param([string]$FixtureRoot,[string]$Scenario)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Install-Tools.ps1')
$source=Join-Path $FixtureRoot 'source';$root=Join-Path $FixtureRoot 'destination';New-Item -ItemType Directory -Force (Join-Path $source 'setup-assets'),(Join-Path $root 'MultiServer')|Out-Null
function Get-CimInstance {param($ClassName)}
$entries=@();foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){[IO.File]::WriteAllText((Join-Path $source $name),('new '+$name));$entries+=$name+':'+(Get-NNHash (Join-Path $source $name));[IO.File]::WriteAllText((Join-Path $root ('MultiServer/'+$name)),('old '+$name))}
[IO.File]::WriteAllText((Join-Path $source 'NetherNetMultiServerSetup.exe'),'setup fixture')
foreach($name in @('Configure-Windows.ps1','Network-Configuration.ps1','Runtime-Operations.ps1','Decommission-NetherNet.ps1','Install-Tools.ps1','Setup-Wizard.ps1','Archive-Operations.ps1')){[IO.File]::WriteAllText((Join-Path $source ('setup-assets/'+$name)),('fixture '+$name))}
[IO.File]::WriteAllText((Join-Path $source 'release-hashes.json'),'{}')
if($Scenario -eq 'failure'){
 function Copy-NNToolFile {param($Path,$Bytes);if($Path -like '*NetherNetMultiServerAdmin.exe'){throw 'simulated copy failure'};[IO.File]::WriteAllBytes($Path,$Bytes)}
}
$failed=$false
try{
 if($Scenario -eq 'script-only'){Invoke-NNInstallTools -Root $root -SourceDirectory $source -CompanionDirectory $source -ScriptOnly -CompanionHashes ($entries -join ',')}
 elseif($Scenario -eq 'entrypoint'){& (Join-Path $PSScriptRoot '../setup-assets/Install-Tools.ps1') -Root $root -SourceDirectory $source -CompanionDirectory $source -SetupPath (Join-Path $source 'NetherNetMultiServerSetup.exe') -CompanionHashes ($entries -join ',')}
 else{Invoke-NNInstallTools -Root $root -SourceDirectory $source -CompanionDirectory $source -SetupPath (Join-Path $source 'NetherNetMultiServerSetup.exe') -CompanionHashes ($entries -join ',')}
}catch{if($Scenario -ne 'failure'){throw};$failed=$true}
[IO.File]::WriteAllText((Join-Path $source 'release-hashes.json'),'{}')
if($Scenario -eq 'failure'){
 if(-not $failed){throw 'failure not observed'}
 foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){if([IO.File]::ReadAllText((Join-Path $root ('MultiServer/'+$name))) -ne ('old '+$name)){throw 'software rollback failed'}}
}else{
 foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){if([IO.File]::ReadAllText((Join-Path $root ('MultiServer/'+$name))) -ne ('new '+$name)){throw 'software copy failed'}}
 if(-not(Test-Path (Join-Path $root 'MultiServer/installer/Uninstall-NetherNet.cmd'))){throw 'uninstall launcher missing'}
}


if($Scenario -eq 'script-only' -and (-not(Test-Path (Join-Path $root 'MultiServer/installer/setup-assets/Setup-Wizard.ps1')) -or -not(Test-Path (Join-Path $root 'MultiServer/installer/setup-assets/Archive-Operations.ps1')))){throw 'script-only assets missing'}

