[CmdletBinding()]
param([string]$Root,[switch]$Elevated,[switch]$ScanOnly,[switch]$VerifyOnly,[string]$VerificationInventory)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'Network-Configuration.ps1')
function Get-NNAssociatedAppDirectories([string]$Root){
 $main=Join-Path $Root 'MultiServer\config.json'
 if(-not(Test-Path -LiteralPath $main)){return}
 $cfg=Get-Content -LiteralPath $main -Raw -Encoding UTF8|ConvertFrom-Json
 $expected=@($cfg.servers|ForEach-Object {[string]$_.network_id+'|'+[string]$_.process.working_directory}|Sort-Object)
 foreach($dir in @(Get-ChildItem -LiteralPath $Root -Directory)){
  if($dir.Name -eq 'MultiServer'){continue}
  $config=Join-Path $dir.FullName 'config.json';$helper=Join-Path $dir.FullName 'NetherNetMultiServer.exe'
  if(-not(Test-Path -LiteralPath $config) -or -not(Test-Path -LiteralPath $helper)){continue}
  Assert-NNScope $Root $dir.FullName|Out-Null
  try{$candidate=Get-Content -LiteralPath $config -Raw -Encoding UTF8|ConvertFrom-Json;$actual=@($candidate.servers|ForEach-Object {[string]$_.network_id+'|'+[string]$_.process.working_directory}|Sort-Object)
   if($expected.Count -gt 0 -and ($expected -join "`n") -eq ($actual -join "`n")){$dir.FullName}
  }catch{continue}
 }
}
function Test-NNCommandOwned([string]$Executable,[string]$Arguments,[string]$Root){
    if(-not $Executable){return $false}
    $rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
    try{$exe=[IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($Executable.Trim('"')))}catch{return $false}
    $known='^(?i)(bedrock_server|NetherNetMultiServer(?:Admin(?:-update)?|Control|Setup|-old)?)\.exe$'
    if($exe.StartsWith($rootPath+'\',[StringComparison]::OrdinalIgnoreCase) -and [IO.Path]::GetFileName($exe) -match $known){return $true}
    $scripts=@('StartMinecraftServers.bat','StartMinecraftServers.ps1','StartMinecraftServers.vbs','MultiServer\Start-NetherNetMultiServer.cmd','MultiServer\Run-NetherNetMultiServer.cmd','Watch-NetherNet.ps1')
    if([string]::IsNullOrWhiteSpace($Arguments)){foreach($script in $scripts){if($exe -eq (Join-Path $rootPath $script)){return $true}}}
    if([IO.Path]::GetFileName($exe) -notin @('cmd.exe','powershell.exe','pwsh.exe','wscript.exe','cscript.exe')){return $false}

        foreach($script in $scripts){
     $path=Join-Path $rootPath $script;$escaped=[regex]::Escape($path)
     switch([IO.Path]::GetFileName($exe).ToLowerInvariant()){
      'cmd.exe' {$pattern='(?i)^\s*(?:/d\s+)?/c\s+"*'+$escaped+'"*\s*$'}
      {$_ -in @('powershell.exe','pwsh.exe')} {$pattern='(?i)^\s*(?:(?:-NoProfile|-NoLogo|-NonInteractive|-WindowStyle\s+(?:Hidden|Normal)|-ExecutionPolicy\s+(?:Bypass|RemoteSigned))\s+)*-File\s+"?'+$escaped+'"?\s*$'}
      default {$pattern='(?i)^\s*(?:(?://B|//Nologo)\s+)*"?'+$escaped+'"?\s*$'}
     }
     if($Arguments -match $pattern){return $true}
    }
    return $false
}
function Split-NNCommand([string]$Command){
 $expanded=[Environment]::ExpandEnvironmentVariables($Command)
 if($expanded -match '^\s*"([^"]+)"(.*)$'){return @($matches[1],$matches[2])}
 if($expanded -match '^\s*(.+?\.exe)\b(.*)$'){return @($matches[1],$matches[2])}
 return @('','')
}
. (Join-Path $PSScriptRoot 'Runtime-Operations.ps1')
. (Join-Path $PSScriptRoot 'Archive-Operations.ps1')
function Get-NNUnownedAliasCandidates([object[]]$ExistingAliases,[string]$PrimaryIP){
 $adapters=@{};foreach($adapter in @(Get-NetAdapter)){$adapters[[int]$adapter.ifIndex]=[string]$adapter.InterfaceGuid}
 foreach($address in @(Get-NetIPAddress -AddressFamily IPv4|Where-Object {$_.SkipAsSource -and $_.PrefixOrigin -ne 'Dhcp' -and $_.IPAddress -ne $PrimaryIP -and $_.IPAddress -notlike '169.254.*'})){
  $guid=$adapters[[int]$address.InterfaceIndex];if(-not $guid){continue}
  if(@($ExistingAliases|Where-Object {$_.ip -eq $address.IPAddress -and ($_.adapter_guid -eq $guid -or $_.interface_index -eq $address.InterfaceIndex)}).Count){continue}
  [pscustomobject]@{ip=$address.IPAddress;owned=$false;origin='unverified-secondary-needs-confirmation';adapter_guid=$guid;interface_index=$address.InterfaceIndex;prefix_length=$address.PrefixLength}
 }
}
function Get-NNInventory([string]$Root,[switch]$AllowMissingConfiguration){
 $rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\');Assert-NNScope $rootPath $rootPath|Out-Null
 if($rootPath -eq [IO.Path]::GetPathRoot($rootPath).TrimEnd('\') -or $rootPath -eq $env:USERPROFILE -or $rootPath -eq $env:SystemRoot -or $rootPath -eq $env:ProgramData){throw 'Choose a dedicated Minecraft installation folder.'}
 $configPath=Join-Path $rootPath 'MultiServer\config.json'
 if(-not(Test-Path -LiteralPath $configPath) -and -not $AllowMissingConfiguration){throw 'Select the actual installation root containing MultiServer\config.json.'}
 $cfg=[pscustomobject]@{servers=@()}
 if(Test-Path -LiteralPath $configPath){$cfg=Get-Content -LiteralPath $configPath -Raw -Encoding UTF8|ConvertFrom-Json}
 $result=[pscustomobject]@{root=$rootPath;processes=@();tasks=@();services=@();startup=@();firewall=@();aliases=@();directories=@();root_files=@();uninstall=@();attention=@();scanned_utc=[DateTime]::UtcNow.ToString('o')}
 foreach($server in $cfg.servers){
  $dir=Assert-NNScope $rootPath $server.process.working_directory
  if((Split-Path $dir -Parent) -ne $rootPath){throw 'Legacy server directory scope is ambiguous. Manual review required.'}
  $exe=Assert-NNScope $rootPath $server.process.executable
  if([IO.Path]::GetFileName($exe) -ne 'bedrock_server.exe'){throw 'Unrecognised server executable.'}
  $result.directories+=$dir
 }
 $result.directories+=Join-Path $rootPath 'MultiServer'
 $result.directories+=@(Get-NNAssociatedAppDirectories $rootPath)
 if(Test-Path -LiteralPath (Join-Path $rootPath 'Addons')){$result.directories+=Join-Path $rootPath 'Addons'}
 foreach($name in @('StartMinecraftServers.bat','StartMinecraftServers.ps1','StartMinecraftServers.vbs','StopMinecraftServers.bat','Install-MinecraftBootTask.ps1','InstallMinecraftBootTask.cmd','Activate-MinecraftBootTask.ps1','ActivateMinecraftBootTask.cmd','Watch-NetherNet.ps1','migration-activation-required','network-maintenance','installation-retired')){if(Test-Path -LiteralPath (Join-Path $rootPath $name)){$result.root_files+=Join-Path $rootPath $name}}
 try{$result.processes=@(Get-CimInstance Win32_Process|Where-Object {Test-NNCommandOwned $_.ExecutablePath $_.CommandLine $rootPath}|Select-Object ProcessId,ExecutablePath,Name,CommandLine)}catch{$result.attention+='Process inventory unavailable: '+$_.Exception.Message}
 try{foreach($task in @(Get-ScheduledTask)){if(@($task.Actions|Where-Object {Test-NNCommandOwned $_.Execute $_.Arguments $rootPath}).Count){if(@($task.Actions|Where-Object {-not(Test-NNCommandOwned $_.Execute $_.Arguments $rootPath)}).Count){$result.attention+='Shared task requires manual review: '+$task.TaskPath+$task.TaskName;continue};$result.tasks+=[pscustomobject]@{name=$task.TaskName;path=$task.TaskPath;actions=@($task.Actions|Select-Object Execute,Arguments);state=[string]$task.State}}}}catch{$result.attention+='Task inventory unavailable: '+$_.Exception.Message}
 try{foreach($svc in @(Get-CimInstance Win32_Service)){$parts=Split-NNCommand $svc.PathName;if(Test-NNCommandOwned $parts[0] $parts[1] $rootPath){$result.services+=[pscustomobject]@{name=$svc.Name;path=$svc.PathName;state=$svc.State}}}}catch{$result.attention+='Service inventory unavailable: '+$_.Exception.Message}
 $keys=@('HKCU:\Software\Microsoft\Windows\CurrentVersion\Run','HKCU:\Software\Microsoft\Windows\CurrentVersion\RunOnce','HKLM:\Software\Microsoft\Windows\CurrentVersion\Run','HKLM:\Software\Microsoft\Windows\CurrentVersion\RunOnce','HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Run','HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\RunOnce')
 foreach($hive in @(Get-ChildItem Registry::HKEY_USERS -ErrorAction SilentlyContinue|Where-Object PSChildName -match '^S-1-5-21-[\d-]+$')){foreach($suffix in @('Run','RunOnce')){$keys+='Registry::'+$hive.Name+'\Software\Microsoft\Windows\CurrentVersion\'+$suffix}}
 foreach($key in ($keys|Select-Object -Unique)){if(Test-Path -LiteralPath $key){try{foreach($property in (Get-ItemProperty -LiteralPath $key).PSObject.Properties){if($property.Name -like 'PS*'){continue};$parts=Split-NNCommand ([string]$property.Value);if(Test-NNCommandOwned $parts[0] $parts[1] $rootPath){$result.startup+=[pscustomobject]@{kind='registry';path=$key;name=$property.Name;value=[string]$property.Value}}}}catch{$result.attention+='Startup registry inaccessible: '+$key}}}
 $folders=@([Environment]::GetFolderPath('Startup'),[Environment]::GetFolderPath('CommonStartup'),[Environment]::GetFolderPath('Desktop'),[Environment]::GetFolderPath('CommonDesktopDirectory'),[Environment]::GetFolderPath('Programs'),[Environment]::GetFolderPath('CommonPrograms'))
 try{foreach($profile in @(Get-CimInstance Win32_UserProfile|Where-Object {-not $_.Special -and $_.LocalPath})){$folders+=Join-Path $profile.LocalPath 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup';if(-not $profile.Loaded){$result.attention+='Offline user registry not scanned: '+$profile.LocalPath}}}catch{$result.attention+='Other-user profile inventory unavailable.'}
 $shell=New-Object -ComObject WScript.Shell
 foreach($folder in ($folders|Where-Object {$_}|Select-Object -Unique)){if(Test-Path -LiteralPath $folder){try{foreach($file in @(Get-ChildItem -LiteralPath $folder -File -Recurse -ErrorAction Stop)){
   if($file.Extension -eq '.lnk'){$shortcut=$shell.CreateShortcut($file.FullName);if(Test-NNCommandOwned $shortcut.TargetPath $shortcut.Arguments $rootPath){$result.startup+=[pscustomobject]@{kind='shortcut';path=$file.FullName;target=$shortcut.TargetPath;arguments=$shortcut.Arguments}}}
      elseif($file.Extension -in @('.cmd','.bat','.ps1','.vbs')){
    $text=Get-Content -LiteralPath $file.FullName -Raw
    if($text -match [regex]::Escape($rootPath+'\') -and $text -match 'NetherNetMultiServer|StartMinecraftServers'){
     $source=Join-Path $rootPath $file.Name
     if($file.Name -in @('StartMinecraftServers.vbs','StartMinecraftServers.bat','StartMinecraftServers.ps1') -and (Test-Path -LiteralPath $source) -and (Get-NNHash $file.FullName) -eq (Get-NNHash $source)){$result.startup+=[pscustomobject]@{kind='file';path=$file.FullName;source=$source;sha256=(Get-NNHash $source)}}
     else{$result.attention+='Review startup script manually: '+$file.FullName}
    }
   }
 } }catch{$result.attention+='Shortcut/startup inventory incomplete: '+$folder}}}
 try{foreach($filter in @(Get-NetFirewallApplicationFilter|Where-Object {Test-NNCommandOwned $_.Program '' $rootPath})){foreach($rule in @($filter|Get-NetFirewallRule)){$result.firewall+=[pscustomobject]@{name=$rule.Name;program=$filter.Program;group=$rule.Group}}}}catch{$result.attention+='Firewall inventory unavailable: '+$_.Exception.Message}
 $manifestPath=Get-NNManifestPath $rootPath
 if(Test-Path -LiteralPath $manifestPath){$m=Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8|ConvertFrom-Json;if($m.root -ne $rootPath -or $m.machine -ne $env:COMPUTERNAME){throw 'Manifest belongs to another machine or installation.'};$result.aliases=@($m.aliases);$result|Add-Member manifest $m}
 else{foreach($server in $cfg.servers){$address=([string]$server.backend -split ':')[0];if($address -eq $cfg.interface_ip){continue};$ip=@(Get-NetIPAddress -AddressFamily IPv4|Where-Object {$_.IPAddress -eq $address -and $_.SkipAsSource -and $_.PrefixOrigin -ne 'Dhcp'});foreach($entry in $ip){$result.aliases+=[pscustomobject]@{ip=$address;owned=$false;origin='legacy-needs-confirmation';interface_index=$entry.InterfaceIndex;prefix_length=$entry.PrefixLength}}}}
 foreach($key in @('HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall','HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall','HKLM:\Software\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall')){if(Test-Path $key){foreach($entry in @(Get-ChildItem -LiteralPath $key)){try{$value=Get-ItemProperty -LiteralPath $entry.PSPath;if($value.InstallLocation -and [IO.Path]::GetFullPath($value.InstallLocation).TrimEnd('\') -eq $rootPath -and $value.DisplayName -match 'NetherNet'){$result.uninstall+=$entry.PSPath}}catch{$result.attention+='Uninstall entry could not be read: '+$entry.PSPath}}}}
 # External application data is never guessed from a product-like name.
 foreach($base in @($env:ProgramData,$env:APPDATA,$env:LOCALAPPDATA,[IO.Path]::GetTempPath())){if($base){foreach($candidate in @(Get-ChildItem -LiteralPath $base -Directory -ErrorAction SilentlyContinue|Where-Object Name -match 'NetherNet|MinecraftServers')){$result.attention+='External data requires ownership review; retained: '+$candidate.FullName}}}
 $result.aliases+=@(Get-NNUnownedAliasCandidates -ExistingAliases $result.aliases -PrimaryIP $cfg.interface_ip)
 $result.startup=@($result.startup|Sort-Object kind,path,name -Unique)
 return $result
}
function Remove-NNVerifiedTree([string]$Root,[string]$Path){
 $path=Assert-NNScope $Root $Path
 if($path -eq $Root){throw 'Whole-root recursive deletion is prohibited.'}
 foreach($item in @(Get-ChildItem -LiteralPath $path -Force -Recurse -ErrorAction Stop)){if($item.Attributes -band [IO.FileAttributes]::ReparsePoint){throw ('Reparse point prevents deletion: '+$item.FullName)}}
 Assert-NNScope $Root $path|Out-Null
 Remove-Item -LiteralPath $path -Recurse -Force -ErrorAction Stop
}
function Invoke-NNDecommission($Inventory,[bool]$DeleteData,[string]$ReportDirectory){
 $root=$Inventory.root
 $current=Get-NNInventory $root
 $otherHelpers=@(Get-CimInstance Win32_Process|Where-Object {$_.Name -eq 'NetherNetMultiServer.exe' -and (-not $_.ExecutablePath -or -not $_.ExecutablePath.StartsWith($root+'\',[StringComparison]::OrdinalIgnoreCase))})
 if($otherHelpers.Count){throw 'Another NetherNet installation is running; the shared local administration pipe cannot be safely attributed. No resources were removed.'}
 if($current.attention.Count){throw ('Inventory has unresolved items. Resolve or review these before removal: '+($current.attention -join '; '))}
 $journal=[pscustomobject]@{root=$root;started_utc=[DateTime]::UtcNow.ToString('o');removed=@();retained=@();failed=@();reboot_verification='pending'}
 $reportPath=Join-Path $ReportDirectory 'decommission-result.json';Write-NNJson $reportPath $journal
 [IO.File]::WriteAllText((Join-Path $root 'installation-retired'),'Retired by reviewed local decommissioning.')
 foreach($task in $current.tasks){Disable-ScheduledTask -TaskName $task.name -TaskPath $task.path|Out-Null;Stop-ScheduledTask -TaskName $task.name -TaskPath $task.path -ErrorAction SilentlyContinue;Unregister-ScheduledTask -TaskName $task.name -TaskPath $task.path -Confirm:$false;$journal.removed+='Task '+$task.path+$task.name}
 foreach($svc in $current.services){Set-Service -Name $svc.name -StartupType Disabled;Stop-Service -Name $svc.name -ErrorAction Stop;& "$env:SystemRoot\System32\sc.exe" delete $svc.name|Out-Null;if($LASTEXITCODE -ne 0){throw 'Service removal failed'};$journal.removed+='Service '+$svc.name}
 foreach($entry in $current.startup){if($entry.kind -eq 'registry'){Remove-ItemProperty -LiteralPath $entry.path -Name $entry.name}else{Remove-Item -LiteralPath $entry.path -Force};$journal.removed+='Startup '+$entry.path}
 Write-NNJson $reportPath $journal
 $helper=@($current.processes|Where-Object Name -eq 'NetherNetMultiServer.exe')
 if($helper.Count){Invoke-NNAdmin 'system.shutdown'|Out-Null}
 $deadline=(Get-Date).AddSeconds(40)
 do{$remaining=@(Get-CimInstance Win32_Process|Where-Object {Test-NNCommandOwned $_.ExecutablePath $_.CommandLine $root}|Where-Object {$_.Name -in @('bedrock_server.exe','NetherNetMultiServer.exe','NetherNetMultiServer-old.exe')});if(-not $remaining.Count){break};Start-Sleep -Seconds 1}while((Get-Date) -lt $deadline)
 if($remaining.Count){throw 'A server could not be stopped cleanly. Data was retained. Stop it manually and rerun; no forced world-process termination was performed.'}
 foreach($proc in @(Get-CimInstance Win32_Process|Where-Object {Test-NNCommandOwned $_.ExecutablePath $_.CommandLine $root})){
  if($proc.Name -match '^NetherNetMultiServer(Admin(?:-update)?|Control|Setup)\.exe$'){Stop-Process -Id $proc.ProcessId -ErrorAction Stop}
 }
 foreach($rule in $current.firewall){$filter=Get-NetFirewallRule -Name $rule.name|Get-NetFirewallApplicationFilter;if(-not(Test-NNCommandOwned $filter.Program '' $root)){throw 'Firewall ownership changed during removal'};Remove-NetFirewallRule -Name $rule.name;$journal.removed+='Firewall '+$rule.name}
 if($current.PSObject.Properties['manifest']){Remove-NNOwnedAliases $root;foreach($alias in @($current.aliases|Where-Object owned)){$journal.removed+='IP '+$alias.ip}}
 foreach($alias in @($current.aliases|Where-Object {-not $_.owned})){$journal.retained+='Unowned/reused alias '+$alias.ip}
 foreach($registration in $current.uninstall){Remove-Item -LiteralPath $registration -ErrorAction Stop;$journal.removed+='Uninstall registration '+$registration}
 Write-NNJson $reportPath $journal
 $after=Get-NNInventory $root
 if($after.processes.Count -or $after.tasks.Count -or $after.services.Count -or $after.startup.Count -or $after.firewall.Count){throw 'Post-removal system inventory is not empty; inspect the report and rerun.'}
 # TCP endpoints are checked independently of process-name discovery.
 $cfg=Get-Content -LiteralPath (Join-Path $root 'MultiServer\config.json') -Raw -Encoding UTF8|ConvertFrom-Json
 foreach($s in $cfg.servers){$parts=$s.backend -split ':';if(@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue|Where-Object {$_.LocalAddress -eq $parts[0] -and $_.LocalPort -eq [int]$parts[1]}).Count){throw 'A retired server endpoint remains listening.'}}
 if($DeleteData){foreach($dir in ($current.directories|Select-Object -Unique)){if(Test-Path -LiteralPath $dir){Remove-NNVerifiedTree $root $dir;$journal.removed+='Directory '+$dir}};foreach($file in $current.root_files){if(Test-Path -LiteralPath $file){$path=Assert-NNScope $root $file;Remove-Item -LiteralPath $path -Force;$journal.removed+='File '+$path}}}
 else{$journal.retained+=$current.directories;$journal.retained+=$current.root_files}
 if($DeleteData){
  $marker=Assert-NNScope $root (Join-Path $root 'installation-retired')
  if(Test-Path -LiteralPath $marker){Remove-Item -LiteralPath $marker -Force;$journal.removed+='Retirement marker '+$marker}
  $remaining=@(Get-ChildItem -LiteralPath $root -Force)
  if(-not $remaining.Count){[IO.Directory]::Delete($root,$false);$journal.removed+='Empty installation folder '+$root}else{$journal.retained+=@($remaining|Select-Object -ExpandProperty FullName)}
 }
 $journal|Add-Member completed_utc ([DateTime]::UtcNow.ToString('o'));Write-NNJson $reportPath $journal
 return $journal
}
function Set-NNAdoptedRemovalManifest($Inventory,[string[]]$Addresses){
 $root=$Inventory.root;$manifestPath=Get-NNManifestPath $root
 $manifest=$null;if($Inventory.PSObject.Properties['manifest']){$manifest=$Inventory.manifest}
 $aliases=@($Inventory.aliases)
 foreach($address in $Addresses){
  $record=@($aliases|Where-Object ip -eq $address);if($record.Count -ne 1){throw 'Alias adoption is ambiguous.'}
  $assigned=@(Get-NetIPAddress -AddressFamily IPv4|Where-Object {$_.IPAddress -eq $address -and $_.SkipAsSource -and $_.PrefixOrigin -ne 'Dhcp'})
  if($assigned.Count -ne 1){throw 'Only an existing dedicated static secondary alias can be adopted.'}
  $primary=@(Get-NNAdapters|Where-Object {$_.interface_index -eq $assigned[0].InterfaceIndex -and -not $_.skip_as_source})
  if($primary.Count -ne 1){throw 'Select/repair the adapter primary address before adopting legacy aliases.'}
  $n=$primary[0];$baseline=Get-NNBaseline $n
  $record[0]|Add-Member adapter_guid $n.adapter_guid -Force;$record[0]|Add-Member primary_ip $n.primary_ip -Force
  $record[0].owned=$true;$record[0].origin='adopted'
  if(-not $manifest){$manifest=[pscustomobject]@{format_version=1;installation_id=[guid]::NewGuid().ToString();machine=$env:COMPUTERNAME;root=$root;network=$n;original_network=$baseline;aliases=$aliases;task_name='';firewall_group='';files=@()}}
 }
 $manifest.aliases=$aliases;Write-NNJson $manifestPath $manifest
}
function Start-NNDecommission {
 if(-not $Root){$Root=Read-Host 'Enter the original Minecraft installation folder (containing MultiServer)'}
 $Root=[IO.Path]::GetFullPath($Root).TrimEnd('\')
 $identity=[Security.Principal.WindowsIdentity]::GetCurrent();$admin=([Security.Principal.WindowsPrincipal]::new($identity)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
 if(-not $admin -and -not $ScanOnly){
  $arguments='-NoProfile -ExecutionPolicy Bypass -File "'+$PSCommandPath+'" -Root "'+$Root+'" -Elevated'
  if($VerifyOnly){$arguments+=' -VerifyOnly'}
  if($VerificationInventory){$arguments+=' -VerificationInventory "'+[IO.Path]::GetFullPath($VerificationInventory)+'"'}
  Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" -ArgumentList $arguments -Verb RunAs -Wait -WindowStyle Normal
  return
 }
 $reportDir=Join-Path ([Environment]::GetFolderPath('MyDocuments')) ('NetherNet-Decommission-'+(Get-Date -Format 'yyyyMMdd-HHmmss'))
 if($reportDir.StartsWith($Root+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Report directory must be outside the selected installation.'}
 New-Item -ItemType Directory -Path $reportDir|Out-Null
 Set-NNPrivateDirectory $reportDir
 $inventory=Get-NNInventory $Root -AllowMissingConfiguration:$VerifyOnly;Write-NNJson (Join-Path $reportDir 'inventory.json') $inventory
 Write-Host ($inventory|ConvertTo-Json -Depth 10)
 Write-Host ('Inventory saved: '+$reportDir)
 if($VerifyOnly){
  if(-not $VerificationInventory){throw 'Supply -VerificationInventory with the original saved inventory.json to verify owned IP aliases after removal.'}
  $previous=Get-Content -LiteralPath $VerificationInventory -Raw -Encoding UTF8|ConvertFrom-Json
  if($previous.root -ne $Root){throw 'Verification report belongs to a different installation.'}
  foreach($alias in @($previous.aliases|Where-Object owned)){if(@(Get-NetIPAddress -AddressFamily IPv4|Where-Object IPAddress -eq $alias.ip).Count){throw ('Owned alias remains: '+$alias.ip)}}
  if($inventory.attention.Count){throw ('Verification incomplete: '+($inventory.attention -join '; '))}
 }
 if($ScanOnly -or $VerifyOnly){if($VerifyOnly -and ($inventory.processes.Count -or $inventory.tasks.Count -or $inventory.services.Count -or $inventory.startup.Count -or $inventory.firewall.Count)){throw 'Retired components remain. Review inventory.'};return}
 if($inventory.attention.Count){throw 'Inventory has unresolved items. Review the saved report before making changes.'}
 if((Read-Host 'Have you verified the destination worlds, packs, identities AND joined BOTH destination servers? Type VERIFIED') -cne 'VERIFIED'){throw 'Destination verification is required; nothing changed.'}
 Write-Host 'Optional final snapshot: create and verify a Migration / Snapshots archive before continuing. Keep it outside the selected installation.'
 $archive=Read-Host 'Enter an existing final migration ZIP to verify, or type SKIP to retain the source data without an extra archive'
 if($archive -cne 'SKIP'){
  Test-NNSnapshotArchive ([IO.Path]::GetFullPath($archive))|Out-Null
 }
 $adoptions=@()
 foreach($alias in @($inventory.aliases|Where-Object {-not $_.owned})){
  if((Read-Host ('Existing secondary address '+$alias.ip+' is NOT owned by setup and may belong to another application. Only type that IP if you know this Minecraft installation owns it and approve removal; otherwise leave blank to retain it')) -ceq $alias.ip){$adoptions+=$alias.ip}
 }
 $mode=Read-Host 'Choose A to decommission and retain data, or B for permanent removal'
 if($mode -cnotin @('A','B')){throw 'Removal cancelled.'}
 if($mode -eq 'B'){
  if($PSScriptRoot.StartsWith($Root+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'For complete removal, copy the whole installer folder outside this installation and run its Uninstall-NetherNet.cmd there. This prevents deleting the running removal tool.'}
  if($archive -ceq 'SKIP'){throw 'Complete removal requires a verified archive outside the installation.'}
  if([IO.Path]::GetFullPath($archive).StartsWith($Root+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'The final archive must be outside deletion scope.'}
  Write-Host 'PERMANENT DATA REMOVAL: the listed server worlds, packs, configuration, account data, identities and local backups inside the verified project directories will be deleted.'
  if((Read-Host ('Type the full installation path to permanently remove the listed data: '+$Root)) -cne $Root){throw 'Permanent removal cancelled.'}
 }
 if((Read-Host 'Type DECOMMISSION to disable this installation and remove its owned IP aliases/firewall/startup resources') -cne 'DECOMMISSION'){throw 'Removal cancelled.'}
 if($adoptions.Count){Set-NNAdoptedRemovalManifest $inventory $adoptions}
 $inventory=Get-NNInventory $Root;Write-NNJson (Join-Path $reportDir 'inventory.json') $inventory
 try{$result=Invoke-NNDecommission $inventory ($mode -eq 'B') $reportDir}catch{
  $resultPath=Join-Path $reportDir 'decommission-result.json'
  if(Test-Path -LiteralPath $resultPath){$failed=Get-Content -LiteralPath $resultPath -Raw -Encoding UTF8|ConvertFrom-Json;$failed.failed+=,$_.Exception.Message;Write-NNJson $resultPath $failed}
  throw
 }
 Write-Host ('Decommissioning finished. Report: '+$reportDir)
 Write-Host 'Reboot verification remains pending. After reboot, run with -VerifyOnly -VerificationInventory pointing to the saved inventory.json. This also works after complete data removal.'
}
if($MyInvocation.InvocationName -ne '.'){
 try{Start-NNDecommission}catch{Write-Error $_ -ErrorAction Continue;Write-Host 'Removal did not complete. Review the inventory and retained data before retrying.';Read-Host 'Press Enter to close';exit 1}
 Read-Host 'Press Enter to close'
}










