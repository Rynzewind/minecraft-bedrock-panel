param([Parameter(Mandatory)][string]$SetupExe,[Parameter(Mandatory)][string]$ReleaseZip,[Parameter(Mandatory)][string]$ScratchDirectory)
$ErrorActionPreference='Stop'
$SetupExe=(Resolve-Path -LiteralPath $SetupExe).Path
$ReleaseZip=(Resolve-Path -LiteralPath $ReleaseZip).Path
$ScratchDirectory=[IO.Path]::GetFullPath($ScratchDirectory)
if(Test-Path -LiteralPath $ScratchDirectory){throw 'Use a new scratch directory for this test.'}
New-Item -ItemType Directory -Path $ScratchDirectory|Out-Null
$p=Start-Process -FilePath $SetupExe -ArgumentList @('/Q','/C',('/T:"'+$ScratchDirectory+'"')) -WindowStyle Hidden -PassThru -Wait
if($p.ExitCode -ne 0){throw 'EXE extraction failed.'}
$payload=Join-Path $ScratchDirectory 'payload.zip'
if((Get-FileHash $payload).Hash -ne (Get-FileHash $ReleaseZip).Hash){throw 'EXE payload checksum mismatch.'}
$shell=Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
$bootstrap=Join-Path $ScratchDirectory 'Bootstrap.ps1'
& $shell -NoProfile -ExecutionPolicy Bypass -File $bootstrap -SmokeTest
if($LASTEXITCODE -ne 0){throw 'Bundled wizard smoke test failed.'}
[IO.File]::WriteAllText($payload,'deliberately corrupted test fixture')
$message=& $shell -NoProfile -ExecutionPolicy Bypass -File $bootstrap -SmokeTest 2>&1
if($LASTEXITCODE -eq 0 -or ($message -join "`n") -notmatch 'checksum'){throw 'Corrupt payload was not rejected.'}
'EXE extraction, payload integrity, wizard smoke test and corrupt-payload rejection passed.'
