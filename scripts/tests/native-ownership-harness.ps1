param([string]$FixtureRoot)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Setup-Wizard.ps1')
function Get-CimInstance {param($ClassName);[pscustomobject]@{Name='NetherNetMultiServer.exe';ExecutablePath='C:\OtherInstallation\MultiServer\NetherNetMultiServer.exe'}}
function Start-Process {throw 'UNSAFE_PROCESS_START'}
function Invoke-NNAdmin {throw 'UNSAFE_RPC'}
try{Start-NNWizardRuntime $FixtureRoot @() $false;throw 'ownership guard did not stop startup'}catch{if($_.Exception.Message -notmatch 'Another NetherNet installation'){throw}}
