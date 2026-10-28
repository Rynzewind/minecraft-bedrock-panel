param([Parameter(Mandatory)][string]$FixtureRoot)
$ErrorActionPreference='Stop'
if(Test-Path $FixtureRoot){throw 'Use a new fixture root.'}
New-Item -ItemType Directory -Path $FixtureRoot|Out-Null
$source=Join-Path $FixtureRoot 'installer.exe';$drive=Join-Path $FixtureRoot 'drive';$backups=Join-Path $FixtureRoot 'previous'
[IO.File]::WriteAllText($source,'first verified installer fixture')
$publisher=Join-Path $PSScriptRoot '../publish-latest-installer.ps1'
& $publisher -Installer $source -ExpectedSha256 (Get-FileHash $source).Hash -BackupDirectory $backups -DriveFolder $drive
$target=Join-Path $drive 'NetherNetSetup.exe';$first=(Get-FileHash $target).Hash
[IO.File]::WriteAllText($source,'second verified installer fixture')
& $publisher -Installer $source -ExpectedSha256 (Get-FileHash $source).Hash -BackupDirectory $backups -DriveFolder $drive
if((Get-FileHash $target).Hash -ne (Get-FileHash $source).Hash){throw 'Latest installer not published.'}
$previous=@(Get-ChildItem $backups -File);if($previous.Count -ne 1 -or (Get-FileHash $previous[0].FullName).Hash -ne $first){throw 'Previous installer not retained.'}
$rejected=$false
try{& $publisher -Installer $source -ExpectedSha256 ('0'*64) -BackupDirectory $backups -DriveFolder $drive}catch{$rejected=$true}
if(-not $rejected -or (Get-FileHash $target).Hash -ne (Get-FileHash $source).Hash){throw 'Mismatch changed the current installer.'}
'Latest-installer publish, previous-copy backup and mismatched-input rejection passed.'
