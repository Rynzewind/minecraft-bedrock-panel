[CmdletBinding()]
param([Parameter(Mandatory)][string]$Installer,[Parameter(Mandatory)][string]$ExpectedSha256,[Parameter(Mandatory)][string]$BackupDirectory,[string]$DriveFolder='G:\My Drive\Minecraft Server Setup')
$ErrorActionPreference='Stop'
$Installer=(Resolve-Path -LiteralPath $Installer).Path
if((Get-FileHash -LiteralPath $Installer -Algorithm SHA256).Hash -ne $ExpectedSha256){throw 'Installer changed after verification.'}
New-Item -ItemType Directory -Path $DriveFolder -Force|Out-Null
$target=Join-Path $DriveFolder 'NetherNetSetup.exe'
if((Test-Path -LiteralPath $target) -and (Get-FileHash -LiteralPath $target).Hash -eq $ExpectedSha256){Write-Output ('Latest installer already published: '+$target);return}
$staged=Join-Path $DriveFolder ('.NetherNetSetup-'+[guid]::NewGuid().ToString('N')+'.tmp')
$previous=$null;$replacing=$false
try {
 Copy-Item -LiteralPath $Installer -Destination $staged
 if((Get-FileHash -LiteralPath $staged).Hash -ne $ExpectedSha256){throw 'Drive installer copy checksum failed.'}
 if(Test-Path -LiteralPath $target){New-Item -ItemType Directory -Path $BackupDirectory -Force|Out-Null;$previous=Join-Path $BackupDirectory ('NetherNetSetup-'+[guid]::NewGuid().ToString('N')+'.exe');Copy-Item -LiteralPath $target -Destination $previous;if((Get-FileHash $target).Hash -ne (Get-FileHash $previous).Hash){throw 'Previous installer backup checksum failed.'}}
 $replacing=$true
 Move-Item -LiteralPath $staged -Destination $target -Force
 if((Get-FileHash -LiteralPath $target).Hash -ne $ExpectedSha256){throw 'Published installer checksum failed.'}
 Write-Output ('Published latest installer: '+$target)
} catch {
 if($replacing -and $previous -and (Test-Path -LiteralPath $previous)){Copy-Item -LiteralPath $previous -Destination $target -Force}
 throw
} finally {if(Test-Path -LiteralPath $staged){Remove-Item -LiteralPath $staged -Force}}
