[CmdletBinding()]
param([Parameter(Mandatory)][string]$GoExe,[Parameter(Mandatory)][string]$OutputDirectory)
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$OutputDirectory=[IO.Path]::GetFullPath($OutputDirectory)
Push-Location $root
try {
    & $GoExe test -count=1 ./...
    if ($LASTEXITCODE -ne 0) {throw 'Tests failed.'}
    & $GoExe vet ./...
    if ($LASTEXITCODE -ne 0) {throw 'Go vet failed.'}
    New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $OutputDirectory 'setup-assets') -Force | Out-Null
        Get-ChildItem -LiteralPath (Join-Path $PSScriptRoot 'setup-assets') -File | ForEach-Object {Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $OutputDirectory ('setup-assets/'+$_.Name)) -Force}
    [IO.File]::WriteAllText((Join-Path $OutputDirectory 'Uninstall-NetherNet.cmd'),"@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Decommission-NetherNet.ps1`" %*`r`n",[Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText((Join-Path $OutputDirectory 'Install-NetherNet.cmd'),"@echo off`r`npowershell.exe -NoProfile -ExecutionPolicy Bypass -File `"%~dp0setup-assets\Setup-Wizard.ps1`" %*`r`n",[Text.UTF8Encoding]::new($false))
    foreach($target in @('nethernet-multi-server','nethernet-multi-server-admin','nethernet-multi-server-control')) {
        $name=switch($target){'nethernet-multi-server' {'NetherNetMultiServer.exe'} 'nethernet-multi-server-admin' {'NetherNetMultiServerAdmin.exe'} 'nethernet-multi-server-control' {'NetherNetMultiServerControl.exe'} 'nethernet-migration-archive' {'NetherNetMigrationArchive.exe'}}
        $flags=if($target -match 'admin$'){'-H windowsgui -s -w'}else{'-s -w'}
        & $GoExe build -buildvcs=false -trimpath -ldflags $flags -o (Join-Path $OutputDirectory $name) ('./cmd/'+$target)
        if ($LASTEXITCODE -ne 0) {throw "Build failed: $target"}
    }
    Write-Output "Release executables built in $OutputDirectory. Package source and documentation with release hashes before deployment."
} finally {Pop-Location}





