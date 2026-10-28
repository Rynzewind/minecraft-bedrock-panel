[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$ServerRoot = 'C:\MinecraftServers',
    [string]$PackageRoot = $PSScriptRoot,
    [switch]$CheckOnly
)
$ErrorActionPreference = 'Stop'
$ServerRoot = [IO.Path]::GetFullPath($ServerRoot)
$PackageRoot = [IO.Path]::GetFullPath($PackageRoot)
if (-not (Test-Path -LiteralPath (Join-Path $ServerRoot 'MultiServer\config.json'))) { throw 'Choose the existing Minecraft installation root.' }
$manifest = Get-Content -LiteralPath (Join-Path $PackageRoot 'release-hashes.json') -Raw | ConvertFrom-Json
$files = @('NetherNetMultiServer.exe','NetherNetMultiServerAdmin.exe','NetherNetMultiServerControl.exe','NetherNet-Migration-Source.zip','setup-assets/Configure-Windows.ps1','setup-assets/Network-Configuration.ps1','setup-assets/Runtime-Operations.ps1','setup-assets/Decommission-NetherNet.ps1','setup-assets/Install-Tools.ps1','Uninstall-NetherNet.cmd','Install-NetherNet.cmd','release-hashes.json','setup-assets/Setup-Wizard.ps1','setup-assets/Archive-Operations.ps1')
foreach ($file in $files) {
    $expected = $manifest.PSObject.Properties[$file].Value
    if ($file -eq 'release-hashes.json') {continue}
    if (-not $expected -or (Get-FileHash -LiteralPath (Join-Path $PackageRoot $file) -Algorithm SHA256).Hash -ne $expected) { throw "Release checksum failed: $file" }
}
function Invoke-Admin([string]$Method, [hashtable]$Parameters = @{}) {
    $pipe = [IO.Pipes.NamedPipeClientStream]::new('.', 'NetherNetMultiServer.Admin.v1', [IO.Pipes.PipeDirection]::InOut)
    try {
        $pipe.Connect(2000)
        $request = @{id='migration-update';method=$Method;params=$Parameters} | ConvertTo-Json -Compress
        $payload = [Text.Encoding]::UTF8.GetBytes($request)
        $writer = [IO.BinaryWriter]::new($pipe,[Text.Encoding]::UTF8,$true)
        $reader = [IO.BinaryReader]::new($pipe,[Text.Encoding]::UTF8,$true)
        $writer.Write([uint32]$payload.Length); $writer.Write($payload); $writer.Flush()
        $length = $reader.ReadUInt32()
        if ($length -gt 1048576) { throw 'Unexpected admin response size.' }
        $response = [Text.Encoding]::UTF8.GetString($reader.ReadBytes($length)) | ConvertFrom-Json
        if ($response.error) { throw $response.error.message }
        return $response.result
    } finally { $pipe.Dispose() }
}
$processes = @(Get-CimInstance Win32_Process -Filter "Name='NetherNetMultiServer.exe' OR Name='bedrock_server.exe' OR Name='NetherNetMultiServerAdmin.exe'")
$helperRunning = @($processes | Where-Object Name -eq 'NetherNetMultiServer.exe').Count -gt 0
$runningIDs = @()
if (@($processes | Where-Object Name -eq 'NetherNetMultiServerAdmin.exe').Count -gt 0) { throw 'Close the admin panel before installing this update.' }
foreach ($process in $processes) {
    if (-not $process.ExecutablePath -or -not $process.ExecutablePath.StartsWith($ServerRoot.TrimEnd('\')+'\',[StringComparison]::OrdinalIgnoreCase)) { throw 'Another Minecraft deployment is running, or its process path cannot be verified. Stop it before updating.' }
}
if ($helperRunning) {
    $statuses = @(Invoke-Admin 'servers.list')
    if (@($statuses | Where-Object { $_.Players -gt 0 }).Count -gt 0) { throw 'Players are connected. Update after they leave.' }
    if (@($statuses | Where-Object { $_.State -notin @('running','stopped','failed') }).Count -gt 0) { throw 'Server state is not safe for update.' }
    $runningIDs = @($statuses | Where-Object State -eq 'running' | ForEach-Object ID)
} elseif (@($processes | Where-Object Name -eq 'bedrock_server.exe').Count -gt 0) {
    throw 'Bedrock servers are running without the helper. Stop them cleanly first.'
}
if ($CheckOnly) { Write-Output 'Release checksums and server state are suitable for update.'; return }
if (-not $PSCmdlet.ShouldProcess($ServerRoot,'Back up and install the Minecraft snapshot/admin update')) { return }
$multi = Join-Path $ServerRoot 'MultiServer'
function Get-UpdateTarget([string]$File) {
    if ($File -eq 'NetherNet-Migration-Source.zip') {return (Join-Path $multi 'engineering-handover\NetherNet-Migration-Source.zip')}
    if ($File -eq 'NetherNetMigrationArchive.exe' -or $File -eq 'Uninstall-NetherNet.cmd' -or $File -eq 'Install-NetherNet.cmd' -or $File -eq 'release-hashes.json' -or $File.StartsWith('setup-assets/')) {return (Join-Path $multi ('installer\'+$File))}
    return (Join-Path $multi $File)
}
if ($helperRunning) {
    foreach ($id in $runningIDs) { $null = Invoke-Admin 'server.stop' @{server_id=$id} }
    $null = Invoke-Admin 'system.shutdown'
    $deadline = (Get-Date).AddSeconds(15)
    do {
        $remaining = @(Get-CimInstance Win32_Process -Filter "Name='NetherNetMultiServer.exe' OR Name='bedrock_server.exe'")
        if ($remaining.Count -eq 0) { break }
        Start-Sleep -Milliseconds 250
    } while ((Get-Date) -lt $deadline)
    if ($remaining.Count -gt 0) { throw 'Processes did not exit. No binaries were replaced.' }
}
$backup = Join-Path $multi ('backups\migration-panel-' + (Get-Date -Format 'yyyyMMdd-HHmmss-fff'))
New-Item -ItemType Directory -Path $backup -Force | Out-Null
try {
    foreach ($file in $files) {
        $target = Get-UpdateTarget $file
        if (Test-Path -LiteralPath $target) { New-Item -ItemType Directory -Path (Split-Path -Parent (Join-Path $backup $file)) -Force|Out-Null; Copy-Item -LiteralPath $target -Destination (Join-Path $backup $file) }
        New-Item -ItemType Directory -Path (Split-Path -Parent $target) -Force | Out-Null
        Copy-Item -LiteralPath (Join-Path $PackageRoot $file) -Destination $target -Force
                $expected=if($file -eq 'release-hashes.json'){(Get-FileHash -LiteralPath (Join-Path $PackageRoot $file) -Algorithm SHA256).Hash}else{$manifest.PSObject.Properties[$file].Value}
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ne $expected) { throw "Installed checksum failed: $file" }
    }
} catch {
    foreach ($file in $files) {
        $previous = Join-Path $backup $file
        if (Test-Path -LiteralPath $previous) {
            $target = Get-UpdateTarget $file
            Copy-Item -LiteralPath $previous -Destination $target -Force
        }
    }
    throw
}
if ($helperRunning) {
    Start-Process -FilePath (Join-Path $multi 'NetherNetMultiServer.exe') -ArgumentList ('"'+(Join-Path $multi 'config.json')+'"') -WorkingDirectory $multi -WindowStyle Hidden
    $ready = $false
    for ($attempt=0; $attempt -lt 30; $attempt++) { try { $null=Invoke-Admin 'servers.list'; $ready=$true; break } catch { Start-Sleep -Seconds 1 } }
    if (-not $ready) { throw "Updated helper did not become ready. Previous binaries are in $backup" }
    foreach ($id in $runningIDs) { $null=Invoke-Admin 'server.start' @{server_id=$id} }
}
Write-Output "Snapshot panel update installed. Rollback binaries: $backup"
Write-Output 'The previous running/stopped state was preserved. Open the panel and use Migration / Snapshots.'





