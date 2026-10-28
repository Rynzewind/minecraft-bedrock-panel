[CmdletBinding()]
param([string]$PlanPath)
. (Join-Path $PSScriptRoot 'Network-Configuration.ps1')
function Get-InstallerElevationState {
    $identity=[Security.Principal.WindowsIdentity]::GetCurrent()
    [pscustomobject]@{IsAdmin=([Security.Principal.WindowsPrincipal]::new($identity)).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)}
}
function Get-NNStartupActionSpec([string]$Root,[string]$Mode) {
 if($Mode -eq 'logon'){return [pscustomobject]@{Execute=(Join-Path $Root 'MultiServer\NetherNetMultiServerAdmin.exe');Arguments='--startup';WorkingDirectory=(Join-Path $Root 'MultiServer')}}
 if($Mode -eq 'boot'){return [pscustomobject]@{Execute=(Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe');Arguments=('-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File "'+(Join-Path $Root 'Watch-NetherNet.ps1')+'"');WorkingDirectory=$Root}}
 throw 'A startup action requires logon or boot mode.'
}

function Test-NNStartupOwned([string]$Execute,[string]$Arguments,[string]$Root){
 $known=@('StartMinecraftServers.bat','StartMinecraftServers.vbs','StartMinecraftServers.ps1','Watch-NetherNet.ps1','MultiServer\NetherNetMultiServer.exe','MultiServer\NetherNetMultiServerAdmin.exe','MultiServer\Start-NetherNetMultiServer.cmd','MultiServer\Run-NetherNetMultiServer.cmd')|ForEach-Object {Join-Path $Root $_}
 $exe=[Environment]::ExpandEnvironmentVariables($Execute).Trim('"')
 if($known -contains $exe){return $true}
 $wrappers=@((Join-Path $env:SystemRoot 'System32\cmd.exe'),(Join-Path $env:SystemRoot 'System32\wscript.exe'),(Join-Path $env:SystemRoot 'System32\cscript.exe'),(Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'))
 if($exe -notin $wrappers -or $Arguments -match '[;&|]'){return $false}
 foreach($path in $known){if($Arguments -match ('(?i)(?:^|[\s"''])'+[regex]::Escape($path)+'(?:$|[\s"''])')){return $true}}
 return $false
}
function Select-NNLegacyStartupTasks([object[]]$Tasks,[string]$Root,[string]$NewTaskName){
 foreach($task in $Tasks){if($task.TaskName -eq $NewTaskName){continue};$triggers=@($task.Triggers);if(-not $triggers.Count -or @($triggers|Where-Object {$_.CimClass.CimClassName -notin @('MSFT_TaskLogonTrigger','MSFT_TaskBootTrigger')}).Count){continue};$actions=@($task.Actions);if(-not $actions.Count){continue};$owned=@($actions|Where-Object {Test-NNStartupOwned $_.Execute $_.Arguments $Root});if($owned.Count -eq $actions.Count){$task}}
}
function Restore-NNLegacyStartup([object[]]$Changes){
 foreach($change in $Changes){
  if($change.kind -eq 'task' -and $change.enabled){Enable-ScheduledTask -TaskName $change.name -TaskPath $change.path|Out-Null}
  elseif($change.kind -eq 'file' -and (Test-Path -LiteralPath $change.backup) -and -not(Test-Path -LiteralPath $change.path)){Move-Item -LiteralPath $change.backup -Destination $change.path}
  elseif($change.kind -eq 'registry'){New-ItemProperty -LiteralPath $change.path -Name $change.name -Value $change.value -PropertyType String -Force|Out-Null}
 }
}
function Retire-NNLegacyStartup([string]$Root,[string]$NewTaskName,[string]$UserSID){
 $changes=[Collections.Generic.List[object]]::new()
 $backup=Join-Path $Root ('MultiServer\backups\legacy-startup-'+[guid]::NewGuid().ToString('N'));New-Item -ItemType Directory -Path $backup -Force|Out-Null
 try{
  foreach($task in @(Select-NNLegacyStartupTasks @(Get-ScheduledTask) $Root $NewTaskName)){
   $file=Join-Path $backup (([guid]::NewGuid().ToString('N'))+'.task.xml');Export-ScheduledTask -TaskName $task.TaskName -TaskPath $task.TaskPath|Set-Content -LiteralPath $file -Encoding UTF8
   $change=[pscustomobject]@{kind='task';name=$task.TaskName;path=$task.TaskPath;enabled=($task.State -ne 'Disabled');backup=$file};$changes.Add($change)
   Disable-ScheduledTask -TaskName $task.TaskName -TaskPath $task.TaskPath|Out-Null
  }
  $profile=Get-ItemProperty -LiteralPath ('HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\'+$UserSID) -ErrorAction SilentlyContinue
  $folders=@([Environment]::GetFolderPath('CommonStartup'));if($profile){$folders+=Join-Path ([Environment]::ExpandEnvironmentVariables($profile.ProfileImagePath)) 'AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup'}
  $shell=New-Object -ComObject WScript.Shell
  foreach($folder in ($folders|Where-Object {$_}|Select-Object -Unique)){foreach($file in @(Get-ChildItem -LiteralPath $folder -File -ErrorAction SilentlyContinue)){
   $owned=$false
   if($file.Extension -eq '.lnk'){$shortcut=$shell.CreateShortcut($file.FullName);$owned=Test-NNStartupOwned $shortcut.TargetPath $shortcut.Arguments $Root}
   elseif($file.Name -in @('StartMinecraftServers.bat','StartMinecraftServers.vbs','StartMinecraftServers.ps1')){$source=Join-Path $Root $file.Name;if((Test-Path -LiteralPath $source) -and (Get-NNHash $file.FullName) -eq (Get-NNHash $source)){$owned=$true}}
   if($owned){$saved=Join-Path $backup (([guid]::NewGuid().ToString('N'))+'-'+$file.Name);$change=[pscustomobject]@{kind='file';path=$file.FullName;backup=$saved};$changes.Add($change);Move-Item -LiteralPath $file.FullName -Destination $saved}
  }}
  foreach($suffix in @('Run','RunOnce')){$key='Registry::HKEY_USERS\'+$UserSID+'\Software\Microsoft\Windows\CurrentVersion\'+$suffix;if(Test-Path -LiteralPath $key){foreach($property in (Get-ItemProperty -LiteralPath $key).PSObject.Properties){if($property.Name -like 'PS*' -or $property.Value -isnot [string]){continue};$command=[string]$property.Value;$exe='';$arguments='';if($command -match '^\s*"([^"]+)"\s*(.*)$'){$exe=$Matches[1];$arguments=$Matches[2]}elseif($command -match '^\s*(\S+)\s*(.*)$'){$exe=$Matches[1];$arguments=$Matches[2]};if(Test-NNStartupOwned $exe $arguments $Root){$change=[pscustomobject]@{kind='registry';path=$key;name=$property.Name;value=$command};$changes.Add($change);Remove-ItemProperty -LiteralPath $key -Name $property.Name}}}}
  $changes.ToArray()|ConvertTo-Json -Depth 6|Set-Content -LiteralPath (Join-Path $backup 'retired.json') -Encoding UTF8
  return $changes.ToArray()
 }catch{Restore-NNLegacyStartup $changes.ToArray();throw}
}

function Invoke-MinecraftWindowsSetup {
param([Parameter(Mandatory)][string]$PlanPath)
$ErrorActionPreference='Stop'
$p=$null
$taskName=$null
$retiredStartup=@()
function Write-Result([bool]$Success,[string]$Message='') {
    if ($p -and $p.root) {
        $result=@{success=$Success;error=$Message}|ConvertTo-Json
        [IO.File]::WriteAllText((Join-Path $p.root 'setup-result.json'),$result,[Text.UTF8Encoding]::new($false))
    }
}
try {
    if (-not (Get-InstallerElevationState).IsAdmin) {throw 'Windows configuration requires administrator access.'}
    $p=Get-Content -LiteralPath $PlanPath -Raw -Encoding UTF8|ConvertFrom-Json
    if ($p.startup -notin @('none','logon','boot')) {throw 'Invalid startup mode.'}
    $root=[IO.Path]::GetFullPath($p.root)
    if ($root -ne $p.root.TrimEnd('\')) {throw 'Use a canonical installation path.'}
    $sid=[Security.Principal.SecurityIdentifier]::new($p.user_sid)
    if ($p.group -notmatch '^NetherNet-Migration-[0-9a-f]{12}$') {throw 'Invalid managed task group.'}
    $taskName=$p.group
    $cfg=Get-Content -LiteralPath (Join-Path $root 'MultiServer\config.json') -Raw -Encoding UTF8|ConvertFrom-Json
    $programs=@((Join-Path $root 'MultiServer\NetherNetMultiServer.exe'))
    foreach($server in $cfg.servers) {
        $exe=[IO.Path]::GetFullPath($server.process.executable)
        if (-not $exe.StartsWith($root+'\',[StringComparison]::OrdinalIgnoreCase)) {throw 'Executable is outside the restored installation.'}
        $programs+=$exe
    }
    $p.firewall_programs=$programs
        $watch=Join-Path $root 'Watch-NetherNet.ps1'
    [IO.File]::WriteAllText($watch,('$ErrorActionPreference=''Stop'''+"`r`n"+'. (Join-Path $PSScriptRoot ''MultiServer\installer\setup-assets\Runtime-Operations.ps1'')'+"`r`n"+'Start-NNWatch $PSScriptRoot'),[Text.UTF8Encoding]::new($false))
    $gate=Join-Path $root 'migration-activation-required'
    [IO.File]::WriteAllText($gate,'Setup pending.')
$task=Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue; if ($task) { Disable-ScheduledTask -TaskName $taskName | Out-Null }

$dlls=@('msvcp140.dll','vcruntime140.dll','vcruntime140_1.dll')
$missing=@($dlls | Where-Object { -not (Test-Path -LiteralPath (Join-Path $env:SystemRoot ('System32\'+$_))) })
if ($missing.Count -gt 0) {
 if (-not $p.runtime_installer -or -not (Test-Path -LiteralPath $p.runtime_installer)) {throw 'Microsoft Visual C++ x64 runtime is missing. Select the official Microsoft runtime installer, or install it and retry.'}
 $signature=Get-AuthenticodeSignature -LiteralPath $p.runtime_installer
 if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch '(^|, )O=Microsoft Corporation(,|$)') {throw 'Runtime installer does not have a valid Microsoft signature.'}
 $info=(Get-Item -LiteralPath $p.runtime_installer).VersionInfo
 if ($info.ProductName -notmatch 'Visual C\+\+' -or $info.OriginalFilename -notmatch '(?i)(vc_redist|vcredist).*x64') {throw 'Choose the official Visual C++ x64 redistributable.'}
 $runtime=Start-Process -FilePath $p.runtime_installer -ArgumentList '/install /passive /norestart' -Wait -PassThru -WindowStyle Hidden
 if ($runtime.ExitCode -eq 3010 -or $runtime.ExitCode -eq 1641) {throw 'Microsoft runtime installed. Restart Windows, then retry Configure and activate.'}
 if ($runtime.ExitCode -ne 0 -and $runtime.ExitCode -ne 1638) {throw ('Microsoft runtime installer failed: '+$runtime.ExitCode)}
 foreach ($dll in $dlls) {if (-not (Test-Path -LiteralPath (Join-Path $env:SystemRoot ('System32\'+$dll)))) {throw 'Runtime remains incomplete. Install the Microsoft x64 runtime and retry.'}}
}


$cfg=Get-Content -LiteralPath (Join-Path $p.root 'MultiServer\config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$ip=Get-NetIPAddress -AddressFamily IPv4 | Where-Object IPAddress -eq $cfg.interface_ip | Select-Object -First 1
if (-not $ip) {throw 'The selected LAN address no longer exists on this PC.'}
$profile=Get-NetConnectionProfile -InterfaceIndex $ip.InterfaceIndex
if ($profile.NetworkCategory -ne 'Private') {throw 'Set the selected trusted LAN connection to Private in Windows Settings, then retry. Setup has not changed its network profile.'}
$resourceManifest=Invoke-NNNetworkSetup $p
$cfg=Get-Content -LiteralPath (Join-Path $p.root 'MultiServer\config.json') -Raw -Encoding UTF8 | ConvertFrom-Json
Get-NetFirewallRule -Group $p.group -ErrorAction SilentlyContinue | Remove-NetFirewallRule
$rules=@([pscustomobject]@{Program=$p.firewall_programs[0];Protocol='UDP';Port=$p.network.discovery_port;Address='Any'})
foreach($server in $p.network.servers){
 $configured=@($cfg.servers|Where-Object id -eq $server.id)
 if($configured.Count -ne 1){throw 'Server ID changed after preview.'}
 $rules+=[pscustomobject]@{Program=$configured[0].process.executable;Protocol='TCP';Port=$server.tcp_port;Address=$server.ip}
 $rules+=[pscustomobject]@{Program=$configured[0].process.executable;Protocol='UDP';Port=$server.udp_range;Address=$server.ip}
}
$index=0
foreach($rule in $rules){
 New-NetFirewallRule -Name ($p.group+'-'+$index) -DisplayName ($p.group+' '+[IO.Path]::GetFileName($rule.Program)+' '+$rule.Protocol) -Group $p.group -Direction Inbound -Action Allow -Program $rule.Program -Protocol $rule.Protocol -LocalPort $rule.Port -LocalAddress $rule.Address -InterfaceAlias $profile.InterfaceAlias -Profile Private -RemoteAddress LocalSubnet|Out-Null
 $index++
}
$retiredStartup=@(Retire-NNLegacyStartup $p.root $taskName $p.user_sid)
$resourceManifest|Add-Member retired_startup $retiredStartup -Force
if ($p.startup -eq 'none') {
$resourceManifest.PSObject.Properties.Remove('startup_command')
Write-NNJson (Get-NNManifestPath $p.root) $resourceManifest
if (Get-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue) { Unregister-ScheduledTask -TaskName $taskName -Confirm:$false }
} else {

$sid=[Security.Principal.SecurityIdentifier]::new($p.user_sid)
$account=$sid.Translate([Security.Principal.NTAccount]).Value
$actionSpec=Get-NNStartupActionSpec $p.root $p.startup
$action=New-ScheduledTaskAction -Execute $actionSpec.Execute -Argument $actionSpec.Arguments -WorkingDirectory $actionSpec.WorkingDirectory
$settings=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit ([TimeSpan]::Zero) -RestartCount 3 -RestartInterval (New-TimeSpan -Minutes 1) -Disable
if ($p.startup -eq 'boot') {
 $credential=Get-Credential -UserName $account -Message 'Boot startup needs your Windows account PASSWORD, not your PIN. The password is passed directly to Windows Task Scheduler.'
 if (-not $credential) {throw 'Boot credential entry cancelled.'}
 $chosen=[Security.Principal.NTAccount]::new($credential.UserName).Translate([Security.Principal.SecurityIdentifier]).Value
 if ($chosen -ne $p.user_sid) {throw 'Use the same Windows account that restored and authenticated the Minecraft accounts.'}
 $principal=New-ScheduledTaskPrincipal -UserId $account -LogonType Password -RunLevel Limited
 $trigger=New-ScheduledTaskTrigger -AtStartup; $trigger.Delay='PT30S'
 $task=New-ScheduledTask -Action $action -Trigger $trigger -Settings $settings -Principal $principal
 $password=$credential.GetNetworkCredential().Password
 try { Register-ScheduledTask -TaskName $taskName -InputObject $task -User $account -Password $password -Force | Out-Null } finally { $password=$null; $credential=$null }
} else {
 $principal=New-ScheduledTaskPrincipal -UserId $account -LogonType Interactive -RunLevel Limited
 $trigger=New-ScheduledTaskTrigger -AtLogOn -User $account
 $task=New-ScheduledTask -Action $action -Trigger $trigger -Settings $settings -Principal $principal
 Register-ScheduledTask -TaskName $taskName -InputObject $task -Force | Out-Null
}

$resourceManifest|Add-Member retired_startup $retiredStartup -Force
$resourceManifest|Add-Member startup_command ([pscustomobject]@{task_name=$taskName;execute=$actionSpec.Execute;arguments=$actionSpec.Arguments;working_directory=$actionSpec.WorkingDirectory}) -Force
Write-NNJson (Get-NNManifestPath $p.root) $resourceManifest
Enable-ScheduledTask -TaskName $taskName | Out-Null
}
        # Activation is only allowed after all reviewed aliases/configuration were applied.
    foreach($server in $p.network.servers){$assigned=@(Get-NetIPAddress -AddressFamily IPv4|Where-Object {$_.IPAddress -eq $server.ip -and $_.InterfaceIndex -eq $p.network.interface_index});if(-not $assigned.Count){throw 'A required alias disappeared before activation.'}}
    Remove-Item -LiteralPath $gate -ErrorAction Stop
    Write-Result $true
    exit 0
} catch {
    if ($retiredStartup.Count) {Restore-NNLegacyStartup $retiredStartup}
    if ($taskName) {Disable-ScheduledTask -TaskName $taskName -ErrorAction SilentlyContinue|Out-Null}
    Write-Result $false $_.Exception.Message
    exit 1
}
}
if ($MyInvocation.InvocationName -ne '.') {Invoke-MinecraftWindowsSetup -PlanPath $PlanPath}




