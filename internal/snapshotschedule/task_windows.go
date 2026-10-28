//go:build windows

package snapshotschedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func TaskName(directory string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(directory))))
	return "NetherNet-WeeklySnapshot-" + hex.EncodeToString(hash[:8])
}

const taskScript = `
$ErrorActionPreference='Stop'
$name=$env:NN_SNAPSHOT_TASK
$exe=$env:NN_SNAPSHOT_EXE
$existing=Get-ScheduledTask -TaskName $name -ErrorAction SilentlyContinue
if($existing){$actions=@($existing.Actions);if($actions.Count -ne 1 -or $actions[0].Execute -ne $exe -or $actions[0].Arguments -ne 'snapshot-scheduled'){throw 'The scheduled task name belongs to another command.'}}
if($env:NN_SNAPSHOT_ENABLED -eq 'true'){
 $action=New-ScheduledTaskAction -Execute $exe -Argument 'snapshot-scheduled' -WorkingDirectory (Split-Path $exe -Parent)
 $days=[int]$env:NN_SNAPSHOT_DAYS
 if($days -lt 1 -or $days -gt 7){throw 'Backup interval must be 1 to 7 days.'}
 $at=[datetime]::ParseExact($env:NN_SNAPSHOT_START,'yyyy-MM-ddTHH:mm:ss',[Globalization.CultureInfo]::InvariantCulture)
 if($days -eq 7){$trigger=New-ScheduledTaskTrigger -Weekly -DaysOfWeek $env:NN_SNAPSHOT_WEEKDAY -At $at}
 else{$trigger=New-ScheduledTaskTrigger -Daily -DaysInterval $days -At $at}
 $account=[Security.Principal.WindowsIdentity]::GetCurrent().Name
 $principal=New-ScheduledTaskPrincipal -UserId $account -LogonType Interactive -RunLevel Limited
 $settings=New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable -MultipleInstances IgnoreNew -ExecutionTimeLimit (New-TimeSpan -Hours 12)
 Register-ScheduledTask -TaskName $name -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force|Out-Null
}else{if($existing){Unregister-ScheduledTask -TaskName $name -Confirm:$false}}
`

func Configure(ctx context.Context, directory string, s Settings) error {
	s = s.normalized()
	start, err := NextStart(s, time.Now())
	if err != nil {
		return err
	}
	old, err := Load(directory)
	if err != nil {
		return err
	}
	if err = Save(directory, s); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", taskScript)
	command.Env = append(os.Environ(), "NN_SNAPSHOT_TASK="+TaskName(directory), "NN_SNAPSHOT_EXE="+filepath.Join(directory, "NetherNetMultiServerControl.exe"), fmt.Sprintf("NN_SNAPSHOT_ENABLED=%t", s.Enabled), fmt.Sprintf("NN_SNAPSHOT_DAYS=%d", s.IntervalDays), "NN_SNAPSHOT_START="+start.Format("2006-01-02T15:04:05"), "NN_SNAPSHOT_WEEKDAY="+s.Weekday.String())
	configureHidden(command)
	if output, e := command.CombinedOutput(); e != nil {
		_ = Save(directory, old)
		return fmt.Errorf("snapshot schedule: %s: %w", strings.TrimSpace(string(output)), e)
	}
	return nil
}

const infoScript = `
$ErrorActionPreference='Stop'
$task=Get-ScheduledTask -TaskName $env:NN_SNAPSHOT_TASK -ErrorAction SilentlyContinue
if(-not $task){@{exists=$false}|ConvertTo-Json -Compress;exit 0}
$actions=@($task.Actions)
if($actions.Count -ne 1 -or $actions[0].Execute -ne $env:NN_SNAPSHOT_EXE -or $actions[0].Arguments -ne 'snapshot-scheduled'){throw 'Task ownership differs from this installation.'}
$info=Get-ScheduledTaskInfo -TaskName $env:NN_SNAPSHOT_TASK
$last='';$next=''
if($info.LastRunTime.Year -ge 2000){$last=$info.LastRunTime.ToString('yyyy-MM-dd HH:mm')}
if($info.NextRunTime.Year -ge 2000){$next=$info.NextRunTime.ToString('yyyy-MM-dd HH:mm')}
@{exists=$true;last_run=$last;next_run=$next;result=[long]$info.LastTaskResult}|ConvertTo-Json -Compress
`

type TaskInfo struct {
	Exists  bool   `json:"exists"`
	LastRun string `json:"last_run"`
	NextRun string `json:"next_run"`
	Result  int64  `json:"result"`
}

func Info(ctx context.Context, directory string) (TaskInfo, error) {
	var info TaskInfo
	command := exec.CommandContext(ctx, filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", infoScript)
	command.Env = append(os.Environ(), "NN_SNAPSHOT_TASK="+TaskName(directory), "NN_SNAPSHOT_EXE="+filepath.Join(directory, "NetherNetMultiServerControl.exe"))
	configureHidden(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return info, fmt.Errorf("read snapshot task: %s: %w", strings.TrimSpace(string(output)), err)
	}
	err = json.Unmarshal(output, &info)
	return info, err
}
func (info TaskInfo) Description() string {
	if !info.Exists {
		return "No automatic snapshot task is installed. Save the schedule to apply it."
	}
	last := info.LastRun
	if last == "" {
		last = "Never"
	}
	next := info.NextRun
	if next == "" {
		next = "Not scheduled"
	}
	result := "success"
	if info.Result == 267009 {
		result = "running"
	} else if info.Result == 267011 {
		result = "not run yet"
	} else if info.Result != 0 {
		result = fmt.Sprintf("result %d; see snapshot-schedule.log", info.Result)
	}
	return fmt.Sprintf("Last scheduled attempt: %s (%s).\r\nNext scheduled run: %s (local Windows time).", last, result, next)
}
