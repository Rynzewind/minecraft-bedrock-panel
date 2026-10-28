[CmdletBinding()]
param([string]$GoExe='')
$ErrorActionPreference='Stop'
$nnRoot=Split-Path -Parent $PSScriptRoot
if([string]::IsNullOrWhiteSpace($GoExe)){$nnGoCommand=Get-Command go -ErrorAction SilentlyContinue;if(!$nnGoCommand){throw 'Install Go 1.26 or newer, or pass -GoExe.'};$GoExe=$nnGoCommand.Source}
$GoExe=(Resolve-Path -LiteralPath $GoExe).Path
$nnStamp=Get-Date -Format 'yyyyMMdd-HHmmss'
$nnOutput=Join-Path $nnRoot ('outputs\release-'+$nnStamp)
if(Test-Path -LiteralPath $nnOutput){throw 'Release output already exists; use a fresh build time.'}
$env:GOCACHE=Join-Path $nnRoot 'work\gocache'
$env:GOMODCACHE=Join-Path $nnRoot 'work\gomodcache'
$env:CGO_ENABLED='0'
$nnResourceCompiler=Join-Path $nnRoot 'work\tools\rsrc.exe'
Push-Location $nnRoot
try {
 if(!(Test-Path -LiteralPath $nnResourceCompiler)){
  New-Item -ItemType Directory -Path (Split-Path $nnResourceCompiler -Parent) -Force|Out-Null
  $nnSavedGoBin=$env:GOBIN
  try{$env:GOBIN=Split-Path $nnResourceCompiler -Parent;& $GoExe install github.com/akavel/rsrc@v0.10.2;if($LASTEXITCODE -ne 0){throw 'Resource compiler installation failed.'}}finally{$env:GOBIN=$nnSavedGoBin}
 }
 & $nnResourceCompiler -manifest 'cmd\nethernet-multi-server-admin\NetherNetMultiServerAdmin.exe.manifest' -ico 'cmd\nethernet-multi-server-admin\NetherNetMultiServerAdmin.ico' -arch amd64 -o 'cmd\nethernet-multi-server-admin\rsrc_windows_amd64.syso'
 if($LASTEXITCODE -ne 0){throw 'Windows resource generation failed.'}
 & $GoExe test -p 1 ./...
 if($LASTEXITCODE -ne 0){throw 'Tests failed.'}
 New-Item -ItemType Directory -Path (Join-Path $nnOutput 'setup-assets') -Force|Out-Null
 foreach($nnTarget in @('nethernet-multi-server','nethernet-multi-server-admin','nethernet-multi-server-control')){
  $nnName=switch($nnTarget){'nethernet-multi-server'{'NetherNetMultiServer.exe'} 'nethernet-multi-server-admin'{'NetherNetMultiServerAdmin.exe'} 'nethernet-multi-server-control'{'NetherNetMultiServerControl.exe'}}
  $nnFlags=if($nnTarget -like '*admin'){'-H windowsgui -s -w'}else{'-s -w'}
  & $GoExe build -buildvcs=false -trimpath -ldflags $nnFlags -o (Join-Path $nnOutput $nnName) ('./cmd/'+$nnTarget)
  if($LASTEXITCODE -ne 0){throw ('Build failed: '+$nnTarget)}
 }
 Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'setup-assets') -File|ForEach-Object{Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $nnOutput 'setup-assets')}
 Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'update-panel.ps1') -Destination $nnOutput
 [IO.File]::WriteAllText((Join-Path $nnOutput 'Install-NetherNet.cmd'),"@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Setup-Wizard.ps1`" %*`r`n",[Text.UTF8Encoding]::new($false))
 [IO.File]::WriteAllText((Join-Path $nnOutput 'Uninstall-NetherNet.cmd'),"@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Decommission-NetherNet.ps1`" %*`r`n",[Text.UTF8Encoding]::new($false))
 $nnSourceEntries=@(Get-ChildItem -LiteralPath $nnRoot -Force|Where-Object Name -NotIn @('.git','work','outputs','bin','.superpowers')|Select-Object -ExpandProperty FullName)
 Compress-Archive -LiteralPath $nnSourceEntries -DestinationPath (Join-Path $nnOutput 'NetherNet-Migration-Source.zip')
 $nnHashes=[ordered]@{}
 Get-ChildItem -LiteralPath $nnOutput -Recurse -File|ForEach-Object{$nnRelative=$_.FullName.Substring($nnOutput.Length+1).Replace('\','/');$nnHashes[$nnRelative]=(Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
 [IO.File]::WriteAllText((Join-Path $nnOutput 'release-hashes.json'),($nnHashes|ConvertTo-Json),[Text.UTF8Encoding]::new($false))
 $nnZip=$nnOutput+'.zip'
 Compress-Archive -Path (Join-Path $nnOutput '*') -DestinationPath $nnZip
 Write-Output ('Release directory: '+$nnOutput)
 Write-Output ('Release ZIP: '+$nnZip)
} finally {Pop-Location}
