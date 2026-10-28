param([string]$FixtureRoot,[string]$Scenario)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Network-Configuration.ps1')
$script:fixtureRoot=$FixtureRoot;$script:scenario=$Scenario;$script:dhcp=if($Scenario -like 'dhcp*'){'Enabled'}else{'Disabled'}
$script:ips=@([pscustomobject]@{IPAddress='10.42.8.9';PrefixLength=20;InterfaceIndex=9;SkipAsSource=$false;AddressState='Preferred';PrefixOrigin='Manual'})
$script:removed=@();$script:added=@();$script:coexist=if($Scenario -eq 'dhcp-mixed-reverse'){'enabled'}else{'disabled'}
$script:coexistPersistent=if($Scenario -in @('dhcp-mixed','dhcp-mixed-failure')){'enabled'}else{'disabled'}
if($Scenario -eq 'dhcp-unsupported'){$script:coexistPersistent='unknown'}
function Set-NNPrivateDirectory {param($Path)}
function Get-NetTCPConnection {[CmdletBinding()]param($State)}
function Get-NetUDPEndpoint {[CmdletBinding()]param()}
function Get-CimInstance {param($ClassName)}
function Get-NetAdapter {[pscustomobject]@{InterfaceGuid='adapter';ifIndex=9;Status='Up'}}
function Get-NetIPAddress {[CmdletBinding()]param($InterfaceIndex,$AddressFamily,$PolicyStore);return $script:ips}
function Get-NetIPInterface {param($InterfaceIndex,$AddressFamily);[pscustomobject]@{Dhcp=$script:dhcp}}
function Set-NetIPInterface {param($InterfaceIndex,$AddressFamily,$Dhcp);$script:dhcp=$Dhcp}
function Get-NetRoute {[CmdletBinding()]param($InterfaceIndex,$DestinationPrefix);[pscustomobject]@{NextHop='10.42.8.1';RouteMetric=25}}
function Get-DnsClientServerAddress {param($InterfaceIndex);[pscustomobject]@{ServerAddresses=@('10.42.8.1')}}
function Test-NNConflict {param($Network,$Address);return ($script:scenario -eq 'conflict')}
function Invoke-NNNetsh {param([string[]]$Arguments)
 $persistent=$Arguments -contains 'store=persistent'
 if($Arguments -contains 'show'){$value=if($persistent){$script:coexistPersistent}else{$script:coexist};return ('DHCP/Static IP coexistence : '+$value)}
 if($Arguments -contains 'dhcpstaticipcoexistence=enabled'){if($persistent){$script:coexistPersistent='enabled'}else{$script:coexist='enabled'}}
 if($Arguments -contains 'dhcpstaticipcoexistence=disabled'){if($persistent){$script:coexistPersistent='disabled'}else{$script:coexist='disabled'}}
 if($Arguments -contains 'delete'){$ip=($Arguments|Where-Object {$_ -like 'address=*'}).Substring(8);$script:removed+=$ip;$script:ips=@($script:ips|Where-Object IPAddress -ne $ip)}
}
function Add-NNAlias {param($Network,$Address)
 if($script:dhcp -eq 'Enabled' -and ($script:coexist -ne 'enabled' -or $script:coexistPersistent -ne 'enabled')){throw 'DHCP alias attempted before both active and persistent coexistence were enabled'}
 if($script:scenario -in @('dhcp-failure','dhcp-mixed-failure')){throw 'simulated DHCP address failure'}
 if($script:scenario -eq 'failure'){throw 'simulated address failure'}
 $script:added+=$Address;$script:ips+= [pscustomobject]@{IPAddress=$Address;PrefixLength=20;InterfaceIndex=9;SkipAsSource=$true;AddressState='Preferred';PrefixOrigin='Manual'}
 if($script:scenario -eq 'write-failure'){[IO.File]::Delete((Join-Path $script:fixtureRoot 'Creative\server.properties'))}
}
New-Item -ItemType Directory -Force (Join-Path $FixtureRoot 'MultiServer'),(Join-Path $FixtureRoot 'Creative')|Out-Null
$props=Join-Path $FixtureRoot 'Creative\server.properties';$original='server-ip=old';[IO.File]::WriteAllText($props,$original)
$cfg=@{servers=@(@{process=@{working_directory=(Join-Path $FixtureRoot 'Creative')}})}
$cfgPath=Join-Path $FixtureRoot 'MultiServer\config.json';Write-NNJson $cfgPath $cfg
$n=[pscustomobject]@{adapter_guid='adapter';interface_index=9;primary_ip='10.42.8.9';prefix_length=20;gateway='10.42.8.1';broadcast='10.42.15.255';discovery_port=7551;servers=@([pscustomobject]@{id='creative';tcp_port=19134;udp_range='49152-49200';ip='10.42.9.246';adopt=($Scenario -eq 'adopt')})}
if($Scenario -in @('reuse','adopt','reuse-adopt-later')){$script:ips+=[pscustomobject]@{IPAddress='10.42.9.246';PrefixLength=20;InterfaceIndex=9;SkipAsSource=$true;AddressState='Preferred';PrefixOrigin='Manual'}}
$p=[pscustomobject]@{root=$FixtureRoot;group='NetherNet-Migration-012345abcdef';startup='none';network=$n;configuration=@([pscustomobject]@{relative_path='MultiServer/config.json';original_sha256=(Get-NNHash $cfgPath);content=([IO.File]::ReadAllText($cfgPath))},[pscustomobject]@{relative_path='Creative/server.properties';original_sha256=(Get-NNHash $props);content='server-ip=10.42.9.246'})}
$failed=$false
try{$m=Invoke-NNNetworkSetup $p}catch{$failed=$true;if($Scenario -notin @('conflict','failure','write-failure','dhcp-failure','dhcp-mixed-failure','dhcp-unsupported')){throw}}
if($Scenario -in @('conflict','failure','write-failure','dhcp-failure','dhcp-mixed-failure','dhcp-unsupported')){
 if(-not $failed){throw 'failure scenario succeeded'}
 if([IO.File]::ReadAllText($props) -ne $original){throw 'original properties were not restored'}
 if(@($script:ips|Where-Object IPAddress -eq '10.42.9.246').Count){throw 'failed assignment leaked alias'}
 if($Scenario -eq 'dhcp-failure' -and ($script:coexist -ne 'disabled' -or $script:coexistPersistent -ne 'disabled')){throw 'rollback leaked DHCP coexistence changes'}
 if($Scenario -eq 'dhcp-mixed-failure' -and ($script:coexist -ne 'disabled' -or $script:coexistPersistent -ne 'enabled')){throw 'rollback changed independent initial coexistence states'}
 if($Scenario -eq 'dhcp-unsupported'){
  $journal=Get-Content -LiteralPath (Join-Path $FixtureRoot 'MultiServer/network-transaction.json') -Raw|ConvertFrom-Json
  if($journal.state -ne 'rolled_back' -or $script:added.Count){throw 'unsupported detection trapped recovery or attempted aliases'}
  $retryFailed=$false
  try{Invoke-NNNetworkSetup $p|Out-Null}catch{$retryFailed=$true;if($_.Exception.Message -notmatch 'cannot safely verify'){throw}}
  if(-not $retryFailed){throw 'unsupported detection accepted on retry'}
 }
 exit 0
}
if($failed){throw 'unexpected failure'}
if($Scenario -eq 'reuse-adopt-later'){
 $n.servers[0].adopt=$true
 foreach($change in $p.configuration){$change.original_sha256=Get-NNHash (Join-Path $FixtureRoot $change.relative_path)}
 $m=Invoke-NNNetworkSetup $p
 Remove-NNOwnedAliases $FixtureRoot
 if($script:removed -notcontains '10.42.9.246'){throw 'later adoption was not removable'}
}
if($Scenario -eq 'reuse' -and $m.aliases[0].owned){throw 'preexisting alias was silently adopted'}
if($Scenario -ne 'reuse' -and -not $m.aliases[0].owned){throw 'created/adopted alias not owned'}
if($Scenario -in @('dhcp','dhcp-mixed','dhcp-mixed-reverse')){
 if($script:dhcp -ne 'Enabled'){throw 'DHCP disabled'}
 Remove-NNOwnedAliases $FixtureRoot
 $expectedPersistent=if($Scenario -eq 'dhcp-mixed'){'enabled'}else{'disabled'}
 $expectedActive=if($Scenario -eq 'dhcp-mixed-reverse'){'enabled'}else{'disabled'}
 if($script:coexist -ne $expectedActive -or $script:coexistPersistent -ne $expectedPersistent -or $script:dhcp -ne 'Enabled'){throw 'uninstall failed to restore independent original DHCP coexistence states'}
}
if($Scenario -eq 'primary-protection'){
 $m.aliases[0].ip='10.42.8.9';Write-NNJson (Get-NNManifestPath $FixtureRoot) $m
 try{Remove-NNOwnedAliases $FixtureRoot;throw 'primary removal did not fail'}catch{if($_.Exception.Message -notmatch 'primary'){throw}}
 if($script:removed -contains '10.42.8.9'){throw 'primary removed'}
}elseif($Scenario -eq 'uninstall'){
 Remove-NNOwnedAliases $FixtureRoot
 if($script:removed -notcontains '10.42.9.246'){throw 'owned alias was not removed'}
 if(-not @($script:ips|Where-Object IPAddress -eq '10.42.8.9').Count){throw 'primary removed'}
}
Write-Output ('PASS '+$Scenario)




