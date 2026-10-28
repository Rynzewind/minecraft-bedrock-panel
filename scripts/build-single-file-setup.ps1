[CmdletBinding()]
param([Parameter(Mandatory)][string]$ReleaseZip,[Parameter(Mandatory)][string]$BuildDirectory,[Parameter(Mandatory)][string]$OutputExe)
$ErrorActionPreference='Stop'
$ReleaseZip=(Resolve-Path -LiteralPath $ReleaseZip).Path
$BuildDirectory=[IO.Path]::GetFullPath($BuildDirectory);$OutputExe=[IO.Path]::GetFullPath($OutputExe)
if(Test-Path -LiteralPath $OutputExe){throw 'Refusing to replace an existing setup executable.'}
New-Item -ItemType Directory -Path $BuildDirectory -Force|Out-Null
New-Item -ItemType Directory -Path (Split-Path $OutputExe -Parent) -Force|Out-Null
$hash=(Get-FileHash -LiteralPath $ReleaseZip -Algorithm SHA256).Hash
Copy-Item -LiteralPath $ReleaseZip -Destination (Join-Path $BuildDirectory 'payload.zip')
$bootstrap=(Get-Content (Join-Path $PSScriptRoot 'packaging/Bootstrap.ps1.in') -Raw).Replace('__PAYLOAD_SHA256__',$hash)
[IO.File]::WriteAllText((Join-Path $BuildDirectory 'Bootstrap.ps1'),$bootstrap,[Text.UTF8Encoding]::new($false))
$sed=@"
[Version]
Class=IEXPRESS
SEDVersion=3
[Options]
PackagePurpose=InstallApp
ShowInstallProgramWindow=1
HideExtractAnimation=0
UseLongFileName=1
InsideCompressed=0
CAB_FixedSize=0
CAB_ResvCodeSigning=0
RebootMode=N
InstallPrompt=%InstallPrompt%
DisplayLicense=%DisplayLicense%
FinishMessage=%FinishMessage%
TargetName=%TargetName%
FriendlyName=%FriendlyName%
AppLaunched=%AppLaunched%
PostInstallCmd=%PostInstallCmd%
AdminQuietInstCmd=%AdminQuietInstCmd%
UserQuietInstCmd=%UserQuietInstCmd%
SourceFiles=SourceFiles
[Strings]
InstallPrompt=
DisplayLicense=
FinishMessage=
TargetName=$OutputExe
FriendlyName=NetherNet Multi-Server Setup
AppLaunched=powershell.exe -NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -File Bootstrap.ps1
PostInstallCmd=<None>
AdminQuietInstCmd=
UserQuietInstCmd=
FILE0="Bootstrap.ps1"
FILE1="payload.zip"
[SourceFiles]
SourceFiles0=$BuildDirectory\
[SourceFiles0]
%FILE0%=
%FILE1%=
"@
$sedPath=Join-Path $BuildDirectory 'NetherNet.sed'
[IO.File]::WriteAllText($sedPath,($sed -replace "`r?`n","`r`n")+"`r`n",[Text.Encoding]::ASCII)
$builder=Join-Path $env:SystemRoot 'System32\iexpress.exe'
$p=Start-Process -FilePath $builder -ArgumentList @('/N','/Q','NetherNet.sed') -WorkingDirectory $BuildDirectory -WindowStyle Hidden -PassThru -Wait
if($p.ExitCode -ne 0 -or -not(Test-Path -LiteralPath $OutputExe)){throw ('Single-file setup build failed: '+$p.ExitCode)}
Get-FileHash -LiteralPath $OutputExe -Algorithm SHA256


