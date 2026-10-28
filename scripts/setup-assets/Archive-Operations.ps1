# Snapshot format v1 operations using Windows/.NET APIs only. No bundled executable is invoked.
$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.IO.Compression,System.IO.Compression.FileSystem,System.Security
. (Join-Path $PSScriptRoot 'Network-Configuration.ps1')
function Assert-NNArchiveName([string]$Name){
 if(-not $Name -or $Name.Contains('\') -or $Name.Contains(':') -or $Name.StartsWith('/') -or $Name.EndsWith('/') -or $Name -match '[\x00-\x1f<>"|?*]'){throw 'Unsafe snapshot path.'}
 foreach($part in $Name.Split('/')){if(-not $part -or $part -in @('.','..') -or $part.TrimEnd(' ','.') -ne $part -or $part -match '^(?i)(CON|PRN|AUX|NUL|COM[0-9]|LPT[0-9])(?:\.|$)'){throw 'Unsafe Windows snapshot name.'}}
 if($Name -match '^(?i)MultiServer/(account-store/|auth-token)' -or $Name -match '^[^/]+/(backups|rollback|uninstall-backups|session-logs)/'){throw 'Snapshot contains nonportable credentials or operational backup data.'}
}
function Copy-NNCheckedStream($InputStream,$OutputStream,[long]$ExpectedSize,[string]$ExpectedHash){
 $sha=[Security.Cryptography.SHA256]::Create();$buffer=[byte[]]::new(1048576);[long]$count=0
 try{while($true){$limit=[int][Math]::Min($buffer.Length,$ExpectedSize-$count+1);$read=$InputStream.Read($buffer,0,$limit);if($read -eq 0){break};$count+=$read;if($count -gt $ExpectedSize){throw 'Snapshot entry exceeds its recorded size.'};if($OutputStream){$OutputStream.Write($buffer,0,$read)};$sha.TransformBlock($buffer,0,$read,$buffer,0)|Out-Null};$sha.TransformFinalBlock([byte[]]::new(0),0,0)|Out-Null;$hash=([BitConverter]::ToString($sha.Hash)).Replace('-','');if($count -ne $ExpectedSize -or ($ExpectedHash -and $hash -ne $ExpectedHash)){throw 'Snapshot checksum or size mismatch.'}}finally{$sha.Dispose()}
}
function Open-NNSnapshot([string]$ArchivePath){
 $zip=[IO.Compression.ZipFile]::OpenRead([IO.Path]::GetFullPath($ArchivePath))
 try{
  if($zip.Entries.Count -gt 300001){throw 'Too many snapshot entries.'}
  $entries=[Collections.Generic.Dictionary[string,object]]::new([StringComparer]::OrdinalIgnoreCase);[long]$total=0
  foreach($entry in $zip.Entries){Assert-NNArchiveName $entry.FullName;if($entries.ContainsKey($entry.FullName)){throw 'Duplicate snapshot path.'};$mode=([long]$entry.ExternalAttributes -shr 16) -band 0xf000;if(($mode -ne 0 -and $mode -ne 0x8000) -or ($entry.ExternalAttributes -band 0x400)){throw 'Snapshot links and nonregular files are not allowed.'};$total+=$entry.Length;if($entry.Length -lt 0 -or $total -gt 1TB){throw 'Snapshot expansion limit exceeded.'};$entries.Add($entry.FullName,$entry)}
  if(-not $entries.ContainsKey('manifest.json') -or $entries['manifest.json'].FullName -cne 'manifest.json' -or $entries['manifest.json'].Length -gt 16MB){throw 'Missing or oversized snapshot manifest.'}
  $memory=[IO.MemoryStream]::new();$stream=$entries['manifest.json'].Open();try{Copy-NNCheckedStream $stream $memory $entries['manifest.json'].Length ''; $manifest=([Text.UTF8Encoding]::new($false,$true)).GetString($memory.ToArray())|ConvertFrom-Json}finally{$stream.Dispose();$memory.Dispose()}
  if($manifest.format_version -ne 1 -or [string]$manifest.source_root -notmatch '^(?:[A-Za-z]:[\\/]|\\\\[^\\]+\\[^\\]+\\)'){throw 'Unsupported snapshot format/source root.'}
  if(@($manifest.files).Count+1 -ne $entries.Count){throw 'Manifest does not describe every snapshot payload.'}
  $seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
  foreach($file in $manifest.files){Assert-NNArchiveName $file.path;if($file.path -eq 'manifest.json' -or -not $seen.Add($file.path) -or $file.sha256 -notmatch '^[a-fA-F0-9]{64}$' -or [long]$file.size -lt 0 -or -not $entries.ContainsKey($file.path) -or $entries[$file.path].FullName -cne $file.path -or $entries[$file.path].Length -ne [long]$file.size){throw 'Invalid snapshot manifest entry.'}}
  return [pscustomobject]@{zip=$zip;manifest=$manifest;entries=$entries;expanded=$total}
 }catch{$zip.Dispose();throw}
}
function Test-NNSnapshotArchive([string]$ArchivePath){
 $snapshot=Open-NNSnapshot $ArchivePath
 try{$i=0;foreach($file in $snapshot.manifest.files){$stream=$snapshot.entries[$file.path].Open();try{Copy-NNCheckedStream $stream $null ([long]$file.size) $file.sha256}finally{$stream.Dispose()};$i++;if(($i%64) -eq 0 -and ('System.Windows.Forms.Application' -as [type])){[Windows.Forms.Application]::DoEvents()}};return $snapshot.manifest}finally{$snapshot.zip.Dispose()}
}
function Read-NNProperties([string]$Text){
 $values=[Collections.Generic.Dictionary[string,string]]::new([StringComparer]::Ordinal)
 foreach($line in ($Text -split '\r?\n')){$trimmed=$line.Trim();if(-not $trimmed -or $trimmed.StartsWith('#') -or $trimmed.StartsWith('!')){continue};$equal=$line.IndexOf('=');if($equal -lt 0){continue};$key=$line.Substring(0,$equal).Trim();if($values.ContainsKey($key)){throw ('Duplicate server property: '+$key)};$values.Add($key,$line.Substring($equal+1).Trim())}
 return $values
}
function Set-NNProperty([string]$Text,[string]$Key,[string]$Value){
 $values=Read-NNProperties $Text;$newline=if($Text.Contains("`r`n")){"`r`n"}else{"`n"};$lines=$Text.Split([string[]]@($newline),[StringSplitOptions]::None);$found=$false
 for($i=0;$i -lt $lines.Length;$i++){$equal=$lines[$i].IndexOf('=');if($equal -ge 0 -and $lines[$i].Substring(0,$equal).Trim() -ceq $Key){$lines[$i]=$Key+'='+$Value;$found=$true}}
 if($found){return ($lines -join $newline)}
 if($Text -and -not $Text.EndsWith($newline)){$Text+=$newline};return ($Text+$Key+'='+$Value+$newline)
}
function Convert-NNMigrationPath([string]$Path,[string]$SourceRoot,[string]$Destination){
 if(-not [IO.Path]::IsPathRooted($Path)){$Path=Join-Path (Join-Path $SourceRoot 'MultiServer') $Path}
 $full=[IO.Path]::GetFullPath($Path);$source=[IO.Path]::GetFullPath($SourceRoot).TrimEnd('\')
 if(-not $full.StartsWith($source+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Snapshot configuration path escapes the source installation.'}
 return (Join-Path $Destination $full.Substring($source.Length+1))
}
function Import-NNPortableAccounts([string]$Stage){
 $portable=Get-Content -LiteralPath (Join-Path $Stage 'accounts-migration.json') -Raw -Encoding UTF8|ConvertFrom-Json
 if($portable.version -ne 1){throw 'Unsupported account metadata.'}
 $ids=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal);$accounts=[Collections.Generic.List[object]]::new();$players=[Collections.Generic.List[object]]::new();$entropy=[Text.Encoding]::UTF8.GetBytes('NetherNetMultiServer/account-store/v1')
 foreach($account in $portable.accounts){if(-not $account.id -or -not $ids.Add($account.id)){throw 'Invalid or duplicate account ID.'};$cipher=[Security.Cryptography.ProtectedData]::Protect([Text.Encoding]::UTF8.GetBytes('{}'),$entropy,[Security.Cryptography.DataProtectionScope]::CurrentUser);$accounts.Add([ordered]@{ID=$account.id;PersonLabel=$account.person_label;EmailLabel=$account.email_label;Gamertag=$account.gamertag;XUID=$account.xuid;Token=[Convert]::ToBase64String($cipher);Order=[int]$account.order;Healthy=$false})}
 foreach($player in $portable.players){if(-not $ids.Contains($player.account_id)){throw 'Player mapping references a missing account.'};$players.Add($player)}
 $directory=Join-Path $Stage 'MultiServer/account-store';[IO.Directory]::CreateDirectory($directory)|Out-Null
 Write-NNJson (Join-Path $directory 'accounts.json') ([ordered]@{Version=1;Accounts=$accounts.ToArray();Players=$players.ToArray()})
}
function Write-NNPortableLaunchers([string]$Stage){
 $legacy=Join-Path $Stage 'StartMinecraftServers.vbs'
 if(Test-Path -LiteralPath $legacy){$backup=Join-Path $Stage 'MultiServer/backups/legacy-launchers';[IO.Directory]::CreateDirectory($backup)|Out-Null;[IO.File]::Move($legacy,(Join-Path $backup 'StartMinecraftServers.vbs'))}
 $start='$ErrorActionPreference=''Stop'''+"`r`n"+'. (Join-Path $PSScriptRoot ''MultiServer\installer\setup-assets\Runtime-Operations.ps1'')'+"`r`n"+'Start-NNWatch $PSScriptRoot'+"`r`n"
 $files=@{'StartMinecraftServers.ps1'=$start;'StartMinecraftServers.bat'="@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0StartMinecraftServers.ps1`"`r`n";'StopMinecraftServers.bat'="@echo off`r`n`"%~dp0MultiServer\NetherNetMultiServerControl.exe`" stop-all`r`n";'MultiServer/Run-NetherNetMultiServer.cmd'="@echo off`r`ncd /d `"%~dp0`"`r`n`"%~dp0NetherNetMultiServer.exe`" `"%~dp0config.json`"`r`n";'MultiServer/Start-NetherNetMultiServer.cmd'="@echo off`r`ncall `"%~dp0..\StartMinecraftServers.bat`"`r`n"}
 foreach($file in $files.GetEnumerator()){[IO.File]::WriteAllText((Join-Path $Stage $file.Key),$file.Value,[Text.UTF8Encoding]::new($false))}
}
function Restore-NNSnapshot([string]$ArchivePath,[string]$Destination,[string]$PrimaryIP,[string]$BroadcastIP,[hashtable]$ServerAddresses){
 $destinationPath=[IO.Path]::GetFullPath($Destination).TrimEnd('\');if(-not [IO.Path]::IsPathRooted($Destination) -or $destinationPath -eq [IO.Path]::GetPathRoot($destinationPath).TrimEnd('\') -or $destinationPath -match '[%\r\n"!`$]'){throw 'Choose an absolute dedicated destination without shell expansion characters.'}
 if(Test-Path -LiteralPath $destinationPath){throw 'Choose a new destination folder; existing worlds/configuration will not be overwritten.'}
 foreach($address in @($PrimaryIP,$BroadcastIP)+@($ServerAddresses.Values)){if(-not $address -or $address -notmatch '^\d{1,3}(\.\d{1,3}){3}$' -or ([Net.IPAddress]::Parse($address)).AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork){throw 'Enter valid IPv4 addresses for the primary, broadcast and every server.'}}
 $parent=Split-Path $destinationPath -Parent;[IO.Directory]::CreateDirectory($parent)|Out-Null;Assert-NNScope $parent $destinationPath|Out-Null
 $snapshot=Open-NNSnapshot $ArchivePath;$stage=$null
 try{
  $drive=[IO.DriveInfo]::new([IO.Path]::GetPathRoot($parent));if($drive.AvailableFreeSpace -lt $snapshot.expanded+256MB){throw 'Insufficient space for the verified restore.'}
  $stage=Join-Path $parent ('.minecraft-restore-'+[guid]::NewGuid().ToString('N'));[IO.Directory]::CreateDirectory($stage)|Out-Null;Set-NNPrivateDirectory $stage
  $checked=[Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase);$i=0
  foreach($file in $snapshot.manifest.files){$path=[IO.Path]::GetFullPath((Join-Path $stage $file.path));if(-not $path.StartsWith($stage+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Snapshot extraction escapes staging.'};$directory=Split-Path $path -Parent;if($checked.Add($directory)){Assert-NNScope $stage $directory|Out-Null;[IO.Directory]::CreateDirectory($directory)|Out-Null};$input=$snapshot.entries[$file.path].Open();$output=$null;try{$output=[IO.File]::Open($path,[IO.FileMode]::CreateNew,[IO.FileAccess]::Write,[IO.FileShare]::None);Copy-NNCheckedStream $input $output ([long]$file.size) $file.sha256}finally{if($output){$output.Dispose()};$input.Dispose()};$i++;if(($i%64) -eq 0 -and ('System.Windows.Forms.Application' -as [type])){[Windows.Forms.Application]::DoEvents()}}
  $sourceManifest=Join-Path $stage 'MultiServer/installation-resources.json';if(Test-Path $sourceManifest){[IO.File]::Move($sourceManifest,(Join-Path $stage 'MultiServer/source-installation-resources.json'))}
  $configPath=Join-Path $stage 'MultiServer/config.json';$cfg=Get-Content -LiteralPath $configPath -Raw -Encoding UTF8|ConvertFrom-Json
  $port=([string]$cfg.listen -split ':')[-1];if(-not $port){$port='7551'};if($port -notmatch '^\d+$' -or [int]$port -lt 1 -or [int]$port -gt 65535){throw 'Invalid discovery port.'}
  $cfg.interface_ip=$PrimaryIP;$cfg.listen=$BroadcastIP+':'+$port;$ids=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
  foreach($server in $cfg.servers){
   if(-not $server.id -or -not $ids.Add($server.id) -or -not $ServerAddresses.ContainsKey($server.id) -or -not $server.process){throw 'Every managed server needs a distinct ID and reviewed IP input.'}
   if([string]$server.network_id -notmatch '^\d+$' -or -not(Test-Path -LiteralPath (Join-Path $stage ('MultiServer/identities/'+$server.network_id+'.pem')))){throw 'Persistent server identity is missing.'}
   $backendPort=([string]$server.backend -split ':')[-1];if($backendPort -notmatch '^\d+$' -or [int]$backendPort -lt 1 -or [int]$backendPort -gt 65535){throw 'Invalid backend port.'};$server.backend=$ServerAddresses[$server.id]+':'+$backendPort
   foreach($property in @('executable','working_directory','world_directory','log_directory')){$server.process.$property=Convert-NNMigrationPath $server.process.$property $snapshot.manifest.source_root $destinationPath}
   $workRelative=$server.process.working_directory.Substring($destinationPath.Length+1);if($workRelative.Contains('\') -or $workRelative -eq 'MultiServer'){throw 'Server working folders must be separate top-level folders.'}
   foreach($property in @('executable','world_directory','log_directory')){if(-not $server.process.$property.StartsWith($server.process.working_directory+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Server process paths escape the server directory.'}}
   $properties=Join-Path (Join-Path $stage $workRelative) 'server.properties';$text=[IO.File]::ReadAllText($properties);$text=Set-NNProperty $text 'server-ip' $ServerAddresses[$server.id];[IO.File]::WriteAllText($properties,$text,[Text.UTF8Encoding]::new($false))
   foreach($property in @('executable','world_directory')){$relative=$server.process.$property.Substring($destinationPath.Length+1);if(-not(Test-Path -LiteralPath (Join-Path $stage $relative))){throw 'A configured executable or world is missing from the snapshot.'}}
  }
  Write-NNJson $configPath $cfg
  $statePath=Join-Path $stage 'MultiServer/content-staging/state.json';if(Test-Path $statePath){$state=Get-Content $statePath -Raw -Encoding UTF8|ConvertFrom-Json;foreach($pack in @($state.packs)+@($state.versions)){if($pack.PSObject.Properties['installed_path']){$pack.installed_path=Convert-NNMigrationPath $pack.installed_path $snapshot.manifest.source_root $destinationPath}};Write-NNJson $statePath $state}
  Import-NNPortableAccounts $stage;Write-NNPortableLaunchers $stage
  foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){if(-not(Test-Path -LiteralPath (Join-Path $stage ('MultiServer/'+$name)))){throw 'A required application executable is missing.'}}
  [IO.File]::WriteAllText((Join-Path $stage 'migration-activation-required'),'Complete reviewed Windows configuration before LAN advertising.')
  if(Test-Path $destinationPath){throw 'Destination appeared during restore; refusing overwrite.'};[IO.Directory]::Move($stage,$destinationPath)
  return [pscustomobject]@{Root=$destinationPath;AccountsRequiringSignIn=@((Get-Content -LiteralPath (Join-Path $destinationPath "MultiServer/account-store/accounts.json") -Raw -Encoding UTF8|ConvertFrom-Json).Accounts).Count;ActivationRequired=$true}
 }finally{$snapshot.zip.Dispose();if($stage -and (Test-Path -LiteralPath $stage)){$safe=Assert-NNScope $parent $stage;if((Split-Path $safe -Leaf) -notmatch '^\.minecraft-restore-[a-f0-9]{32}$'){throw 'Unexpected staging cleanup path.'};Remove-Item -LiteralPath $safe -Recurse -Force}}
}
function Prepare-NNNetworkConfiguration([string]$Root,$Network){
 Assert-NNNetworkPlan $Network
 $configPath=Join-Path $Root 'MultiServer/config.json';$cfg=Get-Content -LiteralPath $configPath -Raw -Encoding UTF8|ConvertFrom-Json
 if(@($cfg.servers).Count -ne @($Network.servers).Count){throw 'The network plan must include every server.'}
 $changes=[Collections.Generic.List[object]]::new();$originalConfigHash=Get-NNHash $configPath;$seen=[Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
 foreach($server in $cfg.servers){
  $selected=@($Network.servers|Where-Object {$_.id -ceq $server.id});if($selected.Count -ne 1 -or -not $seen.Add($server.id)){throw 'Server IDs are missing or ambiguous.'};$selected=$selected[0]
  $port=([string]$server.backend -split ':')[-1];if($port -ne [string]$selected.tcp_port){throw 'Preserve the configured TCP port.'}
  $work=Assert-NNScope $Root $server.process.working_directory;$path=Join-Path $work 'server.properties';$text=[IO.File]::ReadAllText($path);$values=Read-NNProperties $text
  if($values['transport'] -ne 'nethernet' -or $values['server-port'] -ne $port -or $values['server-udp-ports'] -ne $selected.udp_range){throw 'Server transport/TCP/UDP properties differ from the reviewed plan.'}
  $updated=Set-NNProperty $text 'server-ip' $selected.ip;$updated=Set-NNProperty $updated 'enable-lan-visibility' 'false'
  $changes.Add([pscustomobject]@{relative_path=($path.Substring($Root.TrimEnd('\').Length+1).Replace('\','/'));original_sha256=(Get-NNHash $path);content=$updated});$server.backend=$selected.ip+':'+$port
 }
 $cfg.interface_ip=$Network.primary_ip;$cfg.listen=$Network.broadcast+':'+$Network.discovery_port
 $result=@([pscustomobject]@{relative_path='MultiServer/config.json';original_sha256=$originalConfigHash;content=($cfg|ConvertTo-Json -Depth 100)})+@($changes.ToArray())
 return [pscustomobject]@{network=$Network;configuration=$result}
}

