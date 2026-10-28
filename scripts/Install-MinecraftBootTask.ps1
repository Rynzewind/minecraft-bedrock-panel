[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$taskName = 'NetherNet Minecraft Servers'
$root = 'C:\MinecraftServers'
$startScript = Join-Path $root 'StartMinecraftServers.bat'
$multiRoot = Join-Path $root 'MultiServer'
$logPath = Join-Path $multiRoot 'startup-task-install.log'

function Write-InstallLog([string]$Message) {
    $line = '[{0}] {1}' -f (Get-Date -Format 'yyyy-MM-dd HH:mm:ss'), $Message
    Add-Content -LiteralPath $logPath -Value $line -Encoding UTF8
    Write-Host $line
}

$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
    throw 'This installer must be run as Administrator.'
}
if (-not (Test-Path -LiteralPath $startScript)) {
    throw "Startup script not found: $startScript"
}

Write-InstallLog "Preparing scheduled task '$taskName'."
$credential = Get-Credential -UserName $identity.Name -Message 'Enter your Windows account PASSWORD (not your PIN). It is passed directly to Windows Task Scheduler and is not written to a file.'
if ($null -eq $credential) {
    throw 'Credential entry was cancelled.'
}

$backupRoot = Join-Path $multiRoot ('backups\scheduled-tasks-' + (Get-Date -Format 'yyyyMMdd-HHmmss'))
New-Item -ItemType Directory -Path $backupRoot -Force | Out-Null

$related = @(Get-ScheduledTask | Where-Object {
    $_.TaskName -eq $taskName -or
    (($_.Actions.Execute -join ' ') + ' ' + ($_.Actions.Arguments -join ' ')) -match 'StartMinecraftServers|NetherNetMultiServer'
})
foreach ($existing in $related) {
    $safeName = ($existing.TaskPath.Trim('\') + '_' + $existing.TaskName) -replace '[\\/:*?"<>|]', '_'
    Export-ScheduledTask -TaskName $existing.TaskName -TaskPath $existing.TaskPath |
        Set-Content -LiteralPath (Join-Path $backupRoot ($safeName + '.xml')) -Encoding UTF8
    Write-InstallLog "Backed up existing task $($existing.TaskPath)$($existing.TaskName)."
}

$action = New-ScheduledTaskAction `
    -Execute "$env:SystemRoot\System32\cmd.exe" `
    -Argument ('/d /c ""{0}""' -f $startScript) `
    -WorkingDirectory $root
$trigger = New-ScheduledTaskTrigger -AtStartup
$trigger.Delay = 'PT30S'
$settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 5 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -ExecutionTimeLimit ([TimeSpan]::Zero) `
    -MultipleInstances IgnoreNew
$taskPrincipal = New-ScheduledTaskPrincipal `
    -UserId $credential.UserName `
    -LogonType Password `
    -RunLevel Highest
$definition = New-ScheduledTask `
    -Action $action `
    -Trigger $trigger `
    -Settings $settings `
    -Principal $taskPrincipal `
    -Description 'Starts the NetherNet helper and both Minecraft Bedrock servers at Windows boot, independent of interactive sign-in.'

$password = $credential.GetNetworkCredential().Password
try {
    Register-ScheduledTask -TaskName $taskName -InputObject $definition -User $credential.UserName -Password $password -Force | Out-Null
} finally {
    $password = $null
    $credential = $null
}
Write-InstallLog "Registered task for $($taskPrincipal.UserId) with Password logon at system startup."

foreach ($existing in $related) {
    if ($existing.TaskName -ne $taskName -or $existing.TaskPath -ne '\') {
        Disable-ScheduledTask -TaskName $existing.TaskName -TaskPath $existing.TaskPath | Out-Null
        Write-InstallLog "Disabled duplicate startup task $($existing.TaskPath)$($existing.TaskName)."
    }
}

Start-ScheduledTask -TaskName $taskName
Start-Sleep -Seconds 5
$task = Get-ScheduledTask -TaskName $taskName
$info = Get-ScheduledTaskInfo -TaskName $taskName
Write-InstallLog "Verification: state=$($task.State), last_result=$($info.LastTaskResult), next_run=$($info.NextRunTime)."

Write-Host ''
Write-Host 'Installation complete.' -ForegroundColor Green
Write-Host "Task: $taskName"
Write-Host "Account: $($task.Principal.UserId)"
Write-Host "Logon type: $($task.Principal.LogonType)"
Write-Host "Existing task backups: $backupRoot"
Write-Host 'The currently running Minecraft processes were not stopped.'
Write-Host ''
Read-Host 'Press Enter to close'
