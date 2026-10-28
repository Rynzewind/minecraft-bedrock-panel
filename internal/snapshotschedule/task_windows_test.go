//go:build windows

package snapshotschedule

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsTaskUsesMondayMidnightAndFullSnapshotCommand(t *testing.T) {
	mock := `function Get-ScheduledTask { [CmdletBinding()]param($TaskName);return $null }
 function Register-ScheduledTask {[CmdletBinding()]param($TaskName,$Action,$Trigger,$Principal,$Settings,[switch]$Force)
 if($Action.Arguments -ne 'snapshot-scheduled' -or $Action.Execute -ne $env:NN_SNAPSHOT_EXE){throw 'Wrong scheduled command'}
 if($env:NN_SNAPSHOT_DAYS -eq '7'){if($Trigger.DaysOfWeek -ne 2){throw 'Wrong weekly day'}}else{if($Trigger.DaysInterval -ne [int]$env:NN_SNAPSHOT_DAYS){throw 'Wrong day interval'}}
 if(([datetime]$Trigger.StartBoundary).Hour -ne 3 -or ([datetime]$Trigger.StartBoundary).Minute -ne 15){throw 'Wrong backup time'}
 if(-not $Settings.StartWhenAvailable -or $Settings.MultipleInstances -ne 2){throw 'Unsafe overlap/missed-run policy'}
 $global:verified=$true
 }
 `
	command := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", "Import-Module ScheduledTasks;\n"+mock+taskScript+"\nif(-not $global:verified){throw 'Task not registered'};'task specification verified'")
	command.Env = append(os.Environ(), "NN_SNAPSHOT_TASK=NetherNet-Test-Only", `NN_SNAPSHOT_EXE=C:\Minecraft Servers\MultiServer\NetherNetMultiServerControl.exe`, "NN_SNAPSHOT_ENABLED=true", "NN_SNAPSHOT_DAYS=7", "NN_SNAPSHOT_START=2026-10-26T03:15:00", "NN_SNAPSHOT_WEEKDAY=Monday")
	result, err := command.CombinedOutput()
	if err != nil || !strings.Contains(string(result), "task specification verified") {
		t.Fatalf("%v: %s", err, result)
	}
}
func TestWindowsTaskRefusesForeignCommand(t *testing.T) {
	mock := `function Get-ScheduledTask {[CmdletBinding()]param($TaskName);[pscustomobject]@{Actions=@([pscustomobject]@{Execute='C:\Other\other.exe';Arguments=''})}}
 function Unregister-ScheduledTask {throw 'SHOULD NOT REMOVE FOREIGN TASK'}
 `
	command := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", "Import-Module ScheduledTasks;\n"+mock+taskScript+"\nif(-not $global:verified){throw 'Task not registered'};'task specification verified'")
	command.Env = append(os.Environ(), "NN_SNAPSHOT_TASK=NetherNet-Test-Only", `NN_SNAPSHOT_EXE=C:\Minecraft Servers\MultiServer\NetherNetMultiServerControl.exe`, "NN_SNAPSHOT_ENABLED=false")
	result, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(result), "belongs to another command") {
		t.Fatalf("%v: %s", err, result)
	}
}

func TestDailyAndMultiDayWindowsTriggers(t *testing.T) {
	for _, days := range []string{"1", "2", "3", "4", "5", "6"} {
		t.Run(days, func(t *testing.T) {
			script := `Import-Module ScheduledTasks
 function Get-ScheduledTask {[CmdletBinding()]param($TaskName);return $null}
 function Register-ScheduledTask {[CmdletBinding()]param($TaskName,$Action,$Trigger,$Principal,$Settings,[switch]$Force)
 if($Trigger.DaysInterval -ne [int]$env:NN_SNAPSHOT_DAYS -or ([datetime]$Trigger.StartBoundary).Hour -ne 3 -or ([datetime]$Trigger.StartBoundary).Minute -ne 15){throw 'Wrong daily interval/time'}
 $global:verified=$true
 }
 ` + taskScript + `if(-not $global:verified){throw 'Task not registered'};'verified'`
			command := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", script)
			command.Env = append(os.Environ(), "NN_SNAPSHOT_TASK=NetherNet-Test-Only", `NN_SNAPSHOT_EXE=C:\Minecraft Servers\MultiServer\NetherNetMultiServerControl.exe`, "NN_SNAPSHOT_ENABLED=true", "NN_SNAPSHOT_DAYS="+days, "NN_SNAPSHOT_START=2026-10-26T03:15:00", "NN_SNAPSHOT_WEEKDAY=Monday")
			result, err := command.CombinedOutput()
			if err != nil || !strings.Contains(string(result), "verified") {
				t.Fatalf("%v: %s", err, result)
			}
		})
	}
}
