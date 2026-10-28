param()
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Runtime-Operations.ps1')
if(-not(Test-NNHelperStoppedCleanly ([pscustomobject]@{HasExited=$true;ExitCode=0}))){throw 'manual shutdown would trigger crash recovery'}
if(Test-NNHelperStoppedCleanly ([pscustomobject]@{HasExited=$true;ExitCode=1})){throw 'crash treated as manual shutdown'}
if(Test-NNHelperStoppedCleanly ([pscustomobject]@{HasExited=$false;ExitCode=0})){throw 'running process treated as stopped'}
