[CmdletBinding()]
param([ValidateSet('Detect','Repair','Retire','Verify')][string]$Operation='Detect')
$ErrorActionPreference='Stop'
function Write-NNJson($Path,$Value) {
    $temporary=$Path+'.new'
    [IO.File]::WriteAllText($temporary,($Value|ConvertTo-Json -Depth 40),[Text.UTF8Encoding]::new($false))
    if(Test-Path -LiteralPath $Path){[IO.File]::Replace($temporary,$Path,($Path+'.previous'))}else{[IO.File]::Move($temporary,$Path)}
}
function Assert-NNScope([string]$Root,[string]$Path) {
    $r=[IO.Path]::GetFullPath($Root).TrimEnd('\');$p=[IO.Path]::GetFullPath($Path)
    if($p -ne $r -and -not $p.StartsWith($r+'\',[StringComparison]::OrdinalIgnoreCase)){throw 'Resource escapes installation scope.'}
    $cursor=$p
    while($cursor){if(Test-Path -LiteralPath $cursor){if((Get-Item -LiteralPath $cursor -Force).Attributes -band [IO.FileAttributes]::ReparsePoint){throw 'Reparse points cannot be modified.'}};$parent=Split-Path -Parent $cursor;if($parent -eq $cursor){break};$cursor=$parent}
    return $p
}
function Invoke-NNNetsh([string[]]$Arguments) {
    $output=& "$env:SystemRoot\System32\netsh.exe" @Arguments 2>&1
    if($LASTEXITCODE -ne 0){throw ('Windows network command failed: '+($output -join ' '))}
    return ($output -join "`n")
}
function Get-NNCoexistence([int]$Index,[ValidateSet('active','persistent')][string]$Store='active') {
    $text=Invoke-NNNetsh -Arguments @('interface','ipv4','show','interfaces',('interface='+$Index),'level=verbose',('store='+$Store))
    if($text -match '(?im)DHCP/Static IP coexistence\s*:\s*(enabled|disabled)'){return $matches[1]}
    return 'unknown'
}
function Set-NNCoexistence([int]$Index,[ValidateSet('enabled','disabled')][string]$Value,[ValidateSet('active','persistent')][string]$Store) {
    Invoke-NNNetsh -Arguments @('interface','ipv4','set','interface',[string]$Index,('dhcpstaticipcoexistence='+$Value),('store='+$Store))|Out-Null
    if((Get-NNCoexistence $Index $Store) -ne $Value){throw ('Windows did not apply DHCP/static coexistence in the '+$Store+' store.')}
}
function Restore-NNCoexistence([int]$Index,$Baseline) {
    # Legacy journals recorded only the active flag, and restored only the saved store.
    # Do not invent an additional active-store rollback for those journals.
    if($Baseline.PSObject.Properties['coexistence_persistent']){
        $errors=@()
        foreach($store in @('persistent','active')){
            $value=if($store -eq 'persistent'){$Baseline.coexistence_persistent}else{$Baseline.coexistence}
            if($value -in @('enabled','disabled')){try{Set-NNCoexistence $Index $value $store}catch{$errors+=$_.Exception.Message}}
        }
        if($errors.Count){throw ($errors -join '; ')}
    }elseif($Baseline.coexistence -in @('enabled','disabled')){
        Set-NNCoexistence $Index $Baseline.coexistence 'persistent'
    }
}
function Get-NNAdapters {
    $result=@()
    foreach($adapter in @(Get-NetAdapter | Where-Object Status -eq 'Up')){
        $index=[int]$adapter.ifIndex
        $iface=Get-NetIPInterface -InterfaceIndex $index -AddressFamily IPv4
        $ips=@(Get-NetIPAddress -InterfaceIndex $index -AddressFamily IPv4 | Where-Object {$_.AddressState -eq 'Preferred' -and $_.IPAddress -notlike '169.254.*' -and $_.IPAddress -ne '127.0.0.1'})
        $gateway=@(Get-NetRoute -InterfaceIndex $index -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Sort-Object RouteMetric | Select-Object -ExpandProperty NextHop)
        foreach($ip in $ips){
            $result += [pscustomobject]@{adapter_guid=[string]$adapter.InterfaceGuid;interface_index=$index;name=$adapter.Name;primary_ip=$ip.IPAddress;prefix_length=[int]$ip.PrefixLength;gateway=($gateway|Select-Object -First 1);dhcp=[string]$iface.Dhcp;skip_as_source=[bool]$ip.SkipAsSource;physical=[bool]$adapter.HardwareInterface}
        }
    }
    return $result
}
function Get-NNBaseline($Network) {
    $adapter=@(Get-NetAdapter | Where-Object {[string]$_.InterfaceGuid -eq $Network.adapter_guid -and $_.ifIndex -eq $Network.interface_index -and $_.Status -eq 'Up'})
    if($adapter.Count -ne 1){throw 'Selected network adapter changed or is unavailable. Refresh network detection.'}
    $index=[int]$Network.interface_index
    $ips=@(Get-NetIPAddress -InterfaceIndex $index -AddressFamily IPv4)
    $primary=@($ips|Where-Object {$_.IPAddress -eq $Network.primary_ip -and $_.PrefixLength -eq $Network.prefix_length -and $_.AddressState -eq 'Preferred' -and -not $_.SkipAsSource})
    if($primary.Count -ne 1){throw 'Selected primary IP changed or is a secondary address. Refresh network detection.'}
    $iface=Get-NetIPInterface -InterfaceIndex $index -AddressFamily IPv4
    $routes=@(Get-NetRoute -InterfaceIndex $index -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Select-Object NextHop,RouteMetric)
    $dns=@((Get-DnsClientServerAddress -InterfaceIndex $index).ServerAddresses)
    $coexistence=Get-NNCoexistence $index
    [pscustomobject]@{adapter_guid=$Network.adapter_guid;interface_index=$index;primary_ip=$Network.primary_ip;prefix_length=$Network.prefix_length;dhcp=[string]$iface.Dhcp;addresses=@($ips|Select-Object IPAddress,PrefixLength,SkipAsSource,PrefixOrigin);routes=$routes;dns=$dns;coexistence=$coexistence;coexistence_persistent=(Get-NNCoexistence $index 'persistent')}
}
function Test-NNConflict($Network,[string]$Address) {
    & "$env:SystemRoot\System32\PING.EXE" -4 -S $Network.primary_ip -n 1 -w 600 $Address *> $null
    $responded=$LASTEXITCODE -eq 0
    $neighbor=@(Get-NetNeighbor -InterfaceIndex $Network.interface_index -AddressFamily IPv4 -IPAddress $Address -ErrorAction SilentlyContinue | Where-Object {$_.State -in @('Reachable','Stale','Delay','Probe','Permanent') -and $_.LinkLayerAddress -notmatch '^(00[-:]?){5}00$'})
    return ($responded -or $neighbor.Count -gt 0)
}
function Add-NNAlias($Network,[string]$Address) {
    Invoke-NNNetsh -Arguments @('interface','ipv4','add','address',('name='+$Network.interface_index),('address='+$Address+'/'+$Network.prefix_length),'store=persistent','skipassource=true')|Out-Null
}
function Remove-NNAlias($Network,[string]$Address) {
    # Revalidate primary and adapter immediately before removing an owned alias.
    $baseline=Get-NNBaseline $Network
    if($Address -eq $baseline.primary_ip -or @($baseline.routes|Where-Object NextHop -eq $Address).Count){throw 'Refusing to remove a primary or gateway address.'}
    $ip=@(Get-NetIPAddress -InterfaceIndex $Network.interface_index -AddressFamily IPv4 | Where-Object IPAddress -eq $Address)
    if($ip.Count -eq 0){return}
    if(@($ip|Where-Object {-not $_.SkipAsSource -or $_.PrefixOrigin -eq 'Dhcp'}).Count){throw 'Address is no longer a dedicated static secondary alias; manual review required.'}
    Invoke-NNNetsh -Arguments @('interface','ipv4','delete','address',('name='+$Network.interface_index),('address='+$Address),'store=persistent')|Out-Null
    if(@(Get-NetIPAddress -InterfaceIndex $Network.interface_index -AddressFamily IPv4 | Where-Object IPAddress -eq $Address).Count){throw ('Windows did not remove '+$Address)}
}
function Assert-NNBaseline($Network,$Before) {
    $after=Get-NNBaseline $Network
    if($after.dhcp -ne $Before.dhcp -or (($after.routes|ConvertTo-Json -Compress) -ne ($Before.routes|ConvertTo-Json -Compress)) -or (($after.dns|ConvertTo-Json -Compress) -ne ($Before.dns|ConvertTo-Json -Compress))){throw 'Primary DHCP, gateway or DNS settings changed unexpectedly. Rolling back aliases.'}
}
function Get-NNHash([string]$Path){$stream=[IO.File]::OpenRead($Path);$sha=[Security.Cryptography.SHA256]::Create();try{return ([BitConverter]::ToString($sha.ComputeHash($stream))).Replace('-','').ToLowerInvariant()}finally{$stream.Dispose();$sha.Dispose()}}
function Get-NNManifestPath([string]$Root){return (Join-Path $Root 'MultiServer\installation-resources.json')}
function Restore-NNTransaction($Root,$Journal) {
    $errors=@()
    foreach($file in @($Journal.files)){
        try{$path=Assert-NNScope $Root (Join-Path $Root $file.relative_path);$backup=Assert-NNScope $Root $file.backup
            if((Get-NNHash $backup) -ne $file.original_sha256){throw 'Configuration backup hash mismatch.'}
            [IO.File]::Copy($backup,$path,$true)
        }catch{$errors+=$_.Exception.Message}
    }
    foreach($address in @($Journal.created)) {try{Remove-NNAlias $Journal.network $address}catch{$errors+=$_.Exception.Message}}
    if($Journal.baseline.dhcp -eq 'Enabled'){
        try{if((Get-NetIPInterface -InterfaceIndex $Journal.network.interface_index -AddressFamily IPv4).Dhcp -ne 'Enabled'){Set-NetIPInterface -InterfaceIndex $Journal.network.interface_index -AddressFamily IPv4 -Dhcp Enabled}
            Restore-NNCoexistence $Journal.network.interface_index $Journal.baseline
        }catch{$errors+=$_.Exception.Message}
    }
    if($errors.Count){throw ('Rollback needs attention: '+($errors -join '; '))}
}
function Assert-NNNetworkPlan($Network){
 if($Network.prefix_length -lt 1 -or $Network.prefix_length -gt 30 -or $Network.discovery_port -lt 1 -or $Network.discovery_port -gt 65535){throw 'Invalid LAN prefix or discovery port.'}
 $primary=[Net.IPAddress]::Parse($Network.primary_ip).GetAddressBytes();if($primary.Length -ne 4){throw 'IPv4 is required.'}
 $seen=@{};$ranges=@();$expectedBroadcast=[byte[]]::new(4)
 for($i=0;$i -lt 4;$i++){$bits=[Math]::Max(0,[Math]::Min(8,$Network.prefix_length-8*$i));$mask=if($bits -eq 0){0}else{256-[Math]::Pow(2,8-$bits)};$expectedBroadcast[$i]=[byte](($primary[$i] -band [int]$mask) -bor (255-[int]$mask))}
 if(([Net.IPAddress]::new($expectedBroadcast)).ToString() -ne $Network.broadcast){throw 'Broadcast differs from the selected subnet.'}
 foreach($server in $Network.servers){
  $address=[Net.IPAddress]::Parse($server.ip).GetAddressBytes();if($address.Length -ne 4 -or $seen.ContainsKey($server.ip)){throw 'Server IPs must be distinct IPv4 addresses.'};$seen[$server.ip]=$true
  $hostZero=$true
  for($i=0;$i -lt 4;$i++){$bits=[Math]::Max(0,[Math]::Min(8,$Network.prefix_length-8*$i));$mask=if($bits -eq 0){0}else{256-[Math]::Pow(2,8-$bits)};if(($address[$i] -band [int]$mask) -ne ($primary[$i] -band [int]$mask)){throw 'Server address is outside the selected subnet.'};if(($address[$i] -band (255-[int]$mask)) -ne 0){$hostZero=$false}}
  if($hostZero -or $server.ip -in @($Network.primary_ip,$Network.gateway,$Network.broadcast)){throw 'Server address is primary, gateway, network or broadcast.'}
  if($server.tcp_port -lt 1 -or $server.tcp_port -gt 65535 -or $server.udp_range -notmatch '^(\d+)-(\d+)$'){throw 'Invalid server ports.'}
  $lo=[int]$matches[1];$hi=[int]$matches[2];if($lo -lt 1 -or $hi -gt 65535 -or $lo -gt $hi -or ($lo -le $Network.discovery_port -and $Network.discovery_port -le $hi)){throw 'Invalid or conflicting UDP range.'}
  foreach($range in $ranges){if($lo -le $range.hi -and $range.lo -le $hi){throw 'UDP ranges overlap.'}};$ranges+=[pscustomobject]@{lo=$lo;hi=$hi}
 }
}
function Set-NNPrivateDirectory([string]$Path){
 $identity=[Security.Principal.WindowsIdentity]::GetCurrent().User
 $acl=[Security.AccessControl.DirectorySecurity]::new();$acl.SetAccessRuleProtection($true,$false)
 foreach($sid in @($identity,[Security.Principal.SecurityIdentifier]::new('S-1-5-18'),[Security.Principal.SecurityIdentifier]::new('S-1-5-32-544'))){$rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow');$acl.AddAccessRule($rule)}
 [IO.Directory]::SetAccessControl($Path,$acl)
}
function Assert-NNNoPortConflicts($Network){
 $tcp=@(Get-NetTCPConnection -State Listen -ErrorAction Stop);$udp=@(Get-NetUDPEndpoint -ErrorAction Stop)
 foreach($server in $Network.servers){
  if(@($tcp|Where-Object {$_.LocalPort -eq $server.tcp_port -and $_.LocalAddress -in @('0.0.0.0','::',$server.ip)}).Count){throw ('TCP port is already occupied: '+$server.ip+':'+$server.tcp_port)}
  $ports=$server.udp_range -split '-';if(@($udp|Where-Object {$_.LocalPort -ge [int]$ports[0] -and $_.LocalPort -le [int]$ports[1] -and $_.LocalAddress -in @('0.0.0.0','::',$server.ip)}).Count){throw 'Gameplay UDP range is already occupied.'}
 }
 if(@($udp|Where-Object {$_.LocalPort -eq $Network.discovery_port -and $_.LocalAddress -in @('0.0.0.0',$Network.primary_ip)}).Count){throw 'IPv4 discovery port is already occupied.'}
}function Invoke-NNNetworkSetup($Plan) {
    $root=[IO.Path]::GetFullPath($Plan.root).TrimEnd('\');$n=$Plan.network
    if(-not $n){throw 'A reviewed network plan is required. Refresh the installer.'}
    Assert-NNScope $root $root|Out-Null
    $journalPath=Join-Path $root 'MultiServer\network-transaction.json'
    if(Test-Path -LiteralPath $journalPath){$old=Get-Content -LiteralPath $journalPath -Raw -Encoding UTF8|ConvertFrom-Json;if($old.state -notin @('committed','rolled_back')){Restore-NNTransaction $root $old;$old.state='rolled_back';Write-NNJson $journalPath $old;throw 'An interrupted network change was rolled back. Review the settings and retry.'}}
    Assert-NNNetworkPlan $n
        $active=@(Get-CimInstance Win32_Process|Where-Object { $_.ExecutablePath -and $_.ExecutablePath.StartsWith($root+'\',[StringComparison]::OrdinalIgnoreCase) -and $_.Name -in @('bedrock_server.exe','NetherNetMultiServer.exe','NetherNetMultiServer-old.exe')})
    if($active.Count){throw 'Installation server processes must be stopped cleanly before changing network configuration.'}
    Assert-NNNoPortConflicts $n
    $before=Get-NNBaseline $n
    $manifestPath=Get-NNManifestPath $root
    $manifest=$null
    if(Test-Path -LiteralPath $manifestPath){$manifest=Get-Content -LiteralPath $manifestPath -Raw -Encoding UTF8|ConvertFrom-Json;if($manifest.root -ne $root -or $manifest.format_version -ne 1){throw 'Installation manifest does not match this installation.'}}
    if(-not $manifest){$manifest=[pscustomobject]@{format_version=1;installation_id=[guid]::NewGuid().ToString();machine=$env:COMPUTERNAME;root=$root;network=$n;original_network=$before;aliases=@();task_name=$Plan.group;startup=$Plan.startup;firewall_group=$Plan.group;files=@();created_utc=[DateTime]::UtcNow.ToString('o')}}
    if($manifest.machine -ne $env:COMPUTERNAME){throw 'Source resource manifest cannot be used on the destination. Restore through the migration installer.'}
    $all=@(Get-NetIPAddress -AddressFamily IPv4)
    $new=@();$aliases=@($manifest.aliases)
    foreach($server in @($n.servers)){
        if($server.ip -eq $n.primary_ip -or $server.ip -eq $n.gateway -or $server.ip -eq $n.broadcast){throw 'Server aliases cannot use the primary, gateway or broadcast address.'}
        $existing=@($all|Where-Object IPAddress -eq $server.ip)
        if($existing.Count){if(@($existing|Where-Object {$_.InterfaceIndex -ne $n.interface_index -or -not $_.SkipAsSource -or $_.PrefixOrigin -eq 'Dhcp'}).Count){throw 'Requested IP is assigned locally outside the dedicated static aliases on the selected adapter.'}}
        elseif(Test-NNConflict $n $server.ip){throw ('Address is already occupied: '+$server.ip)}else{$new+=$server.ip}
        $record=@($aliases|Where-Object {$_.ip -eq $server.ip -and $_.adapter_guid -eq $n.adapter_guid})
        if(-not $record.Count){$aliases += [pscustomobject]@{ip=$server.ip;adapter_guid=$n.adapter_guid;interface_index=$n.interface_index;prefix_length=$n.prefix_length;primary_ip=$n.primary_ip;owned=($existing.Count -eq 0 -or [bool]$server.adopt);origin=$(if($existing.Count -eq 0){'created'}elseif($server.adopt){'adopted'}else{'reused'})}}
        elseif($server.adopt){$record[0].owned=$true;if($record[0].origin -eq 'reused'){$record[0].origin='adopted'}}
    }
    $backupDir=Join-Path $root ('MultiServer\backups\network-'+[guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $backupDir -Force|Out-Null
    Set-NNPrivateDirectory $backupDir
    $files=@();$allowed=@('MultiServer/config.json')
    $cfg=Get-Content -LiteralPath (Join-Path $root 'MultiServer\config.json') -Raw -Encoding UTF8|ConvertFrom-Json
    foreach($s in $cfg.servers){$work=Assert-NNScope $root $s.process.working_directory;$allowed+=($work.Substring($root.Length+1).Replace('\','/')+'/server.properties')}
    foreach($change in @($Plan.configuration)){
        if($change.relative_path -notin $allowed){throw 'Unexpected configuration target in network plan.'}
        $path=Assert-NNScope $root (Join-Path $root $change.relative_path)
        if((Get-NNHash $path) -ne $change.original_sha256){throw 'Configuration changed after preview. Refresh and review the plan again.'}
        $backup=Join-Path $backupDir ([guid]::NewGuid().ToString('N')+'.bak');[IO.File]::Copy($path,$backup,$false)
        $files += [pscustomobject]@{relative_path=$change.relative_path;backup=$backup;original_sha256=$change.original_sha256}
    }
    if($files.Count -ne $allowed.Count){throw 'Network plan is missing configuration files.'}
    $journal=[pscustomobject]@{state='prepared';network=$n;baseline=$before;created=@();files=$files}
    Write-NNJson $journalPath $journal
    try{
        if($before.dhcp -eq 'Enabled' -and $new.Count){
            if($before.coexistence -notin @('enabled','disabled') -or $before.coexistence_persistent -notin @('enabled','disabled')){throw 'This Windows version/language cannot safely verify DHCP coexistence in both stores. No primary address was converted. Configure secondary addresses manually or use a separately approved static primary configuration.'}
            Set-NNCoexistence $n.interface_index 'enabled' 'persistent'
            Set-NNCoexistence $n.interface_index 'enabled' 'active'
            Assert-NNBaseline $n $before
        }
        foreach($address in $new){
            $journal.created+=,$address;Write-NNJson $journalPath $journal
            Add-NNAlias $n $address
            $deadline=(Get-Date).AddSeconds(15)
            do{$assigned=@(Get-NetIPAddress -InterfaceIndex $n.interface_index -AddressFamily IPv4|Where-Object {$_.IPAddress -eq $address -and $_.AddressState -eq 'Preferred' -and $_.SkipAsSource});if($assigned.Count){break};Start-Sleep -Milliseconds 300}while((Get-Date) -lt $deadline)
            if(-not $assigned.Count){throw ('Windows did not assign a usable static alias: '+$address)}
            $persistent=@(Get-NetIPAddress -InterfaceIndex $n.interface_index -AddressFamily IPv4 -PolicyStore PersistentStore | Where-Object IPAddress -eq $address)
            if(-not $persistent.Count){throw 'Alias is not persistent across reboot.'}
        }
        Assert-NNBaseline $n $before
        foreach($change in $Plan.configuration){$path=Assert-NNScope $root (Join-Path $root $change.relative_path);$tmp=$path+'.network-new';[IO.File]::WriteAllText($tmp,$change.content,[Text.UTF8Encoding]::new($false));[IO.File]::Replace($tmp,$path,($path+'.network-previous'))}
        $manifest.network=$n;$manifest.aliases=$aliases;$manifest.files=$files
        $manifest|Add-Member startup $Plan.startup -Force
        Write-NNJson $manifestPath $manifest
        $journal.state='committed';Write-NNJson $journalPath $journal
        return $manifest
    }catch{
        $reason=$_.Exception.Message
        try{Restore-NNTransaction $root $journal;$journal.state='rolled_back';Write-NNJson $journalPath $journal}catch{throw ($reason+'; '+$_.Exception.Message)}
        throw $reason
    }
}
function Remove-NNOwnedAliases($Root) {
    $path=Get-NNManifestPath $Root
    if(-not(Test-Path -LiteralPath $path)){throw 'No installation resource manifest. Legacy aliases need explicit ownership review; none were removed.'}
    $m=Get-Content -LiteralPath $path -Raw -Encoding UTF8|ConvertFrom-Json
    if($m.root -ne [IO.Path]::GetFullPath($Root).TrimEnd('\') -or $m.machine -ne $env:COMPUTERNAME -or $m.format_version -ne 1){throw 'Resource manifest belongs to a different installation or PC.'}
    foreach($alias in @($m.aliases|Where-Object owned)){
        if($alias.origin -notin @('created','adopted')){throw 'Unverified alias ownership.'}
        $primary=@(Get-NNAdapters|Where-Object {$_.adapter_guid -eq $alias.adapter_guid -and -not $_.skip_as_source})
        if($primary.Count -ne 1){throw 'The alias adapter needs a unique current primary address. Repair network configuration before removal.'}
        $network=$primary[0]
        Remove-NNAlias $network $alias.ip
    }
    if($m.original_network.dhcp -eq 'Enabled' -and $m.original_network.coexistence -in @('enabled','disabled')){
        $baseline=Get-NNBaseline $m.network
        $remaining=@(Get-NetIPAddress -InterfaceIndex $m.network.interface_index -AddressFamily IPv4|Where-Object {$_.SkipAsSource -and $_.PrefixOrigin -ne 'Dhcp'})
        if(-not $remaining.Count -and $baseline.dhcp -eq 'Enabled'){
            Restore-NNCoexistence $m.network.interface_index $m.original_network
            Assert-NNBaseline $m.network $baseline
        }
    }
}
if($MyInvocation.InvocationName -ne '.'){
    if($Operation -eq 'Detect'){@(Get-NNAdapters)|ConvertTo-Json -Depth 8;exit 0}
    throw 'Use the setup/uninstaller to review and approve changes.'
}






