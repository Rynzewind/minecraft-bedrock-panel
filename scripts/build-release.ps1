param([string]$GoExe = '')
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
if ([string]::IsNullOrWhiteSpace($GoExe)) { $GoExe = Join-Path $root 'work\toolchains\go\bin\go.exe' }
if (-not (Test-Path -LiteralPath $GoExe)) { throw "Go toolchain not found: $GoExe" }
$env:GOCACHE = Join-Path $root 'work\gocache'
$env:GOMODCACHE = Join-Path $root 'work\gomodcache'
$env:GOPATH = Join-Path $root 'work\gopath'
$env:GOSUMDB = 'off'
$env:CGO_ENABLED = '0'
Push-Location $root
try {
	$operatorGuide = 'outputs\CONTENT-PACKS-AND-SERVER-CONTROLS.txt'
	foreach ($heading in @('Importing an add-on','Pending changes','Start, Stop, and Restart','Externally running','Backups','Rollback after an error')) {
		if (-not (Select-String -LiteralPath $operatorGuide -SimpleMatch $heading -Quiet)) { throw "Operator guide is missing required heading: $heading" }
	}
    $rsrc = Join-Path $root 'work\tools\rsrc.exe'
    $manifest = 'cmd\nethernet-multi-server-admin\NetherNetMultiServerAdmin.exe.manifest'
    $icon = 'cmd\nethernet-multi-server-admin\NetherNetMultiServerAdmin.ico'
    $resource = 'cmd\nethernet-multi-server-admin\rsrc_windows_amd64.syso'
    if (-not (Test-Path -LiteralPath $rsrc)) { throw "Windows resource compiler not found: $rsrc" }
    if (-not (Test-Path -LiteralPath $icon)) { throw "Windows administrator icon not found: $icon" }
    & $rsrc -manifest $manifest -ico $icon -arch amd64 -o $resource
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $resource)) { throw 'Failed to embed the administrator Common Controls v6 manifest' }
    & $GoExe test ./...
    if ($LASTEXITCODE -ne 0) { throw "Go tests failed: $LASTEXITCODE" }
    New-Item -ItemType Directory -Force -Path 'outputs\rollback' | Out-Null
    if ((Test-Path 'outputs\NetherNetMultiServer.exe') -and -not (Test-Path 'outputs\rollback\NetherNetMultiServer-single-account.exe')) { Copy-Item 'outputs\NetherNetMultiServer.exe' 'outputs\rollback\NetherNetMultiServer-single-account.exe' }
    & $GoExe build -buildvcs=false -trimpath -ldflags '-s -w' -o 'outputs\NetherNetMultiServer-AccountPool.exe' ./cmd/nethernet-multi-server
    if ($LASTEXITCODE -ne 0) { throw "Service build failed: $LASTEXITCODE" }
    & $GoExe build -buildvcs=false -trimpath -ldflags '-H windowsgui -s -w' -o 'outputs\NetherNetMultiServerAdmin.exe' ./cmd/nethernet-multi-server-admin
    if ($LASTEXITCODE -ne 0) { throw "Admin build failed: $LASTEXITCODE" }
	& $GoExe build -buildvcs=false -trimpath -ldflags '-s -w' -o 'outputs\NetherNetMultiServerControl.exe' ./cmd/nethernet-multi-server-control
	if ($LASTEXITCODE -ne 0) { throw "Control utility build failed: $LASTEXITCODE" }
    New-Item -ItemType Directory -Force -Path 'outputs\final\identities' | Out-Null
    Copy-Item -Force 'outputs\NetherNetMultiServer-AccountPool.exe' 'outputs\final\NetherNetMultiServer.exe'
    Copy-Item -Force 'outputs\NetherNetMultiServerAdmin.exe' 'outputs\final\NetherNetMultiServerAdmin.exe'
	Copy-Item -Force 'outputs\NetherNetMultiServerControl.exe' 'outputs\final\NetherNetMultiServerControl.exe'
    Copy-Item -Force 'outputs\config.json' 'outputs\final\config.json'
    Copy-Item -Force 'outputs\README.txt' 'outputs\final\README.txt'
    Copy-Item -Force 'outputs\ACCOUNT-POOL-SETUP.txt' 'outputs\final\ACCOUNT-POOL-SETUP.txt'
    Copy-Item -Force 'outputs\CONTENT-PACKS-AND-SERVER-CONTROLS.txt' 'outputs\final\CONTENT-PACKS-AND-SERVER-CONTROLS.txt'
    Copy-Item -Force 'outputs\Start-NetherNetMultiServer.cmd' 'outputs\final\Start-NetherNetMultiServer.cmd'
	Copy-Item -Force 'outputs\StartMinecraftServers.bat' 'outputs\final\StartMinecraftServers.bat'
	Copy-Item -Force 'outputs\Run-NetherNetMultiServer.cmd' 'outputs\final\Run-NetherNetMultiServer.cmd'
	Copy-Item -Force 'outputs\StopMinecraftServers.bat' 'outputs\final\StopMinecraftServers.bat'
	Copy-Item -Force 'outputs\StartMinecraftServers.vbs' 'outputs\final\StartMinecraftServers.vbs'
    Copy-Item -Force 'outputs\identities\*' 'outputs\final\identities\'
	New-Item -ItemType Directory -Force -Path 'outputs\candidate' | Out-Null
	Copy-Item -Force 'outputs\final\*' 'outputs\candidate\' -Recurse
    foreach ($file in @('outputs\NetherNetMultiServer-AccountPool.exe','outputs\NetherNetMultiServerAdmin.exe','outputs\final\NetherNetMultiServer.exe','outputs\final\NetherNetMultiServerAdmin.exe','outputs\rollback\NetherNetMultiServer-single-account.exe')) {
        if (-not (Test-Path $file) -or (Get-Item $file).Length -eq 0) { throw "Missing or empty artifact: $file" }
        Get-FileHash -Algorithm SHA256 $file | Format-Table Path,Hash -AutoSize
    }
} finally { Pop-Location }
