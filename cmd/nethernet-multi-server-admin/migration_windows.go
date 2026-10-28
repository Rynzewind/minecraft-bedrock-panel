package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/migration"
	"nethernet-multi-server/internal/snapshotschedule"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type migrationPage struct {
	frequency, hour, minute, weekday *walk.ComboBox
	scheduleSave                     *walk.PushButton
	scheduleInfo                     *walk.Label
	weekly                           *walk.CheckBox
	settingsDir                      string
	settings                         snapshotschedule.Settings
	savingSchedule                   bool
	includeAddons                    *walk.CheckBox
	owner                            *adminWindow
	folder                           *walk.LineEdit
	message                          *walk.TextEdit
	progress                         *walk.ProgressBar
	create, cancel, open             *walk.PushButton
	jobID, path                      string
	watching                         bool
}

func (p *migrationPage) resume() {
	defer p.refreshScheduleInfo()
	ctx, cancel := timeoutContext()
	defer cancel()
	var s migration.JobStatus
	if e := p.owner.client.Call(ctx, "migration.status", adminapi.MigrationJobParams{}, &s); e != nil {
		return
	}
	p.owner.sync(func() {
		if s.Done {
			p.message.SetText(snapshotSummary(s))
			p.progress.SetValue(s.Progress)
			p.path = s.Path
			p.open.SetEnabled(s.Path != "")
		} else {
			p.watch(s.ID)
		}
	})
}

