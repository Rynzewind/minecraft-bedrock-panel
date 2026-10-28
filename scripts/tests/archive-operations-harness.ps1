param([string]$FixtureRoot,[string]$Scenario='valid')
$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot '../setup-assets/Archive-Operations.ps1')
$source=Join-Path $FixtureRoot 'source';$dest=Join-Path $FixtureRoot 'new';New-Item -ItemType Directory -Path $source|Out-Null
$old=Join-Path $FixtureRoot 'old';$files=@{}
$servers=@();foreach($id in @('creative','survival')){$name=if($id -eq 'creative'){'Creative'}else{'Survival'};$port=if($id -eq 'creative'){19134}else{19136};$network=if($id -eq 'creative'){'13468123134000001934'}else{'13468123136000001936'};$range=if($id -eq 'creative'){'49152-49200'}else{'49201-49249'};$dir=Join-Path $old $name
 $servers+=@{id=$id;name=$name;level_name='World';network_id=$network;backend=('192.168.1.137:'+$port);process=@{executable=(Join-Path $dir 'bedrock_server.exe');working_directory=$dir;world_directory=(Join-Path $dir 'worlds\World');log_directory=(Join-Path $dir 'logs')}}
 $files[$name+'/bedrock_server.exe']='fixture';$files[$name+'/server.properties']="# preserve`r`ntransport=nethernet`r`nserver-ip=192.168.1.137`r`nserver-port=$port`r`nserver-udp-ports=$range`r`nenable-lan-visibility=false`r`n";$files[$name+'/worlds/World/db/000001.log']='world transactions';$files['MultiServer/identities/'+$network+'.pem']='persistent identity'
}
$files['MultiServer/config.json']=(@{interface_ip='192.168.1.137';listen='192.168.1.255:7551';future=@{retain=$true};servers=$servers}|ConvertTo-Json -Depth 10)
foreach($name in @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe')){$files['MultiServer/'+$name]='fixture '+$name}
$files['accounts-migration.json']='{"version":1,"accounts":[{"id":"a1","person_label":"Test","gamertag":"Player","xuid":"42","order":0}],"players":[{"account_id":"a1","console_xuid":"24","console_gamertag":"Console","last_seen":"2026-10-20T12:34:56Z"}]}'
$files['MultiServer/content-staging/state.json']=(@{packs=@(@{installed_path=(Join-Path $old 'MultiServer\content-staging\packs\id\2.0.0')});versions=@(@{installed_path=(Join-Path $old 'MultiServer\content-staging\packs\id\1.0.0')});enabled_versions=@{survival=@{id=@(1,0,0)}}}|ConvertTo-Json -Depth 10)
$manifestFiles=@();foreach($path in $files.Keys){$bytes=[Text.Encoding]::UTF8.GetBytes($files[$path]);$sha=[Security.Cryptography.SHA256]::Create();try{$hash=([BitConverter]::ToString($sha.ComputeHash($bytes))).Replace('-','')}finally{$sha.Dispose()};$manifestFiles+=@{path=$path;size=$bytes.Length;sha256=$hash}}
if($Scenario -eq 'checksum'){$manifestFiles[0].sha256=('0'*64)}
$manifest=@{format_version=1;source_root=$old;created_utc='2026-10-20T00:00:00Z';files=$manifestFiles}
$zipPath=Join-Path $FixtureRoot 'test.zip';$zip=[IO.Compression.ZipFile]::Open($zipPath,[IO.Compression.ZipArchiveMode]::Create)
try{foreach($path in $files.Keys){$entry=$zip.CreateEntry($path);$stream=$entry.Open();try{$bytes=[Text.Encoding]::UTF8.GetBytes($files[$path]);$stream.Write($bytes,0,$bytes.Length)}finally{$stream.Dispose()}};$entry=$zip.CreateEntry('manifest.json');$stream=$entry.Open();try{$bytes=[Text.Encoding]::UTF8.GetBytes(($manifest|ConvertTo-Json -Depth 20));$stream.Write($bytes,0,$bytes.Length)}finally{$stream.Dispose()};if($Scenario -eq 'traversal'){$zip.CreateEntry('../escape')|Out-Null}}finally{$zip.Dispose()}
if($Scenario -ne 'valid'){try{Test-NNSnapshotArchive $zipPath|Out-Null;throw 'invalid archive accepted'}catch{if($_.Exception.Message -eq 'invalid archive accepted'){throw}};if(Test-Path $dest){throw 'invalid archive created destination'};exit 0}
Restore-NNSnapshot -ArchivePath $zipPath -Destination $dest -PrimaryIP '10.42.8.9' -BroadcastIP '10.42.15.255' -ServerAddresses @{creative='10.42.9.246';survival='10.42.9.247'}|Out-Null
$packState=Get-Content (Join-Path $dest 'MultiServer/content-staging/state.json') -Raw|ConvertFrom-Json
if($packState.versions[0].installed_path -ne (Join-Path $dest 'MultiServer\content-staging\packs\id\1.0.0')){throw 'retained pack version path was not rebased'}
$cfg=Get-Content (Join-Path $dest 'MultiServer/config.json') -Raw -Encoding UTF8|ConvertFrom-Json
if(-not $cfg.future.retain -or $cfg.interface_ip -ne '10.42.8.9' -or $cfg.listen -ne '10.42.15.255:7551'){throw 'configuration fields lost'}
foreach($s in $cfg.servers){$ip=if($s.id -eq 'creative'){'10.42.9.246'}else{'10.42.9.247'};if(($s.backend -split ':')[0] -ne $ip){throw 'wrong backend'};if([IO.File]::ReadAllText((Join-Path $s.process.world_directory 'db/000001.log')) -ne 'world transactions'){throw 'world changed'};if([IO.File]::ReadAllText((Join-Path $s.process.working_directory 'server.properties')) -notmatch [regex]::Escape('server-ip='+$ip)){throw 'properties mismatch'}}
$disk=Get-Content (Join-Path $dest 'MultiServer/account-store/accounts.json') -Raw|ConvertFrom-Json
Add-Type -AssemblyName System.Security
$plain=[Security.Cryptography.ProtectedData]::Unprotect([Convert]::FromBase64String($disk.Accounts[0].Token),[Text.Encoding]::UTF8.GetBytes('NetherNetMultiServer/account-store/v1'),[Security.Cryptography.DataProtectionScope]::CurrentUser)
if([Text.Encoding]::UTF8.GetString($plain) -ne '{}' -or $disk.Accounts[0].Healthy -or $disk.Players[0].last_seen -ne '2026-10-20T12:34:56Z'){throw 'account migration incompatible'}
if(-not(Test-Path (Join-Path $dest 'migration-activation-required'))){throw 'activation gate missing'}
try{Restore-NNSnapshot -ArchivePath $zipPath -Destination $dest -PrimaryIP '10.42.8.9' -BroadcastIP '10.42.15.255' -ServerAddresses @{creative='10.42.9.246';survival='10.42.9.247'};throw 'overwrote existing destination'}catch{if($_.Exception.Message -eq 'overwrote existing destination'){throw}}

