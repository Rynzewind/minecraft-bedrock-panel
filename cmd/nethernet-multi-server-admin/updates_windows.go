package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/serverupdate"
	"os/exec"
	"strings"
	"time"
)

type updatesPage struct {
	progress                        *serverProgressDialog
	tabs                            *walk.TabWidget
	a                               *adminWindow
	summary                         *walk.TextEdit
	check, install, recover, backup *walk.PushButton
	pack                            *walk.ComboBox
	source                          *walk.LineEdit
	sourceInfo                      *walk.TextLabel
	release                         serverupdate.Check
	last                            serverupdate.Status
	busy                            bool
	packs                           []contentpacks.InventoryItem
	sources                         map[string]string
}

func (a *adminWindow) updatesTab() TabPage {
	p := &updatesPage{a: a}
	a.updates = p
	servers := TabPage{Title: "Servers", Layout: VBox{}, Children: []Widget{TextLabel{Text: "Update both servers together. Worlds, add-ons, accounts and settings are backed up and preserved."}, Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{AssignTo: &p.check, Text: "Check for updates", OnClicked: p.checkUpdates}, PushButton{AssignTo: &p.install, Text: "Update servers...", Enabled: false, OnClicked: p.start}, PushButton{AssignTo: &p.backup, Text: "Open backup", Enabled: false, OnClicked: p.openBackup}, PushButton{AssignTo: &p.recover, Text: "Recover backup...", Enabled: false, OnClicked: p.restore}}}, TextEdit{AssignTo: &p.summary, ReadOnly: true, MinSize: Size{Height: 140}, StretchFactor: 1, Text: "Check for the latest official stable release. Updates run only when everyone has left the servers.\r\nFuture releases need matching NetherNet protocol support."}}}
	addons := TabPage{Title: "Add-ons", Layout: VBox{}, Children: []Widget{TextLabel{Text: "Save each pack's official project page. Automatic CurseForge checks require API access; source pages are checked manually for now."}, ComboBox{AssignTo: &p.pack, Model: []string{"Load installed add-ons..."}, CurrentIndex: 0, OnCurrentIndexChanged: p.selectPack}, Label{Text: "Official project page:"}, LineEdit{AssignTo: &p.source, ToolTipText: "HTTPS project page, such as CurseForge or the author's website"}, Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Text: "Save source", OnClicked: p.saveSource}, PushButton{Text: "Open source", OnClicked: p.openSource}, PushButton{Text: "Reload add-ons", OnClicked: p.loadSources}}}, TextLabel{AssignTo: &p.sourceInfo, Text: "Select an add-on and save its source page. Pack names alone cannot reliably identify projects."}, VSpacer{}}}
	return TabPage{Title: "Updates", Layout: VBox{MarginsZero: true}, Children: []Widget{TabWidget{AssignTo: &p.tabs, Pages: []TabPage{scrollableAdminPage(servers), scrollableAdminPage(addons)}}}}
}
func (p *updatesPage) showError(e error) { p.summary.SetText(e.Error()) }
func (p *updatesPage) checkUpdates() {
	if p.busy {
		return
	}
	p.busy = true
	p.check.SetEnabled(false)
	p.install.SetEnabled(false)
	p.summary.SetText("Checking official stable releases...")
	p.readStatus()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var out serverupdate.Check
		e := p.a.client.Call(ctx, "updates.check", struct{}{}, &out)
		var s serverupdate.Status
		if e == nil {
			e = p.a.client.Call(ctx, "updates.status", struct{}{}, &s)
		}
		p.a.sync(func() {
			p.busy = false
			p.check.SetEnabled(true)
			if e != nil {
				p.showError(e)
				return
			}
			p.release = out
			p.last = s
			var b strings.Builder
			for _, v := range out.Installed {
				fmt.Fprintf(&b, "%s: %s\r\n", v.ID, v.Version)
			}
			fmt.Fprintf(&b, "Available: %s\r\nNetherNet supports: %s (protocol %d)\r\n", out.Release.Version, out.Release.HelperVersion, out.Release.Protocol)
			if !out.Release.Compatible {
				b.WriteString(out.Release.Reason)
			} else if !out.Needed {
				b.WriteString("Both servers are up to date.")
			} else {
				b.WriteString("Compatible update available. Review the update preview before installation.")
			}
			if s.Recovery {
				b.WriteString("\r\nRecovery required: " + s.Message)
			}
			p.summary.SetText(b.String())
			p.install.SetEnabled(out.Release.Compatible && out.Needed && !s.Recovery && s.Done)
			p.backup.SetEnabled(s.Backup != "")
			p.recover.SetEnabled(s.Recovery && s.Backup != "")
			if !s.Done {
				p.runningProgress()
			}
		})
	}()
	p.loadSources()
}
func (p *updatesPage) start() {
	if p.busy || !p.release.Release.Compatible {
		return
	}
	r := p.release.Release
	text := fmt.Sprintf("Update both servers to %s from the official Minecraft download?\r\n\r\nServers will stop cleanly. A verified local backup will be created before replacement. Worlds, add-ons, settings and accounts are retained. Previously running servers will restart.\r\n\r\nNo players may be connected. Add-on compatibility is not guaranteed by the Minecraft version check.\r\n\r\n%s", r.Version, r.URL)
	if walk.MsgBox(p.a.mw, "Preview Minecraft server update", text, walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	view, e := newUpdateProgressDialog(p.a.mw)
	if e != nil {
		p.showError(e)
		return
	}
	p.progress = view
	defer func() { p.progress = nil; view.dlg.Dispose() }()
	p.busy = true
	p.install.SetEnabled(false)
	p.check.SetEnabled(false)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var s serverupdate.Status
		e := p.a.client.Call(ctx, "updates.start", map[string]string{"version": r.Version}, &s)
		if e != nil {
			p.a.sync(func() {
				p.busy = false
				p.check.SetEnabled(true)
				p.showError(e)
				view.renderUpdate(serverupdate.Status{Phase: "failed", Done: true, Message: e.Error()}, time.Now(), e.Error())
			})
			return
		}
		p.watch()
	}()
	view.dlg.Run()
}
func (p *updatesPage) watch() {
	started := time.Now()
	history := ""
	lastPhase := ""
	for {
		if p.a.closed.Load() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var s serverupdate.Status
		e := p.a.client.Call(ctx, "updates.status", struct{}{}, &s)
		cancel()
		p.a.sync(func() {
			if e != nil {
				p.summary.SetText("Unable to read update status: " + e.Error())
				if p.progress != nil {
					p.progress.renderUpdate(serverupdate.Status{Done: true, Phase: "failed", Message: "Connection interrupted. The server update continues in the helper. Close and check again to reconnect."}, started, history)
				}
				return
			}
			p.last = s
			if s.Phase != lastPhase {
				history += time.Now().Format("15:04:05") + "  " + updatePhaseTitle(s.Phase) + "\r\n"
				lastPhase = s.Phase
			}
			if p.progress != nil {
				p.progress.renderUpdate(s, started, history)
			}
			p.summary.SetText(fmt.Sprintf("%s\r\n%s\r\nBackup: %s\r\nArchive SHA-256: %s", s.Phase, s.Message, s.Backup, s.SHA256))
			p.backup.SetEnabled(s.Backup != "")
			p.recover.SetEnabled(s.Done && s.Recovery && s.Backup != "")
			if s.Done {
				p.busy = false
				p.check.SetEnabled(true)
			}
		})
		if e != nil {
			p.a.sync(func() { p.busy = false; p.check.SetEnabled(true) })
			return
		}
		if s.Done {
			return
		}
		time.Sleep(time.Second)
	}
}
func (p *updatesPage) openBackup() {
	if p.last.Backup != "" {
		_ = exec.Command("explorer.exe", p.last.Backup).Start()
	}
}
func (p *updatesPage) restore() {
	if p.busy {
		return
	}
	if walk.MsgBox(p.a.mw, "Recover pre-update worlds", "Restore the pre-update server backup? Any play since the update will be absent from the restored worlds. The displaced current files will be retained in the backup folder.\r\n\r\nAll servers must be stopped. The older server requires its matching NetherNet helper before use.", walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	p.busy = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
		defer cancel()
		var s serverupdate.Status
		e := p.a.client.Call(ctx, "updates.recover", map[string]bool{"confirm": true}, &s)
		p.a.sync(func() {
			p.busy = false
			if e != nil {
				p.showError(e)
			} else {
				p.last = s
				p.summary.SetText(s.Message)
				p.recover.SetEnabled(false)
			}
		})
	}()
}
func (p *updatesPage) loadSources() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var packs []contentpacks.InventoryItem
		var sources map[string]string
		e := p.a.client.Call(ctx, "packs.list", struct{}{}, &packs)
		if e == nil {
			e = p.a.client.Call(ctx, "addons.sources.list", struct{}{}, &sources)
		}
		p.a.sync(func() {
			if e != nil {
				p.sourceInfo.SetText(e.Error())
				return
			}
			p.packs = packs
			p.sources = sources
			names := []string{}
			for _, v := range packs {
				names = append(names, fmt.Sprintf("%s — %d.%d.%d", v.Name, v.Version[0], v.Version[1], v.Version[2]))
			}
			p.pack.SetModel(names)
			if len(names) > 0 {
				p.pack.SetCurrentIndex(0)
				p.selectPack()
			} else {
				p.sourceInfo.SetText("No add-ons in the library.")
			}
		})
	}()
}
func (p *updatesPage) selectPack() {
	if p.pack == nil || p.source == nil {
		return
	}
	i := p.pack.CurrentIndex()
	if i < 0 || i >= len(p.packs) {
		return
	}
	u := p.sources[p.packs[i].UUID]
	p.source.SetText(u)
	if u == "" {
		p.sourceInfo.SetText("No source linked. Automatic update status is unknown.")
	} else {
		p.sourceInfo.SetText("Source linked. Open the page to check releases; automatic API checks are not enabled.")
	}
}
func (p *updatesPage) saveSource() {
	i := p.pack.CurrentIndex()
	if i < 0 || i >= len(p.packs) {
		return
	}
	id := p.packs[i].UUID
	u := strings.TrimSpace(p.source.Text())
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		e := p.a.client.Call(ctx, "addons.sources.set", map[string]string{"uuid": id, "url": u}, nil)
		p.a.sync(func() {
			if e != nil {
				p.sourceInfo.SetText(e.Error())
			} else {
				p.sources[id] = u
				p.sourceInfo.SetText("Source saved. Open its page to review available releases.")
			}
		})
	}()
}
func (p *updatesPage) openSource() {
	i := p.pack.CurrentIndex()
	if i < 0 || i >= len(p.packs) {
		return
	}
	if u := p.sources[p.packs[i].UUID]; u != "" {
		_ = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", u).Start()
	}
}

func (p *updatesPage) readStatus() {
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		var s serverupdate.Status
		e := p.a.client.Call(ctx, "updates.status", struct{}{}, &s)
		p.a.sync(func() {
			if e != nil {
				return
			}
			p.last = s
			p.backup.SetEnabled(s.Backup != "")
			p.recover.SetEnabled(s.Recovery && s.Done && s.Backup != "")
			if s.Recovery {
				p.summary.SetText(s.Message + "\r\nBackup: " + s.Backup)
			}
			if !s.Done && p.progress == nil {
				p.runningProgress()
			}
		})
	}()
}