func (a *adminWindow) migrationTab() TabPage {
	p := &migrationPage{owner: a}
	a.migration = p
	exe, _ := os.Executable()
	p.settingsDir = filepath.Dir(exe)
	p.settings, _ = snapshotschedule.Load(p.settingsDir)
	defaultFolder := p.settings.OutputRoot
	var hours, minutes []string
	for i := 0; i < 24; i++ {
		hours = append(hours, fmt.Sprintf("%02d", i))
	}
	for i := 0; i < 60; i++ {
		minutes = append(minutes, fmt.Sprintf("%02d", i))
	}
	hour, minute := 0, 0
	fmt.Sscanf(p.settings.At, "%d:%d", &hour, &minute)
	return TabPage{Title: "Snapshots", Layout: VBox{}, Children: []Widget{
		TextLabel{Text: "Create a portable copy of both servers, worlds, packs and panel settings. Running servers briefly stop, then restart after copying."},
		CheckBox{AssignTo: &p.includeAddons, Alignment: AlignHNearVCenter, Text: "Include the complete add-on library and original downloads", ToolTipText: "Includes disabled add-ons and saved server selections", Checked: true},
		TextLabel{Text: "Without the library, server-installed packs and world pack selections are still preserved."},
		TextLabel{Text: "The Xbox LAN fix and 90-second login fix are preserved. Microsoft accounts must be signed in again on the new PC."},
		Composite{Layout: HBox{}, Children: []Widget{Label{Text: "Save snapshots to:"}, LineEdit{AssignTo: &p.folder, Text: defaultFolder, OnEditingFinished: p.saveFolder}, PushButton{Text: "Browse...", OnClicked: p.browse}}},
		CheckBox{AssignTo: &p.weekly, Alignment: AlignHNearVCenter, Text: "Create scheduled full snapshots", Checked: p.settings.Enabled, OnClicked: p.saveSchedule},
		Composite{Layout: HBox{}, Children: []Widget{
			Label{Text: "Frequency:"}, ComboBox{AssignTo: &p.frequency, Model: []string{"Daily", "Every 2 days", "Every 3 days", "Every 4 days", "Every 5 days", "Every 6 days", "Weekly"}, CurrentIndex: p.settings.IntervalDays - 1, OnCurrentIndexChanged: p.frequencyChanged},
			Label{Text: "At:"}, ComboBox{AssignTo: &p.hour, Model: hours, CurrentIndex: hour}, Label{Text: ":"}, ComboBox{AssignTo: &p.minute, Model: minutes, CurrentIndex: minute},
			Label{Text: "Weekly day:"}, ComboBox{AssignTo: &p.weekday, Model: []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}, CurrentIndex: (int(p.settings.Weekday) + 6) % 7, Enabled: p.settings.IntervalDays == 7},
			PushButton{AssignTo: &p.scheduleSave, Text: "Save schedule", OnClicked: p.saveSchedule},
		}},
		Label{AssignTo: &p.scheduleInfo, Text: "Automatic schedule status is loading..."},
		TextLabel{Text: "Manual snapshots stay in Protected. Automatic snapshots keep 7 verified copies in Scheduled.\r\nAutomatic backups warn players for 5 minutes, then save/restart even with players online. Times use the Windows local clock; sign-in required."},
		Composite{Layout: HBox{}, Children: []Widget{PushButton{AssignTo: &p.create, Text: "Create protected snapshot", OnClicked: p.start}, PushButton{AssignTo: &p.cancel, Text: "Cancel snapshot", Enabled: false, OnClicked: p.cancelJob}, PushButton{AssignTo: &p.open, Text: "Open output folder", Enabled: false, OnClicked: p.openFolder}, PushButton{Text: "Repair Network Configuration", OnClicked: p.repairNetwork}}},
		ProgressBar{AssignTo: &p.progress, MaxValue: 100},
		TextEdit{AssignTo: &p.message, ReadOnly: true, Text: "Choose an output folder outside the Minecraft installation. Keep snapshots private: they include player data and server identities.", MinSize: Size{Height: 100}},
		VSpacer{},
	}}
}
func (p *migrationPage) browse() {
	dlg := &walk.FileDialog{Title: "Choose a snapshot folder", FilePath: p.folder.Text()}
	if ok, e := dlg.ShowBrowseFolder(p.owner.mw); e == nil && ok {
		p.folder.SetText(dlg.FilePath)
		p.saveFolder()
	}
}
func (p *migrationPage) start() {
	if p.watching {
		return
	}
	folder := p.folder.Text()
	p.create.SetEnabled(false)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var servers []supervisor.Status
		e := p.owner.client.Call(ctx, "servers.list", struct{}{}, &servers)
		p.owner.sync(func() {
			if e != nil {
				p.create.SetEnabled(true)
				p.message.SetText(e.Error())
				return
			}
			text := "The servers will stop briefly to capture consistent worlds, then previously running servers will restart.\r\n\r\n"
			for _, s := range servers {
				text += fmt.Sprintf("%s: %s, %d connected player(s)\r\n", s.Name, s.State, s.Players)
			}
			text += "\r\nCreate a snapshot now?"
			if walk.MsgBox(p.owner.mw, "Create migration snapshot", text, walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
				p.create.SetEnabled(true)
				return
			}
			p.begin(folder)
		})
	}()
}
func (p *migrationPage) begin(folder string) {
	folder = snapshotschedule.ProtectedDestination(folder, time.Now())
	includeAddons := p.includeAddons.Checked()
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		var result struct {
			JobID string `json:"job_id"`
		}
		e := p.owner.client.Call(ctx, "migration.start", adminapi.MigrationStartParams{OutputDirectory: folder, IncludeAddons: &includeAddons, Kind: "manual"}, &result)
		p.owner.sync(func() {
			if e != nil {
				p.create.SetEnabled(true)
				p.message.SetText(e.Error())
				return
			}
			p.watch(result.JobID)
		})
	}()
}
func (p *migrationPage) watch(id string) {
	if p.watching {
		return
	}
	p.jobID = id
	p.watching = true
	p.create.SetEnabled(false)
	p.cancel.SetEnabled(true)
	p.open.SetEnabled(false)
	go func() {
		for !p.owner.closed.Load() {
			ctx, cancel := timeoutContext()
			var s migration.JobStatus
			e := p.owner.client.Call(ctx, "migration.status", adminapi.MigrationJobParams{JobID: id}, &s)
			cancel()
			p.owner.sync(func() {
				if e != nil {
					p.message.SetText("Snapshot status unavailable: " + e.Error())
					return
				}
				p.jobID = s.ID
				p.progress.SetValue(s.Progress)
				p.message.SetText(snapshotSummary(s))
				if s.Done {
					p.path = s.Path
					p.watching = false
					p.create.SetEnabled(true)
					p.cancel.SetEnabled(false)
					p.open.SetEnabled(s.Path != "")
				}
			})
			if e == nil && s.Done {
				return
			}
			time.Sleep(time.Second)
		}
	}()
}
func (p *migrationPage) cancelJob() {
	id := p.jobID
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		e := p.owner.client.Call(ctx, "migration.cancel", adminapi.MigrationJobParams{JobID: id}, nil)
		if e != nil {
			p.owner.sync(func() { p.message.SetText(e.Error()) })
		}
	}()
}
func (p *migrationPage) openFolder() {
	if p.path != "" {
		_ = exec.Command("explorer.exe", filepath.Dir(p.path)).Start()
	}
}
func (p *migrationPage) repairNetwork() {
	exe, e := os.Executable()
	if e != nil {
		return
	}
	root := filepath.Dir(filepath.Dir(exe))
	wizard := filepath.Join(root, "MultiServer", "installer", "setup-assets", "Setup-Wizard.ps1")
	if _, e = os.Stat(wizard); e != nil {
		p.message.SetText("Install the complete updated installer under MultiServer/installer first: " + e.Error())
		return
	}
	command := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", wizard, "-RepairRoot", root)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if e = command.Start(); e != nil {
		p.message.SetText(e.Error())
	}
}

