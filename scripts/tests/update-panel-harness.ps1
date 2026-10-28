param([string]$FixtureRoot)
$ErrorActionPreference='Stop'
$source=Join-Path $FixtureRoot 'package';$root=Join-Path $FixtureRoot 'installation'
New-Item -ItemType Directory -Force $source,(Join-Path $root 'MultiServer'),(Join-Path $root 'Creative\worlds')|Out-Null
[IO.File]::WriteAllText((Join-Path $root 'MultiServer\config.json'),'original-config')
[IO.File]::WriteAllText((Join-Path $root 'Creative\worlds\kept.txt'),'world unchanged')
function Get-CimInstance {param($ClassName,$Filter)}
function Get-FileHash {param($LiteralPath,$Algorithm);$stream=[IO.File]::OpenRead($LiteralPath);$sha=[Security.Cryptography.SHA256]::Create();try{[pscustomobject]@{Hash=([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','')}}finally{$stream.Dispose();$sha.Dispose()}}
$files=@('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe','NetherNet-Migration-Source.zip','setup-assets/Configure-Windows.ps1','setup-assets/Network-Configuration.ps1','setup-assets/Runtime-Operations.ps1','setup-assets/Decommission-NetherNet.ps1','setup-assets/Install-Tools.ps1','Uninstall-NetherNet.cmd','Install-NetherNet.cmd','setup-assets/Setup-Wizard.ps1','setup-assets/Archive-Operations.ps1')
$hashes=@{};foreach($file in $files){$path=Join-Path $source $file;New-Item -ItemType Directory -Path (Split-Path $path -Parent) -Force|Out-Null;[IO.File]::WriteAllText($path,('fixture '+$file));$hashes[$file]=(Get-FileHash $path).Hash}
$hashes|ConvertTo-Json|Set-Content (Join-Path $source 'release-hashes.json')
& (Join-Path $PSScriptRoot '../update-panel.ps1') -ServerRoot $root -PackageRoot $source
if((Get-FileHash (Join-Path $source 'release-hashes.json')).Hash -ne (Get-FileHash (Join-Path $root 'MultiServer\installer\release-hashes.json')).Hash){throw 'installed metadata mismatch'}
if([IO.File]::ReadAllText((Join-Path $root 'Creative\worlds\kept.txt')) -ne 'world unchanged'){throw 'updater modified worlds'}
if([IO.File]::ReadAllText((Join-Path $root 'MultiServer\config.json')) -ne 'original-config'){throw 'updater modified configuration'}
