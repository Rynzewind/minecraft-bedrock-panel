param([string]$FixtureRoot)
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Decommission-NetherNet.ps1')
New-Item -ItemType Directory -Force (Join-Path $FixtureRoot 'MultiServer'),(Join-Path $FixtureRoot 'Creative')|Out-Null
if(-not(Test-NNCommandOwned (Join-Path $FixtureRoot 'Creative\bedrock_server.exe') '' $FixtureRoot)){throw 'owned server not recognised'}
if(Test-NNCommandOwned (Join-Path ($FixtureRoot+'-unrelated') 'Creative\bedrock_server.exe') '' $FixtureRoot){throw 'unrelated server matched'}
if(Test-NNCommandOwned 'C:\Other\bedrock_server.exe' '' $FixtureRoot){throw 'name-only ownership'}
if(-not(Test-NNCommandOwned 'C:\Windows\System32\cmd.exe' ('/c "'+(Join-Path $FixtureRoot 'StartMinecraftServers.bat')+'"') $FixtureRoot)){throw 'legacy task not recognised'}
if(Test-NNCommandOwned 'C:\Windows\System32\cmd.exe' ('/c "'+(Join-Path ($FixtureRoot+'-unrelated') 'StartMinecraftServers.bat')+'"') $FixtureRoot){throw 'unrelated task matched'}
try{Assert-NNScope $FixtureRoot (Join-Path (Split-Path $FixtureRoot -Parent) 'outside')|Out-Null;throw 'outside accepted'}catch{if($_.Exception.Message -notmatch 'escapes'){throw}}
$candidate=Join-Path $FixtureRoot 'MultiServer-Candidate';New-Item -ItemType Directory -Path $candidate|Out-Null
$cfg=@{servers=@(@{network_id='permanent';process=@{working_directory=(Join-Path $FixtureRoot 'Creative')}})}
Write-NNJson (Join-Path $FixtureRoot 'MultiServer\config.json') $cfg
Write-NNJson (Join-Path $candidate 'config.json') $cfg
[IO.File]::WriteAllText((Join-Path $candidate 'NetherNetMultiServer.exe'),'fixture')
if(@(Get-NNAssociatedAppDirectories $FixtureRoot).Count -ne 1){throw 'associated legacy helper omitted'}
$cfg.servers[0].network_id='unrelated';Write-NNJson (Join-Path $candidate 'config.json') $cfg
if(@(Get-NNAssociatedAppDirectories $FixtureRoot).Count -ne 0){throw 'unrelated installation selected'}
if(Test-NNCommandOwned 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' ('-Command "Get-Content '''+(Join-Path $FixtureRoot 'StartMinecraftServers.bat')+'''"') $FixtureRoot){throw 'inspection shell incorrectly classified as a project watchdog'}
if(Test-NNCommandOwned 'C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe' ('-Command Get-Content "'+(Join-Path $FixtureRoot 'StartMinecraftServers.bat')+'"') $FixtureRoot){throw 'read-only shell incorrectly classified as a project watchdog'}
if(-not(Test-NNCommandOwned (Join-Path $FixtureRoot 'StartMinecraftServers.vbs') '' $FixtureRoot)){throw 'direct startup shortcut target omitted'}