func (p *migrationPage) saveFolder() {
	if p.folder == nil || p.savingSchedule {
		return
	}
	next := p.settings
	next.OutputRoot = p.folder.Text()
	if err := snapshotschedule.Save(p.settingsDir, next); err != nil {
		p.message.SetText("Snapshot folder was not saved: " + err.Error())
		return
	}
	p.settings = next
}
func (p *migrationPage) saveSchedule() {
	if p.savingSchedule {
		return
	}
	next := p.settings
	next.OutputRoot = p.folder.Text()
	next.Enabled = p.weekly.Checked()
	next.IntervalDays = p.frequency.CurrentIndex() + 1
	next.At = fmt.Sprintf("%02d:%02d", p.hour.CurrentIndex(), p.minute.CurrentIndex())
	next.Weekday = time.Weekday((p.weekday.CurrentIndex() + 1) % 7)
	p.savingSchedule = true
	p.weekly.SetEnabled(false)
	p.folder.SetEnabled(false)
	p.scheduleSave.SetEnabled(false)
	p.frequency.SetEnabled(false)
	p.hour.SetEnabled(false)
	p.minute.SetEnabled(false)
	p.weekday.SetEnabled(false)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		err := snapshotschedule.Configure(ctx, p.settingsDir, next)
		p.owner.sync(func() {
			p.savingSchedule = false
			p.weekly.SetEnabled(true)
			p.folder.SetEnabled(true)
			p.scheduleSave.SetEnabled(true)
			p.frequency.SetEnabled(true)
			p.hour.SetEnabled(true)
			p.minute.SetEnabled(true)
			p.frequencyChanged()
			if err != nil {
				p.weekly.SetChecked(p.settings.Enabled)
				p.message.SetText("Schedule was not changed: " + err.Error())
				return
			}
			p.settings = next
			if next.Enabled {
				p.message.SetText(next.Description() + ", local Windows time. Full snapshots keep 7 verified copies in Scheduled dated folders under " + next.OutputRoot + ". The first run is at the next selected time; Windows keeps the interval between runs. Details: snapshot-schedule.log beside the panel.")
			} else {
				p.message.SetText("Automatic snapshots disabled. Your frequency and time choices have been saved.")
			}
			go p.refreshScheduleInfo()
		})
	}()
}

func (p *migrationPage) frequencyChanged() {
	if p.weekday != nil && p.frequency != nil {
		p.weekday.SetEnabled(!p.savingSchedule && p.frequency.CurrentIndex() == 6)
	}
}
func (p *migrationPage) refreshScheduleInfo() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := snapshotschedule.Info(ctx, p.settingsDir)
	p.owner.sync(func() {
		if err != nil {
			p.scheduleInfo.SetText("Could not read the Windows task status. See Activity log.")
			p.owner.appendEvent(err.Error())
			return
		}
		p.scheduleInfo.SetText(info.Description())
	})
}

func (p *migrationPage) pollScheduleInfo() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		if p.owner.closed.Load() {
			return
		}
		p.refreshScheduleInfo()
	}
}
