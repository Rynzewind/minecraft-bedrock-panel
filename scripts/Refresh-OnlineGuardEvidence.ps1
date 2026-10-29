$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

# This task is deliberately read-only: it records the effective firewall
# snapshot for the unprivileged NetherNet helper. Gateway admission still
# rejects the file unless it is fresh and exactly matches its own policy.
$destination = Join-Path $env:ProgramData 'NetherNet\onlineguard-evidence.json'
$directory = Split-Path -Parent $destination
New-Item -ItemType Directory -Path $directory -Force | Out-Null

function Values($value) { @($value | ForEach-Object { [string]$_ } | Where-Object { $_ -ne '' }) }
function AnyValue($value) { if (-not $value) { return 'Any' }; [string]$value }

$before = @(Get-NetIPAddress | ForEach-Object { (([string]$_.IPAddress -split '%')[0]) } | Sort-Object -Unique)
$profiles = @(Get-NetFirewallProfile -PolicyStore ActiveStore | ForEach-Object {
    [pscustomobject]@{Name=[string]$_.Name;Enabled=([string]$_.Enabled -eq 'True');DefaultInboundAction=[string]$_.DefaultInboundAction;AllowInboundRules=([string]$_.AllowInboundRules -eq 'True');DisabledInterfaceAliases=@(Values $_.DisabledInterfaceAliases)}
})
$rules = @(Get-NetFirewallRule -PolicyStore ActiveStore -Direction Inbound -Enabled True | ForEach-Object {
    $r = $_
    $a = $r | Get-NetFirewallApplicationFilter
    $p = $r | Get-NetFirewallPortFilter
    $ad = $r | Get-NetFirewallAddressFilter
    $s = $r | Get-NetFirewallServiceFilter
    $i = $r | Get-NetFirewallInterfaceFilter
    $it = $r | Get-NetFirewallInterfaceTypeFilter
    $security = $r | Get-NetFirewallSecurityFilter
    $profileNames = @(([string]$r.Profile -split ',\s*'))
    if ([string]$r.Profile -eq 'Any') { $profileNames = @('Domain','Private','Public') }
    [pscustomobject]@{PrimaryStatus=[string]$r.PrimaryStatus;EnforcementStatus=(@($r.EnforcementStatus | ForEach-Object { [string]$_ }) -join ',');Spec=[pscustomobject]@{
        DynamicTarget=[string]$p.DynamicTarget;ICMPTypes=@(Values $p.IcmpType);Owner=[string]$r.Owner;PolicyAppID=[string]$a.PolicyAppId;LooseSourceMapping=[bool]$r.LooseSourceMapping;LocalOnlyMapping=[bool]$r.LocalOnlyMapping;RemoteDynamicKeywordAddresses=@(Values $ad.RemoteDynamicKeywordAddresses)
        Name=[string]$r.Name;Group=[string]$r.Group;Program=[string]$a.Program;Direction=[string]$r.Direction;Action=[string]$r.Action;Enabled=([string]$r.Enabled -eq 'True');Profiles=$profileNames
        Protocol=[string]$p.Protocol;LocalAddresses=@(Values $ad.LocalAddress);RemoteAddresses=@(Values $ad.RemoteAddress);LocalPorts=@(Values $p.LocalPort);RemotePorts=@(Values $p.RemotePort)
        InterfaceAliases=@(Values $i.InterfaceAlias);InterfaceTypes=@(Values $it.InterfaceType);Service=[string]$s.Service;Package=(AnyValue $a.Package)
        Authentication=[string]$security.Authentication;Encryption=[string]$security.Encryption;OverrideBlockRules=([string]$security.OverrideBlockRules -eq 'True')
        EdgeTraversalPolicy=[string]$r.EdgeTraversalPolicy;LocalUsers=@(Values $security.LocalUser);RemoteUsers=@(Values $security.RemoteUser);RemoteMachines=@(Values $security.RemoteMachine);Platforms=@(Values $r.Platforms)
    }}
})
$after = @(Get-NetIPAddress | ForEach-Object { (([string]$_.IPAddress -split '%')[0]) } | Sort-Object -Unique)
if (($before -join ',') -ne ($after -join ',')) { throw 'Host addresses changed during firewall audit' }
$json = [pscustomobject]@{Platform='windows';Complete=$true;ObservedAt=[DateTime]::UtcNow.ToString('o');FirewallServiceRunning=((Get-Service MpsSvc).Status -eq 'Running');BaseFilteringEngineRunning=((Get-Service BFE).Status -eq 'Running');LocalAddresses=$after;Profiles=$profiles;Rules=$rules} | ConvertTo-Json -Depth 12 -Compress
$temporary = "$destination.$PID.tmp"
[IO.File]::WriteAllText($temporary, $json, [Text.UTF8Encoding]::new($false))
Move-Item -LiteralPath $temporary -Destination $destination -Force
