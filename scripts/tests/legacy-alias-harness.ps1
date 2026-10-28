param()
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Decommission-NetherNet.ps1')
function Get-NetAdapter {[pscustomobject]@{ifIndex=9;InterfaceGuid='adapter'}}
function Get-NetIPAddress {param($AddressFamily);@([pscustomobject]@{IPAddress='192.168.1.144';SkipAsSource=$false;PrefixOrigin='Manual';InterfaceIndex=9;PrefixLength=24},[pscustomobject]@{IPAddress='192.168.1.137';SkipAsSource=$true;PrefixOrigin='Manual';InterfaceIndex=9;PrefixLength=24},[pscustomobject]@{IPAddress='192.168.1.246';SkipAsSource=$true;PrefixOrigin='Manual';InterfaceIndex=9;PrefixLength=24},[pscustomobject]@{IPAddress='192.168.1.247';SkipAsSource=$true;PrefixOrigin='Manual';InterfaceIndex=9;PrefixLength=24})}
$candidates=@(Get-NNUnownedAliasCandidates -ExistingAliases @() -PrimaryIP '192.168.1.137')
if($candidates.Count -ne 2 -or @($candidates|Where-Object owned).Count -ne 0 -or @($candidates|Where-Object {$_.ip -in @('192.168.1.137','192.168.1.144')}).Count){throw 'legacy secondary discovery inferred ownership or selected a primary address'}

