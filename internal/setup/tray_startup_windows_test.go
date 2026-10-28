package setup

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogonStartsAdminDirectlyAndBootKeepsWatcher(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "setup-assets", "Configure-Windows.ps1")
	command := `. '` + script + `'; $a=Get-NNStartupActionSpec 'C:\Minecraft Servers' 'logon';if($a.Execute -ne 'C:\Minecraft Servers\MultiServer\NetherNetMultiServerAdmin.exe' -or $a.Arguments -ne '--startup'){throw 'Logon must start tray admin directly'};$b=Get-NNStartupActionSpec 'C:\Minecraft Servers' 'boot';if($b.Arguments -notmatch 'Watch-NetherNet.ps1'){throw 'Boot watcher changed'};'verified'`
	c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command)
	out, err := c.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "verified") {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestLegacyStartupOwnershipExcludesSharedAndUnrelatedTasks(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "setup-assets", "Configure-Windows.ps1")
	command := `. '` + script + `';$root='C:\Minecraft Servers';$owned=[pscustomobject]@{TaskName='legacy';Triggers=@([pscustomobject]@{CimClass=[pscustomobject]@{CimClassName='MSFT_TaskLogonTrigger'}});Actions=@([pscustomobject]@{Execute='C:\Minecraft Servers\StartMinecraftServers.bat';Arguments=''})};$other=[pscustomobject]@{TaskName='other';Triggers=$owned.Triggers;Actions=@([pscustomobject]@{Execute='C:\Other\StartMinecraftServers.bat';Arguments=''})};$shared=[pscustomobject]@{TaskName='shared';Triggers=$owned.Triggers;Actions=@($owned.Actions[0],$other.Actions[0])};$maintenance=[pscustomobject]@{TaskName='daily';Triggers=@([pscustomobject]@{CimClass=[pscustomobject]@{CimClassName='MSFT_TaskDailyTrigger'}});Actions=$owned.Actions};$result=@(Select-NNLegacyStartupTasks @($owned,$other,$shared,$maintenance) $root 'new');if($result.Count -ne 1 -or $result[0].TaskName -ne 'legacy'){throw 'Unsafe legacy task selection'};if(Test-NNStartupOwned "$env:SystemRoot\System32\cmd.exe" '/c "C:\Minecraft Servers\StartMinecraftServers.bat" & notepad.exe' $root){throw 'Accepted shared shell command'};'verified'`
	out, err := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", command).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "verified") {
		t.Fatalf("%v: %s", err, out)
	}
}
