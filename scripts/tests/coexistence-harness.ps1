param([string]$Scenario)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Network-Configuration.ps1')
$script:stores=@{active='disabled';persistent='disabled'}
$script:writes=@()
function Invoke-NNNetsh {param([string[]]$Arguments)
 $store=($Arguments|Where-Object {$_ -like 'store=*'}).Substring(6)
 if($Arguments -contains 'show'){return ('DHCP/Static IP coexistence : '+$script:stores[$store])}
 $value=($Arguments|Where-Object {$_ -like 'dhcpstaticipcoexistence=*'}).Substring(24)
 $script:writes+=,$store
 if($Scenario -eq 'restore-failure' -and $store -eq 'persistent'){throw 'simulated persistent write failure'}
 if($Scenario -ne 'silent-noop'){$script:stores[$store]=$value}
}
switch($Scenario){
 'silent-noop' {
  foreach($store in @('active','persistent')){
   $failed=$false
   try{Set-NNCoexistence 9 'enabled' $store}catch{$failed=$true}
   if(-not $failed){throw ('silent '+$store+' write accepted')}
  }
 }
 'restore-failure' {
  $script:stores.active='enabled'
  $failed=$false
  try{Restore-NNCoexistence 9 ([pscustomobject]@{coexistence='disabled';coexistence_persistent='disabled'})}catch{$failed=$true}
  if(-not $failed -or $script:stores.active -ne 'disabled'){throw 'one failed store blocked restoration of the other store'}
 }
 'legacy' {
  $script:stores.active='enabled';$script:stores.persistent='enabled'
  Restore-NNCoexistence 9 ([pscustomobject]@{coexistence='disabled'})
  if($script:stores.active -ne 'enabled' -or $script:stores.persistent -ne 'disabled' -or $script:writes.Count -ne 1){throw 'legacy journal invented an active-store rollback'}
 }
 default {throw 'Unknown scenario'}
}
'PASS '+$Scenario
