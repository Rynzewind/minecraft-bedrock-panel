[CmdletBinding()]
param([string]$Root,[string]$SourceDirectory,[string]$CompanionDirectory,[string]$SetupPath,[string]$CompanionHashes,[string]$ArchivePath,[switch]$ScriptOnly)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'Network-Configuration.ps1')
function Copy-NNToolFile([string]$Path,[byte[]]$Bytes){[IO.File]::WriteAllBytes($Path,$Bytes)}
function Invoke-NNInstallTools([string]$Root,[string]$SourceDirectory,[string]$CompanionDirectory,[string]$SetupPath,[string]$CompanionHashes,[string]$ArchivePath,[switch]$ScriptOnly){
 $rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\');Assert-NNScope $rootPath $rootPath|Out-Null
 $active=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($rootPath+'\',[StringComparison]::OrdinalIgnoreCase) -and $_.Name -in @('bedrock_server.exe','NetherNetMultiServer.exe')})
 if($active.Count){throw 'Stop the installation cleanly before updating its software.'}
 $files=[Collections.Generic.List[object]]::new()
 function Add-Replacement([string]$Path,[byte[]]$Bytes){
  Assert-NNScope $rootPath $Path|Out-Null
  $exists=Test-Path -LiteralPath $Path
    $old=$null
  if($exists){$old=[IO.File]::ReadAllBytes($Path);$sha=[Security.Cryptography.SHA256]::Create();try{$same=([BitConverter]::ToString($sha.ComputeHash($old))) -eq ([BitConverter]::ToString($sha.ComputeHash($Bytes)))}finally{$sha.Dispose()};if($same){return}}
  $files.Add([pscustomobject]@{path=$Path;bytes=$Bytes;old=$old;existed=$exists})
 }
 $seen=@{}
 foreach($entry in ($CompanionHashes -split ',')){
  $parts=$entry -split ':'
  if($parts.Count -ne 2 -or $parts[0] -notin @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe') -or $seen.ContainsKey($parts[0])){throw 'Invalid companion executable manifest.'}
  $seen[$parts[0]]=$true;$source=Join-Path $CompanionDirectory $parts[0]
  if((Get-NNHash $source) -ne $parts[1]){throw ('Companion executable checksum failed: '+$parts[0])}
  Add-Replacement (Join-Path $rootPath ('MultiServer\'+$parts[0])) ([IO.File]::ReadAllBytes($source))
 }
 if($seen.Count -ne 3){throw 'Companion executable manifest is incomplete.'}
 $destination=Join-Path $rootPath 'MultiServer\installer'
  if($ScriptOnly){
  Add-Replacement (Join-Path $destination 'release-hashes.json') ([IO.File]::ReadAllBytes((Join-Path $SourceDirectory 'release-hashes.json')))
  $install="@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Setup-Wizard.ps1`" %*`r`n"
  Add-Replacement (Join-Path $destination 'Install-NetherNet.cmd') ([Text.Encoding]::UTF8.GetBytes($install))
 }elseif($ArchivePath){
  if([IO.Path]::GetFileName($ArchivePath) -ne 'NetherNetMigrationArchive.exe'){throw 'Unrecognised archive executable.'}
  Add-Replacement (Join-Path $destination 'NetherNetMigrationArchive.exe') ([IO.File]::ReadAllBytes($ArchivePath))
  Add-Replacement (Join-Path $destination 'release-hashes.json') ([IO.File]::ReadAllBytes((Join-Path $SourceDirectory 'release-hashes.json')))
  $install="@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Setup-Wizard.ps1`" %*`r`n"
  Add-Replacement (Join-Path $destination 'Install-NetherNet.cmd') ([Text.Encoding]::UTF8.GetBytes($install))
 }else{
  if([IO.Path]::GetFileName($SetupPath) -ne 'NetherNetMultiServerSetup.exe'){throw 'Unrecognised setup executable.'}
  Add-Replacement (Join-Path $destination 'NetherNetMultiServerSetup.exe') ([IO.File]::ReadAllBytes($SetupPath))
 }
 foreach($name in @('Configure-Windows.ps1','Network-Configuration.ps1','Runtime-Operations.ps1','Decommission-NetherNet.ps1','Install-Tools.ps1')){
  Add-Replacement (Join-Path $destination ('setup-assets\'+$name)) ([IO.File]::ReadAllBytes((Join-Path $SourceDirectory ('setup-assets\'+$name))))
 }
 if($ArchivePath -or $ScriptOnly){Add-Replacement (Join-Path $destination 'setup-assets\Setup-Wizard.ps1') ([IO.File]::ReadAllBytes((Join-Path $SourceDirectory 'setup-assets\Setup-Wizard.ps1')))}
 if($ScriptOnly){Add-Replacement (Join-Path $destination 'setup-assets\Archive-Operations.ps1') ([IO.File]::ReadAllBytes((Join-Path $SourceDirectory 'setup-assets\Archive-Operations.ps1')))}
 $launcher="@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Decommission-NetherNet.ps1`" %*`r`n"
 Add-Replacement (Join-Path $destination 'Uninstall-NetherNet.cmd') ([Text.Encoding]::UTF8.GetBytes($launcher))
 foreach($file in $files){if($file.existed){$handle=[IO.File]::Open($file.path,[IO.FileMode]::Open,[IO.FileAccess]::Write,[IO.FileShare]::Read);$handle.Dispose()}}
 $backup=Join-Path $rootPath ('MultiServer\backups\setup-software-'+[guid]::NewGuid().ToString('N'))
 $completed=[Collections.Generic.List[object]]::new()
 try{
  foreach($file in $files){
   if($file.existed){$relative=$file.path.Substring($rootPath.Length+1);$saved=Join-Path $backup $relative;New-Item -ItemType Directory -Path (Split-Path $saved -Parent) -Force|Out-Null;[IO.File]::WriteAllBytes($saved,$file.old)}
   New-Item -ItemType Directory -Path (Split-Path $file.path -Parent) -Force|Out-Null
   $completed.Add($file);Copy-NNToolFile $file.path $file.bytes
  }
 }catch{
  $reason=$_.Exception.Message
  for($i=$completed.Count-1;$i -ge 0;$i--){$file=$completed[$i];try{if($file.existed){[IO.File]::WriteAllBytes($file.path,$file.old)}elseif(Test-Path -LiteralPath $file.path){Remove-Item -LiteralPath $file.path -Force}}catch{$reason+='; rollback needs attention: '+$_.Exception.Message}}
  throw ($reason+'; original software backups: '+$backup)
 }
}
if($MyInvocation.InvocationName -ne '.'){
 try{Invoke-NNInstallTools -Root $Root -SourceDirectory $SourceDirectory -CompanionDirectory $CompanionDirectory -SetupPath $SetupPath -CompanionHashes $CompanionHashes -ArchivePath $ArchivePath -ScriptOnly:$ScriptOnly}catch{Write-Error $_ -ErrorAction Continue;exit 1}
}




