[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$taskName = 'NetherNet Minecraft Servers'
$root = 'C:\MinecraftServers'
$multiRoot = Join-Path $root 'MultiServer'
$stopScript = Join-Path $root 'StopMinecraftServers.bat'
$fallbackStart = Join-Path $root 'StartMinecraftServers.bat'
$logPath = Join-Path $multiRoot 'startup-task-activation.log'

function Write-ActivationLog([string]$Message) {
    $line = '[{0}] {1}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Message
    Add-Content -LiteralPath $logPath -Value $line -Encoding UTF8
    Write-Host $line
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This activation must be run as Administrator.'
}

$task = Get-ScheduledTask -TaskName $taskName -ErrorAction Stop
if ($task.Principal.LogonType -ne 'Password') {
    throw "Scheduled task has unexpected logon type: $($task.Principal.LogonType)"
}

Write-ActivationLog 'Beginning clean transition from the interactive session to the boot task.'
& $stopScript
if ($LASTEXITCODE -ne 0) {
    throw "Clean Minecraft shutdown failed with exit code $LASTEXITCODE."
}

$deadline = (Get-Date).AddSeconds(45)
do {
    $remaining = @(Get-Process NetherNetMultiServer,bedrock_server -ErrorAction SilentlyContinue)
    if ($remaining.Count -eq 0) { break }
    Start-Sleep -Seconds 1
} while ((Get-Date) -lt $deadline)
if ($remaining.Count -ne 0) {
    throw 'Minecraft processes did not exit cleanly within 45 seconds.'
}
Write-ActivationLog 'Interactive-session stack stopped cleanly.'

try {
    Start-ScheduledTask -TaskName $taskName
    $deadline = (Get-Date).AddSeconds(60)
    do {
        Start-Sleep -Seconds 2
        $helper = @(Get-Process NetherNetMultiServer -ErrorAction SilentlyContinue)
        $servers = @(Get-Process bedrock_server -ErrorAction SilentlyContinue)
        if ($helper.Count -eq 1 -and $servers.Count -eq 2) { break }
    } while ((Get-Date) -lt $deadline)
    if ($helper.Count -ne 1 -or $servers.Count -ne 2) {
        throw "Background startup incomplete: helper=$($helper.Count), servers=$($servers.Count)."
    }
    $all = @($helper) + @($servers)
    $wrongSession = @($all | Where-Object SessionId -ne 0)
    if ($wrongSession.Count -ne 0) {
        throw "One or more Minecraft processes are not in background session 0: $($wrongSession.Id -join ', ')."
    }
    $info = Get-ScheduledTaskInfo -TaskName $taskName
    Write-ActivationLog "Background stack verified: helper=$($helper[0].Id), BDS=$($servers.Id -join ','), session=0, task_result=$($info.LastTaskResult)."
} catch {
    Write-ActivationLog "Background startup failed: $($_.Exception.Message)"
    Write-ActivationLog 'Attempting the normal startup script as a recovery action.'
    & $fallbackStart
    throw
}

Write-Host ''
Write-Host 'Background transition complete.' -ForegroundColor Green
Write-Host 'The helper and both servers now run independently of interactive sign-in.'
Write-Host ''
Read-Host 'Press Enter to close'
