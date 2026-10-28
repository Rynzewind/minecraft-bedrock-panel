[CmdletBinding()]
param([Parameter(Mandatory)][string]$ReleaseZip,[Parameter(Mandatory)][string]$BuildDirectory,[Parameter(Mandatory)][string]$OutputExe,[string]$PublishDirectory='G:\My Drive\Minecraft Server Setup')
$ErrorActionPreference='Stop'
$BuildDirectory=[IO.Path]::GetFullPath($BuildDirectory);$OutputExe=[IO.Path]::GetFullPath($OutputExe)
& (Join-Path $PSScriptRoot 'build-single-file-setup.ps1') -ReleaseZip $ReleaseZip -BuildDirectory $BuildDirectory -OutputExe $OutputExe
& (Join-Path $PSScriptRoot 'tests/single-file-package-harness.ps1') -SetupExe $OutputExe -ReleaseZip $ReleaseZip -ScratchDirectory (Join-Path $BuildDirectory ('test-'+[guid]::NewGuid().ToString('N')))
$expected=(Get-FileHash -LiteralPath $OutputExe).Hash
$platform=Get-ChildItem 'C:\ProgramData\Microsoft\Windows Defender\Platform' -Directory|Sort-Object Name -Descending|Select-Object -First 1
$scan=& (Join-Path $platform.FullName 'MpCmdRun.exe') -Scan -ScanType 3 -File $OutputExe
$scan|Set-Content -LiteralPath (Join-Path (Split-Path $OutputExe -Parent) 'Defender-Scan.txt')
if($LASTEXITCODE -ne 0 -or -not($scan -match 'found no threats')){throw 'Installer Defender scan did not pass.'}
$protection=Get-MpComputerStatus
if(-not $protection.AntivirusEnabled -or -not $protection.RealTimeProtectionEnabled){throw 'Windows protection must remain enabled.'}
($expected.ToLowerInvariant()+'  '+[IO.Path]::GetFileName($OutputExe))|Set-Content -LiteralPath (Join-Path (Split-Path $OutputExe -Parent) 'SHA256.txt')
& (Join-Path $PSScriptRoot 'publish-latest-installer.ps1') -Installer $OutputExe -ExpectedSha256 $expected -BackupDirectory (Join-Path $BuildDirectory 'previous-drive-installers') -DriveFolder $PublishDirectory
