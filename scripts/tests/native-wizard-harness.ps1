param([string]$FixtureRoot)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Setup-Wizard.ps1')
$form=New-NNSetupWindow
function Get-NNWizardRoot {$FixtureRoot}
function Get-NNWizardPackage {[pscustomobject]@{hashes='fixture'}}
function Get-NNWizardAdapter {[pscustomobject]@{primary_ip='10.42.8.9';prefix_length=20}}
function Get-NNRestoreSummary {param([string]$ArchivePath);'Fixture saved servers and addons'}
function Confirm-NNRestorePlan {param([string]$Summary);return $true}
$script:deployed=$false
function Install-NNWizardTools {param($Root,$Package);$script:deployed=$true}
function Invoke-NNArchive {param([string[]]$Arguments);if($Arguments.Count -ne 7 -or $Arguments[0] -ne '--restore-network' -or $Arguments[5] -ne '10.42.9.246' -or $Arguments[6] -ne '10.42.9.247'){throw ('Wizard lost archive arguments: '+($Arguments -join '|'))}}
try{$script:WizardFields.creative.Text='10.42.9.246';$script:WizardFields.survival.Text='10.42.9.247';$script:WizardFields.archive.Text='fixture.zip';Invoke-NNWizardAction 'restore';if(-not $script:deployed){throw $script:WizardFields.status.Text}}finally{$form.Dispose()}
