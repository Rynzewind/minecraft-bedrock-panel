[CmdletBinding()]
param([string]$RepairRoot,[switch]$SmokeTest)
$ErrorActionPreference='Stop'
$script:WizardAssets=$PSScriptRoot
$script:DistributionRoot=Split-Path $PSScriptRoot -Parent
. (Join-Path $script:WizardAssets 'Network-Configuration.ps1')
. (Join-Path $script:WizardAssets 'Runtime-Operations.ps1')
. (Join-Path $script:WizardAssets 'Install-Tools.ps1')
. (Join-Path $script:WizardAssets 'Archive-Operations.ps1')
Add-Type -AssemblyName System.Windows.Forms,System.Drawing
$script:WizardFields=@{};$script:WizardBusy=$false;$script:WizardRows=@();$script:RepairMode=[bool]$RepairRoot
function Set-NNWizardStatus([string]$Message){$script:WizardFields.status.Text=$Message;[Windows.Forms.Application]::DoEvents()}
function Invoke-NNArchive([string[]]$Arguments){
 switch($Arguments[0]){
  '--restore-network' {if($Arguments.Count -ne 7){throw 'Invalid restore arguments.'};return (Restore-NNSnapshot -ArchivePath $Arguments[1] -Destination $Arguments[2] -PrimaryIP $Arguments[3] -BroadcastIP $Arguments[4] -ServerAddresses @{creative=$Arguments[5];survival=$Arguments[6]})}
  '--verify' {return (Test-NNSnapshotArchive $Arguments[1])}
  '--plan' {$network=Get-Content -LiteralPath $Arguments[1] -Raw -Encoding UTF8|ConvertFrom-Json;$plan=Prepare-NNNetworkConfiguration $Arguments[2] $network;Write-NNJson $Arguments[3] $plan;return}
  default {throw 'Unsupported archive operation. Use the updated administration panel to create migration snapshots.'}
 }
}
function Get-NNWizardPackage {
 $manifestPath=Join-Path $script:DistributionRoot 'release-hashes.json'
 $hashes=Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8|ConvertFrom-Json
 $companions=$script:DistributionRoot
 if(-not(Test-Path -LiteralPath (Join-Path $companions 'NetherNetMultiServer.exe'))){$companions=Split-Path $script:DistributionRoot -Parent}
 $pairs=@()
 foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe','setup-assets/Configure-Windows.ps1','setup-assets/Network-Configuration.ps1','setup-assets/Runtime-Operations.ps1','setup-assets/Decommission-NetherNet.ps1','setup-assets/Install-Tools.ps1','setup-assets/Setup-Wizard.ps1','setup-assets/Archive-Operations.ps1')){
  $expected=$hashes.PSObject.Properties[$name].Value
  $base=if($name -in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){$companions}else{$script:DistributionRoot}
  if(-not $expected -or (Get-NNHash (Join-Path $base $name)) -ne $expected){throw ('Release checksum failed: '+$name+'. Keep the complete extracted installer folder together.')}
  if($base -eq $companions -and $name -like 'NetherNetMultiServer*'){$pairs+=$name+':'+$expected}
 }
 if($hashes.PSObject.Properties['NetherNetOnline.exe']){$expected=$hashes.PSObject.Properties['NetherNetOnline.exe'].Value;if((Get-NNHash (Join-Path $companions 'NetherNetOnline.exe')) -ne $expected){throw 'Online companion checksum failed.'};$pairs+='NetherNetOnline.exe:'+$expected}
 [pscustomobject]@{companions=$companions;hashes=($pairs -join ',')}
}
function Install-NNWizardTools([string]$Root,$Package){
 Invoke-NNInstallTools -Root $Root -SourceDirectory $script:DistributionRoot -CompanionDirectory $Package.companions -ScriptOnly -CompanionHashes $Package.hashes
}
function Get-NNWizardRoot {
 $root=[IO.Path]::GetFullPath($script:WizardFields.root.Text.Trim()).TrimEnd('\')
 if($root -eq [IO.Path]::GetPathRoot($root).TrimEnd('\')){throw 'Choose a dedicated server folder, not a drive root.'}
 return $root
}
function Get-NNWizardAdapter {
 $index=$script:WizardFields.adapter.SelectedIndex
 if($index -lt 0 -or $index -ge $script:WizardRows.Count){throw 'Select an active IPv4 LAN adapter.'}
 return $script:WizardRows[$index]
}
function Get-NNBroadcast([string]$Address,[int]$Prefix){
 $bytes=[Net.IPAddress]::Parse($Address).GetAddressBytes();if($bytes.Length -ne 4 -or $Prefix -lt 1 -or $Prefix -gt 30){throw 'The selected adapter needs a usable IPv4 subnet.'}
 for($i=0;$i -lt 4;$i++){$bits=[Math]::Max(0,[Math]::Min(8,$Prefix-$i*8));$mask=if($bits -eq 0){0}else{256-[Math]::Pow(2,8-$bits)};$bytes[$i]=[byte](($bytes[$i] -band [int]$mask) -bor (255-[int]$mask))}
 return ([Net.IPAddress]::new($bytes)).ToString()
}
function Update-NNWizardAdapter {
 if(-not $script:WizardFields.creative){return}
 try{$n=Get-NNWizardAdapter;$broadcast=Get-NNBroadcast $n.primary_ip $n.prefix_length;$script:WizardFields.info.Text=('Primary '+$n.primary_ip+'/'+$n.prefix_length+' | Gateway '+$n.gateway+' | DHCP '+$n.dhcp+' | Broadcast '+$broadcast)
  $bytes=[Net.IPAddress]::Parse($broadcast).GetAddressBytes();$suggestions=@()
  for($attempt=0;$attempt -lt 8 -and $suggestions.Count -lt 2;$attempt++){for($i=3;$i -ge 0;$i--){if($bytes[$i] -gt 0){$bytes[$i]--;break};$bytes[$i]=255};$candidate=([Net.IPAddress]::new($bytes)).ToString();if($candidate -notin @($n.primary_ip,$n.gateway)){$suggestions+=$candidate}}
  if($suggestions.Count -eq 2){$script:WizardFields.creative.Text=$suggestions[1];$script:WizardFields.survival.Text=$suggestions[0]}
 }catch{$script:WizardFields.info.Text=$_.Exception.Message}
}
function Refresh-NNWizardAdapters {
 $script:WizardRows=@(Get-NNAdapters|Where-Object {-not $_.skip_as_source})
 $script:WizardFields.adapter.Items.Clear()
 foreach($n in $script:WizardRows){$script:WizardFields.adapter.Items.Add(($n.name+' - '+$n.primary_ip+'/'+$n.prefix_length))|Out-Null}
 if($script:WizardRows.Count){$script:WizardFields.adapter.SelectedIndex=0}
}
function Get-NNWizardNetwork([string]$Root){
 $n=Get-NNWizardAdapter;$cfg=Get-Content -LiteralPath (Join-Path $Root 'MultiServer/config.json') -Raw -Encoding UTF8|ConvertFrom-Json
 $servers=@()
 foreach($server in $cfg.servers){
  if($server.id -notin @('creative','survival')){throw 'This wizard expects the existing Creative and Survival server IDs.'}
  $properties=[IO.File]::ReadAllText((Join-Path $server.process.working_directory 'server.properties'))
  $range=[regex]::Match($properties,'(?m)^server-udp-ports\s*=\s*([^\r\n]+)').Groups[1].Value.Trim()
  $servers+=[pscustomobject]@{id=$server.id;ip=$script:WizardFields[$server.id].Text.Trim();tcp_port=[int](($server.backend -split ':')[-1]);udp_range=$range;adopt=$script:WizardFields.adopt.Checked}
 }
 [pscustomobject]@{adapter_guid=$n.adapter_guid;interface_index=$n.interface_index;primary_ip=$n.primary_ip;prefix_length=$n.prefix_length;gateway=$n.gateway;broadcast=(Get-NNBroadcast $n.primary_ip $n.prefix_length);discovery_port=[int](($cfg.listen -split ':')[-1]);servers=$servers}
}
function Stop-NNWizardInstallation([string]$Root){
 $otherHelpers=@(Get-CimInstance Win32_Process|Where-Object {$_.Name -eq 'NetherNetMultiServer.exe' -and $_.ExecutablePath -ne (Join-Path $Root 'MultiServer/NetherNetMultiServer.exe')})
 if($otherHelpers.Count){throw 'Another NetherNet installation is running; local administration ownership is ambiguous. No processes were stopped.'}
 $owned=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($Root+'\',[StringComparison]::OrdinalIgnoreCase) -and $_.Name -in @('bedrock_server.exe','NetherNetMultiServer.exe')})
 $helper=@($owned|Where-Object {$_.ExecutablePath -eq (Join-Path $Root 'MultiServer/NetherNetMultiServer.exe')})
 $running=@()
 if($owned.Count -and -not $helper.Count){throw 'Stop externally running Bedrock servers cleanly before configuration.'}
 if($helper.Count){$statuses=@(Invoke-NNAdmin 'servers.list');if(@($statuses|Where-Object {$_.State -notin @('running','stopped','failed')}).Count){throw 'A snapshot or unstable/external server state prevents maintenance.'};$running=@($statuses|Where-Object State -eq 'running'|ForEach-Object {$_.ID})
  [IO.File]::WriteAllText((Join-Path $Root 'network-maintenance'),'Reviewed network maintenance.')
  try{Invoke-NNAdmin 'system.shutdown'|Out-Null}catch{Remove-Item -LiteralPath (Join-Path $Root 'network-maintenance') -ErrorAction SilentlyContinue;throw}
  $deadline=(Get-Date).AddSeconds(45)
  do{$left=@(Get-CimInstance Win32_Process|Where-Object {$_.ExecutablePath -and $_.ExecutablePath.StartsWith($Root+'\',[StringComparison]::OrdinalIgnoreCase) -and $_.Name -in @('bedrock_server.exe','NetherNetMultiServer.exe')});if(-not $left.Count){break};Start-Sleep -Milliseconds 300}while((Get-Date) -lt $deadline)
  if($left.Count){throw 'The servers did not stop cleanly; no network changes were applied.'}
 }
 return $running
}
function Start-NNWizardRuntime([string]$Root,[string[]]$IDs,[bool]$StartAll){
 if(Test-Path -LiteralPath (Join-Path $Root 'installation-retired')){throw 'This installation is retired.'}
 $multi=Join-Path $Root 'MultiServer';$path=Join-Path $multi 'NetherNetMultiServer.exe'
  $allHelpers=@(Get-CimInstance Win32_Process|Where-Object Name -eq 'NetherNetMultiServer.exe')
 if(@($allHelpers|Where-Object {$_.ExecutablePath -ne $path}).Count){throw 'Another NetherNet installation is running; this wizard will not connect to or control it.'}
 $helper=@($allHelpers|Where-Object ExecutablePath -eq $path)
 if(-not $helper.Count){Start-Process -FilePath $path -WorkingDirectory $multi -WindowStyle Hidden -RedirectStandardOutput (Join-Path $multi 'helper-runtime.log') -RedirectStandardError (Join-Path $multi 'helper-errors.log')|Out-Null}
 $ready=$false
 for($i=0;$i -lt 40;$i++){try{$statuses=@(Invoke-NNAdmin 'servers.list');$ready=$true;break}catch{[Windows.Forms.Application]::DoEvents();Start-Sleep -Milliseconds 500}}
 if(-not $ready){throw 'Helper administration did not become ready.'}
 foreach($s in $statuses){if(($StartAll -or $IDs -contains $s.ID) -and $s.State -in @('stopped','failed')){Invoke-NNAdmin 'server.start' @{server_id=$s.ID}|Out-Null}}
}
function Get-NNRestoreSummary([string]$ArchivePath){
 $manifest=Test-NNSnapshotArchive $ArchivePath
 $worlds=@($manifest.files|ForEach-Object {if($_.path -match '^[^/]+/worlds/([^/]+)/'){ $Matches[1] }}|Sort-Object -Unique)
 $packCount=0;$enabled=0;$disabled=0;$pending=0
 $archive=[IO.Compression.ZipFile]::OpenRead([IO.Path]::GetFullPath($ArchivePath))
 try{$entry=$archive.GetEntry('MultiServer/content-staging/state.json');if($entry){if($entry.Length -gt 16MB){throw 'Addon selection state is too large to preview.'};$reader=[IO.StreamReader]::new($entry.Open());try{$state=$reader.ReadToEnd()|ConvertFrom-Json}finally{$reader.Dispose()};$packCount=@($state.packs).Count;$pending=@($state.pending).Count;foreach($server in $state.enabled.PSObject.Properties){foreach($selection in $server.Value.PSObject.Properties){if($selection.Value -eq $true){$enabled++}else{$disabled++}}}}}finally{$archive.Dispose()}
 return ('Verified saved setup: '+@($manifest.files).Count+' files'+"`r`nWorlds: "+($worlds -join ', ')+"`r`nAddons: "+$packCount+' library pack(s), '+$enabled+' enabled and '+$disabled+' disabled server selections, '+$pending+' pending change(s).'+"`r`nServer-installed addon files and world selections are restored as saved. Original downloads are included when present in the snapshot.`r`nRestore creates a fresh server folder; it never overwrites an existing world.")
}
function Confirm-NNRestorePlan([string]$Summary){return ([Windows.Forms.MessageBox]::Show($Summary+"`r`n`r`nRestore this saved setup now?",'Review saved servers and addons','YesNo','Question') -eq 'Yes')}
function Update-NNWizardMode {
 if(-not $script:WizardFields.ContainsKey('mode')){return}
 $restore=($script:WizardFields.mode.SelectedIndex -eq 0 -and -not $script:RepairMode)
 foreach($name in @('archive','archiveBrowse','preview','restore')){if($script:WizardFields.ContainsKey($name)){$script:WizardFields[$name].Enabled=$restore}}
}

function Invoke-NNWizardAction([string]$Action){
 if($script:WizardBusy){return};$script:WizardBusy=$true
 foreach($control in $script:WizardFields.form.Controls){$control.Enabled=$false};$script:WizardFields.status.Enabled=$true
 try{
  $root=Get-NNWizardRoot;$package=Get-NNWizardPackage
  if($Action -eq 'preview'){
   Set-NNWizardStatus (Get-NNRestoreSummary $script:WizardFields.archive.Text.Trim())
  }elseif($Action -eq 'restore'){
   $restoreSummary=Get-NNRestoreSummary $script:WizardFields.archive.Text.Trim()
   Set-NNWizardStatus $restoreSummary
   if(-not (Confirm-NNRestorePlan $restoreSummary)){return}
   $n=Get-NNWizardAdapter;Set-NNWizardStatus 'Verifying and restoring the snapshot. No servers will be started.'
   Invoke-NNArchive -Arguments @('--restore-network',$script:WizardFields.archive.Text.Trim(),$root,$n.primary_ip,(Get-NNBroadcast $n.primary_ip $n.prefix_length),$script:WizardFields.creative.Text.Trim(),$script:WizardFields.survival.Text.Trim())|Out-Null
   Install-NNWizardTools $root $package
   Set-NNWizardStatus ($restoreSummary+"

Servers, addons and saved selections restored. Bindings match the entered addresses; complete Configure Windows before starting.")
  }elseif($Action -eq 'panel'){
   Start-NNWizardRuntime $root @() $false
   Start-Process -FilePath (Join-Path $root 'MultiServer/NetherNetMultiServerAdmin.exe') -WorkingDirectory (Join-Path $root 'MultiServer')|Out-Null
   Set-NNWizardStatus 'Administration opened. Its lifecycle controls will start the helper and servers quietly.'
  }else{
   if(-not $script:WizardFields.reserved.Checked -or -not $script:WizardFields.stopped.Checked){throw 'Confirm router DHCP exclusions/reservations and that the old deployment is stopped.'}
   $network=Get-NNWizardNetwork $root
   $input=Join-Path $root ('MultiServer/.network-input-'+[guid]::NewGuid().ToString('N')+'.json');$output=$input+'.plan.json'
   try{
    Write-NNJson $input $network;Invoke-NNArchive -Arguments @('--plan',$input,$root,$output)|Out-Null
    $plan=Get-Content -LiteralPath $output -Raw -Encoding UTF8|ConvertFrom-Json
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $sha=[Security.Cryptography.SHA256]::Create();try{$hash=([BitConverter]::ToString($sha.ComputeHash([Text.Encoding]::UTF8.GetBytes($root.ToLowerInvariant())))).Replace('-','').ToLowerInvariant()}finally{$sha.Dispose()}
    $startup=@('none','logon','boot')[$script:WizardFields.startup.SelectedIndex]
    foreach($property in @{root=$root;user_sid=$sid;startup=$startup;runtime_installer=$script:WizardFields.runtime.Text.Trim();group=('NetherNet-Migration-'+$hash.Substring(0,12));firewall_programs=@()}.GetEnumerator()){$plan|Add-Member $property.Key $property.Value -Force}
    Write-NNJson $output $plan
    $preview='Adapter: '+(Get-NNWizardAdapter).name+"`r`nPrimary retained: "+$network.primary_ip+'/'+$network.prefix_length+"`r`nBroadcast: "+$network.broadcast+':'+$network.discovery_port
    foreach($s in $network.servers){$preview+="`r`n"+$s.id+': '+$s.ip+':'+$s.tcp_port+' UDP '+$s.udp_range}
    $preview+="`r`n`r`nRunning managed servers will stop cleanly. Approve persistent static aliases, configuration updates, scoped firewall rules and the selected startup mode?"
    if([Windows.Forms.MessageBox]::Show($preview,'Review network configuration','YesNo','Question') -ne 'Yes'){Set-NNWizardStatus 'Configuration cancelled; no network changes applied.';return}
    Set-NNWizardStatus 'Stopping this installation cleanly, then applying the reviewed Windows configuration...'
    $running=@(Stop-NNWizardInstallation $root)
    Install-NNWizardTools $root $package
    $arguments='-NoProfile -ExecutionPolicy Bypass -File "'+(Join-Path $script:WizardAssets 'Configure-Windows.ps1')+'" -PlanPath "'+$output+'"'
    $elevated=[Diagnostics.ProcessStartInfo]::new();$elevated.FileName="$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe";$elevated.Arguments=$arguments;$elevated.UseShellExecute=$true;$elevated.Verb='runas';$elevated.WindowStyle='Hidden'
    $p=[Diagnostics.Process]::Start($elevated)
    try{while(-not $p.WaitForExit(150)){[Windows.Forms.Application]::DoEvents()};$p.WaitForExit();$exitCode=$p.ExitCode}finally{$p.Dispose()}
    if($exitCode -ne 0){$result=Get-Content -LiteralPath (Join-Path $root 'setup-result.json') -Raw -Encoding UTF8|ConvertFrom-Json;throw $result.error}
    Remove-Item -LiteralPath (Join-Path $root 'network-maintenance') -ErrorAction SilentlyContinue
    Start-NNWizardRuntime $root $running (-not $script:RepairMode)
    $summary='Static aliases and all configuration files were applied. Stopped servers remain stopped during repair.'
    if(-not $script:RepairMode -or $running.Count){for($attempt=0;$attempt -lt 35;$attempt++){$report=@(Get-NNRuntimeReport $root);$failed=@($report|Where-Object status -eq 'failed');if(-not $failed.Count){break};[Windows.Forms.Application]::DoEvents();Start-Sleep -Seconds 1};$summary=($report|ForEach-Object {$_.status+': '+$_.check+' - '+$_.detail}) -join "`r`n";if($failed.Count){throw ('Endpoint validation needs attention: '+$summary)}}
    Set-NNWizardStatus ($summary+"`r`n`r`nReauthenticate accounts as needed and verify both console joins. Reboot persistence still needs an actual restart check.")
   }finally{Remove-Item -LiteralPath $input,$output,($input+'.previous'),($output+'.previous') -ErrorAction SilentlyContinue}
  }
 }catch{Set-NNWizardStatus ('Needs attention: '+$_.Exception.Message+"`r`nConfiguration backups/journals are retained. Correct the issue and retry; do not restore over existing worlds.")}
 finally{$script:WizardBusy=$false;foreach($control in $script:WizardFields.form.Controls){$control.Enabled=$true};if($script:RepairMode){$script:WizardFields.restore.Enabled=$false};Update-NNWizardMode}
}
function New-NNSetupWindow {
 $form=[Windows.Forms.Form]::new();$form.Text='NetherNet - Static IP Setup and Migration';$form.Size=[Drawing.Size]::new(920,760);$form.MinimumSize=[Drawing.Size]::new(840,700);$form.StartPosition='CenterScreen';$script:WizardFields.form=$form
 function Add-Field([string]$Name,[string]$Type,[string]$Text,[int]$X,[int]$Y,[int]$Width,[int]$Height=25){$control=New-Object ('System.Windows.Forms.'+$Type);$control.Text=$Text;$control.Location=[Drawing.Point]::new($X,$Y);$control.Size=[Drawing.Size]::new($Width,$Height);$form.Controls.Add($control);$script:WizardFields[$Name]=$control;return $control}
 Add-Field 'archiveLabel' 'Label' 'Snapshot ZIP:' 15 18 110|Out-Null;Add-Field 'archive' 'TextBox' '' 130 15 640|Out-Null
 $browse=Add-Field 'archiveBrowse' 'Button' 'Browse...' 780 14 105;$browse.Add_Click({$d=[Windows.Forms.OpenFileDialog]::new();$d.Filter='Migration snapshots (*.zip)|*.zip';if($d.ShowDialog() -eq 'OK'){$script:WizardFields.archive.Text=$d.FileName};$d.Dispose()})
 Add-Field 'rootLabel' 'Label' 'Server folder:' 15 58 110|Out-Null;Add-Field 'root' 'TextBox' 'C:\MinecraftServers' 130 55 640|Out-Null
 $choose=Add-Field 'rootBrowse' 'Button' 'Choose...' 780 54 105;$choose.Add_Click({$d=[Windows.Forms.FolderBrowserDialog]::new();$d.Description='Choose a parent folder; restore will create MinecraftServers inside it.';if($d.ShowDialog() -eq 'OK'){$script:WizardFields.root.Text=Join-Path $d.SelectedPath 'MinecraftServers'};$d.Dispose()})
 Add-Field 'adapterLabel' 'Label' 'LAN adapter:' 15 98 110|Out-Null;$combo=Add-Field 'adapter' 'ComboBox' '' 130 95 640;$combo.DropDownStyle='DropDownList';$combo.Add_SelectedIndexChanged({Update-NNWizardAdapter})
 $refresh=Add-Field 'refresh' 'Button' 'Refresh' 780 94 105;$refresh.Add_Click({try{Refresh-NNWizardAdapters}catch{Set-NNWizardStatus $_.Exception.Message}})
 Add-Field 'info' 'Label' 'Select the active Ethernet adapter. The PC primary IP, gateway and DNS are retained.' 15 132 870 40|Out-Null
 Add-Field 'creativeLabel' 'Label' 'Creative static IP:' 15 180 130|Out-Null;Add-Field 'creative' 'TextBox' '' 150 177 250|Out-Null
 Add-Field 'survivalLabel' 'Label' 'Survival static IP:' 450 180 135|Out-Null;Add-Field 'survival' 'TextBox' '' 590 177 250|Out-Null
 Add-Field 'reserved' 'CheckBox' 'I reviewed these IPs and reserved/excluded them from the router DHCP pool. Suggestions are provisional.' 15 216 870|Out-Null
 Add-Field 'adopt' 'CheckBox' 'Also remove these server aliases on uninstall if they already exist (explicit ownership adoption).' 15 246 870|Out-Null
 Add-Field 'runtimeLabel' 'Label' 'Optional VC++ x64 installer:' 15 290 180|Out-Null;Add-Field 'runtime' 'TextBox' '' 200 287 570|Out-Null
 $runtimeBrowse=Add-Field 'runtimeBrowse' 'Button' 'Browse...' 780 286 105;$runtimeBrowse.Add_Click({$d=[Windows.Forms.OpenFileDialog]::new();$d.Filter='Microsoft VC++ x64 installer (*.exe)|*.exe';if($d.ShowDialog() -eq 'OK'){$script:WizardFields.runtime.Text=$d.FileName};$d.Dispose()})
 Add-Field 'startupLabel' 'Label' 'Startup:' 15 330 110|Out-Null;$startup=Add-Field 'startup' 'ComboBox' '' 130 327 755;$startup.DropDownStyle='DropDownList';$startup.Items.AddRange(@('Manual start only','Start panel in tray and all servers at Windows sign-in','Start at Windows boot without interactive login (Windows password required)'));$startup.SelectedIndex=1
 Add-Field 'stopped' 'CheckBox' 'The previous deployment is stopped; this PC may use the transferred server IPs and identities.' 15 365 870|Out-Null
 $previewButton=Add-Field 'preview' 'Button' 'Preview saved setup' 15 405 170 32;$previewButton.Add_Click({Invoke-NNWizardAction 'preview'});$restore=Add-Field 'restore' 'Button' 'Restore servers + addons' 195 405 210 32;$restore.Add_Click({Invoke-NNWizardAction 'restore'})
 $configure=Add-Field 'configure' 'Button' 'Configure Windows / activate' 420 405 300 32;$configure.Add_Click({Invoke-NNWizardAction 'configure'})
 $panel=Add-Field 'panel' 'Button' 'Open panel' 735 405 150 32;$panel.Add_Click({Invoke-NNWizardAction 'panel'})
 $status=Add-Field 'status' 'TextBox' 'Restore to a fresh folder, review the active adapter and static server IPs, then configure Windows. No network changes occur until you confirm the preview and Windows UAC prompt.' 15 455 870 245;$status.Multiline=$true;$status.ReadOnly=$true;$status.ScrollBars='Vertical';$status.Anchor='Top,Bottom,Left,Right'
 $form.Add_FormClosing({param($sender,$eventArgs);if($script:WizardBusy){$eventArgs.Cancel=$true}})
 $form.Height+=40;$form.MinimumSize=[Drawing.Size]::new(840,740)
 foreach($control in $form.Controls){$control.Top+=40};$status.Height=245
 $mode=Add-Field 'mode' 'ComboBox' '' 15 8 870 28;$mode.DropDownStyle='DropDownList';$mode.Items.AddRange(@('Restore previously saved servers, worlds and addons from a snapshot','Configure an existing installation / repair network'));$mode.SelectedIndex=0;$mode.Add_SelectedIndexChanged({Update-NNWizardMode});Update-NNWizardMode
 return $form
}
if($MyInvocation.InvocationName -eq '.') {return}
$form=New-NNSetupWindow
if($SmokeTest){try{if(-not $script:WizardFields.creative -or -not $script:WizardFields.survival -or $script:WizardFields.adopt.Checked -or $script:WizardFields.reserved.Checked){throw 'Missing or implicitly approved IP controls'};Write-Output 'Native setup wizard controls verified.'}finally{$form.Dispose()};exit 0}
try{
 Refresh-NNWizardAdapters
 if($RepairRoot){$script:WizardFields.root.Text=$RepairRoot;$script:WizardFields.restore.Enabled=$false;$form.Text='NetherNet - Repair Network Configuration';$cfg=Get-Content -LiteralPath (Join-Path $RepairRoot 'MultiServer/config.json') -Raw -Encoding UTF8|ConvertFrom-Json
  for($i=0;$i -lt $script:WizardRows.Count;$i++){if($script:WizardRows[$i].primary_ip -eq $cfg.interface_ip){$script:WizardFields.adapter.SelectedIndex=$i}}
  foreach($s in $cfg.servers){if($script:WizardFields.ContainsKey($s.id)){$script:WizardFields[$s.id].Text=($s.backend -split ':')[0]}}
  $manifest=Join-Path $RepairRoot 'MultiServer/installation-resources.json';if(Test-Path $manifest){$saved=Get-Content $manifest -Raw|ConvertFrom-Json;$index=@('none','logon','boot').IndexOf($saved.startup);if($index -ge 0){$script:WizardFields.startup.SelectedIndex=$index}}
 }
}catch{Set-NNWizardStatus $_.Exception.Message}
[Windows.Forms.Application]::EnableVisualStyles();$form.ShowDialog()|Out-Null;$form.Dispose()







