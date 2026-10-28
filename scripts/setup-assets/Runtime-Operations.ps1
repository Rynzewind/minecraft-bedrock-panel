[CmdletBinding()]
param([ValidateSet('Report')][string]$Operation,[Alias('Root')][string]$RuntimeRoot)
function Invoke-NNAdmin([string]$Method,[hashtable]$Parameters=@{}){
 $pipe=[IO.Pipes.NamedPipeClientStream]::new('.','NetherNetMultiServer.Admin.v1',[IO.Pipes.PipeDirection]::InOut)
 try{$pipe.Connect(1500);$writer=[IO.BinaryWriter]::new($pipe);$reader=[IO.BinaryReader]::new($pipe);$b=[Text.Encoding]::UTF8.GetBytes((@{id='network-maintenance';method=$Method;params=$Parameters}|ConvertTo-Json -Compress));$writer.Write([uint32]$b.Length);$writer.Write($b);$writer.Flush();$size=$reader.ReadUInt32();if($size -gt 1048576){throw 'Unexpected admin response size'};$r=[Text.Encoding]::UTF8.GetString($reader.ReadBytes($size))|ConvertFrom-Json;if($r.error){throw $r.error.message};return $r.result}finally{$pipe.Dispose()}
}
function Get-NNRuntimeReport([string]$Root){
 $cfg=Get-Content -LiteralPath (Join-Path $Root 'MultiServer\config.json') -Raw -Encoding UTF8|ConvertFrom-Json
 $checks=@();$processes=@(Get-CimInstance Win32_Process);$listeners=@(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue)
 $addresses=@(Get-NetIPAddress -AddressFamily IPv4|Select-Object -ExpandProperty IPAddress)
 $checks+=[pscustomobject]@{check='primary address';status=$(if($addresses -contains $cfg.interface_ip){'passed'}else{'failed'});detail=$cfg.interface_ip}
 foreach($s in $cfg.servers){
  $parts=$s.backend -split ':';$owned=@($processes|Where-Object {[string]::Equals($_.ExecutablePath,$s.process.executable,[StringComparison]::OrdinalIgnoreCase)})
  $listener=@($listeners|Where-Object {$_.LocalAddress -eq $parts[0] -and $_.LocalPort -eq [int]$parts[1] -and $owned.ProcessId -contains $_.OwningProcess})
  $reachable=$false;$client=[Net.Sockets.TcpClient]::new([Net.IPEndPoint]::new([Net.IPAddress]::Parse($cfg.interface_ip),0))
  try{$task=$client.ConnectAsync($parts[0],[int]$parts[1]);if($task.Wait(1500)){$reachable=$client.Connected}}catch{}finally{$client.Dispose()}
  $checks+=[pscustomobject]@{check=($s.id+' assigned IP');status=$(if($addresses -contains $parts[0]){'passed'}else{'failed'});detail=$parts[0]}
  $checks+=[pscustomobject]@{check=($s.id+' owned TCP listener and backend connection');status=$(if($owned.Count -eq 1 -and $listener.Count -gt 0 -and $reachable){'passed'}else{'failed'});detail=$s.backend}
 }
 $helperPath=Join-Path $Root 'MultiServer\NetherNetMultiServer.exe';$helper=@($processes|Where-Object {[string]::Equals($_.ExecutablePath,$helperPath,[StringComparison]::OrdinalIgnoreCase)})
 $discoveryPort=[int](($cfg.listen -split ':')[-1]);$udp=@(Get-NetUDPEndpoint -ErrorAction SilentlyContinue|Where-Object {$_.LocalAddress -in @('0.0.0.0',$cfg.interface_ip) -and $_.LocalPort -eq $discoveryPort -and $helper.ProcessId -contains $_.OwningProcess})
 $checks+=[pscustomobject]@{check='helper discovery socket';status=$(if($helper.Count -eq 1 -and $udp.Count){'passed'}else{'failed'});detail=$cfg.listen}
 $checks+=[pscustomobject]@{check='Xbox joins and independent LAN discovery';status='manual';detail='Join each server from a console to verify end-to-end discovery/authentication.'}
 return $checks
}
function Test-NNHelperStoppedCleanly($Process){
 try{return ($Process.HasExited -and $Process.ExitCode -eq 0)}catch{return $false}
}
function Start-NNWatch([string]$Root){
 $ErrorActionPreference='Stop';$multi=Join-Path $Root 'MultiServer';$helper=Join-Path $multi 'NetherNetMultiServer.exe';$cfg=Get-Content -LiteralPath (Join-Path $multi 'config.json') -Raw -Encoding UTF8|ConvertFrom-Json
 $log=Join-Path $multi 'startup-readiness.log';$bootDeadline=(Get-Date).AddMinutes(3)
 function Write-WatchLog([string]$Message){Add-Content -LiteralPath $log -Value ((Get-Date -Format o)+' '+$Message)}
 while($true){
  foreach($marker in @('installation-retired','network-maintenance','migration-activation-required')){if(Test-Path -LiteralPath (Join-Path $Root $marker)){Write-WatchLog ('Startup paused: '+$marker);return}}
  $ips=@(Get-NetIPAddress -AddressFamily IPv4|Select-Object -ExpandProperty IPAddress);$required=@($cfg.interface_ip)+@($cfg.servers|ForEach-Object {($_.backend -split ':')[0]})
  if(@($required|Where-Object {$_ -notin $ips}).Count){Write-WatchLog 'Configured IPs unavailable. Waiting for the selected adapter; use Repair Network Configuration if addresses changed.';if((Get-Date) -gt $bootDeadline){throw 'Network readiness timed out.'};Start-Sleep -Seconds 3;continue}
  $running=@(Get-CimInstance Win32_Process|Where-Object {[string]::Equals($_.ExecutablePath,$helper,[StringComparison]::OrdinalIgnoreCase)})
   $watchedHelper=if(-not $running.Count){Start-Process -FilePath $helper -WorkingDirectory $multi -WindowStyle Hidden -RedirectStandardOutput (Join-Path $multi 'helper-runtime.log') -RedirectStandardError (Join-Path $multi 'helper-errors.log') -PassThru}else{[Diagnostics.Process]::GetProcessById([int]$running[0].ProcessId)}
  $null=$watchedHelper.Handle
  $ready=$false
  for($i=0;$i -lt 60;$i++){try{$statuses=@(Invoke-NNAdmin 'servers.list');$ready=$true;break}catch{Start-Sleep -Seconds 1}}
  if(-not $ready){throw 'Helper administration did not become ready.'}
  foreach($s in $statuses){if($s.State -in @('stopped','failed')){Invoke-NNAdmin 'server.start' @{server_id=$s.ID}|Out-Null}elseif($s.State -notin @('running','starting')){throw 'Unmanaged or unstable server state prevents safe startup.'}}
  Start-Sleep -Seconds 3
  $report=@(Get-NNRuntimeReport $Root);$failed=@($report|Where-Object status -eq 'failed')
  if(-not $failed.Count){Write-WatchLog 'Both owned TCP endpoints and discovery socket verified.';break}
  if((Get-Date) -gt $bootDeadline){Write-WatchLog ($failed|ConvertTo-Json -Compress);throw 'Endpoint readiness timed out.'}
 }
 # Remain attached so Task Scheduler can observe a crash and apply its recovery policy.
 while($true){
  Start-Sleep -Seconds 5
  foreach($marker in @('installation-retired','network-maintenance')){if(Test-Path -LiteralPath (Join-Path $Root $marker)){Write-WatchLog ('Watchdog stopped: '+$marker);return}}
    try{$statuses=@(Invoke-NNAdmin 'servers.list')}catch{
   if(Test-NNHelperStoppedCleanly $watchedHelper){Write-WatchLog 'Helper exited cleanly; automatic recovery is not needed.';return}
   throw
  }
  if(@($statuses|Where-Object State -eq 'failed').Count){throw 'A managed server crashed. Task Scheduler may retry startup.'}
  $helperProcess=@(Get-CimInstance Win32_Process|Where-Object {[string]::Equals($_.ExecutablePath,$helper,[StringComparison]::OrdinalIgnoreCase)})
  if(-not $helperProcess.Count){if(Test-NNHelperStoppedCleanly $watchedHelper){return};throw 'Discovery helper exited unexpectedly.'}
 }
}
if($MyInvocation.InvocationName -ne '.' -and $Operation -eq 'Report'){@(Get-NNRuntimeReport $RuntimeRoot)|ConvertTo-Json -Depth 8}


