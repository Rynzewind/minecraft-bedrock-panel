package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"strconv"
	"strings"
	"time"
)

func (p *updatesPage) addonSettingsTab() TabPage {
	return TabPage{Title: "Add-ons", Layout: VBox{}, Children: []Widget{
		TextLabel{Text: "Check official releases or review a downloaded archive. Staging preserves deployed versions; use Content Packs to apply changes when a server is stopped."},
		GroupBox{Title: "CurseForge API access (optional)", Layout: VBox{}, Children: []Widget{
			TextLabel{AssignTo: &p.keyInfo, Text: "No API status loaded. Keys are encrypted for this Windows user and excluded from portable snapshots."},
			LineEdit{AssignTo: &p.apiKey, PasswordMode: true, ToolTipText: "Enter a new API key. Existing keys are never displayed."},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Text: "Save / replace key", OnClicked: p.saveKey}, PushButton{Text: "Test saved key", OnClicked: func() {
				p.addonCall("Testing API access...", "addons.credentials.test", struct{}{}, nil, func() { p.loadSources() })
			}}, PushButton{Text: "Remove key", OnClicked: p.removeKey}}},
		}},
		ComboBox{AssignTo: &p.pack, Model: []string{"Load installed add-ons..."}, CurrentIndex: 0, OnCurrentIndexChanged: p.selectPack},
		TextLabel{AssignTo: &p.sourceInfo, Text: "Select an add-on. Project names alone cannot reliably identify a source."},
		Label{Text: "Official HTTPS project page:"}, LineEdit{AssignTo: &p.source},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{Label{Text: "CurseForge project ID (optional):"}, LineEdit{AssignTo: &p.projectID}, Label{Text: "Release channel:"}, ComboBox{AssignTo: &p.channel, Model: []string{"stable", "beta", "alpha"}, CurrentIndex: 0}}},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Text: "Save source", OnClicked: p.saveSource}, PushButton{Text: "Open source", OnClicked: p.openSource}, PushButton{Text: "Reload add-ons", OnClicked: p.loadSources}}},
		Label{Text: "Manual review notes (does not prove the installed version is current):"}, LineEdit{AssignTo: &p.notes},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Text: "Record manual check", OnClicked: p.manualChecked}, PushButton{Text: "Check selected source", OnClicked: p.checkAddon}, PushButton{Text: "Check all linked sources", OnClicked: p.checkAllAddons}}},
		ComboBox{AssignTo: &p.releases, Model: []string{"Check a CurseForge source to list releases"}, CurrentIndex: 0},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{Label{Text: "Stage for:"}, CheckBox{AssignTo: &p.creativeTarget, Text: "Creative"}, CheckBox{AssignTo: &p.survivalTarget, Text: "Survival"}, HSpacer{}}},
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{PushButton{Text: "Review local archive...", OnClicked: p.localAddon}, PushButton{Text: "Download selected release / review...", OnClicked: p.downloadAddon}}},
		TextEdit{AssignTo: &p.settingsLog, ReadOnly: true, MinSize: Size{Height: 120}, Text: "No update has been staged. API access can be added later; local archive review works without a key."}, VSpacer{},
	}}
}
func (p *updatesPage) selectedAddon() string {
	if p.pack == nil {
		if p.addonTable != nil {
			i := p.addonTable.CurrentIndex()
			if i >= 0 && i < len(p.addonModel.rows) {
				return p.addonModel.rows[i].UUID
			}
		}
		return ""
	}
	i := p.pack.CurrentIndex()
	if i < 0 || i >= len(p.packs) {
		return ""
	}
	return p.packs[i].UUID
}
func (p *updatesPage) addonCall(message, method string, params, out any, done func()) {
	if p.addonBusy {
		return
	}
	p.addonBusy = true
	p.setAddonMessage(message)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		e := p.a.client.Call(ctx, method, params, out)
		p.a.sync(func() {
			p.addonBusy = false
			if e != nil {
				p.setAddonMessage(e.Error())
				p.refreshAddonOverview()
				return
			}
			p.setAddonMessage("Completed. " + message)
			if done != nil {
				done()
			}
		})
	}()
}
func (p *updatesPage) saveKey() {
	key := p.apiKey.Text()
	p.apiKey.SetText("")
	p.addonCall("Saving encrypted API key...", "addons.credentials.save", map[string]string{"Key": key}, nil, func() { p.loadSources() })
}
func (p *updatesPage) removeKey() {
	if walk.MsgBox(p.addonOwner(), "Remove saved API key", "Remove this Windows user's saved CurseForge key? Source links and add-ons are retained.", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	p.addonCall("Removing saved key...", "addons.credentials.remove", struct{}{}, nil, func() { p.loadSources() })
}
func (p *updatesPage) loadSources() {
	p.addonLoadGeneration++
	generation := p.addonLoadGeneration
	selected := p.selectedAddon()
	owner := p.settingsDialog
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var packs []contentpacks.InventoryItem
		var records map[string]addonupdates.Source
		var status addonupdates.KeyStatus
		e := p.a.client.Call(ctx, "packs.list", struct{}{}, &packs)
		if e == nil {
			e = p.a.client.Call(ctx, "addons.sources.records", struct{}{}, &records)
		}
		if e == nil {
			e = p.a.client.Call(ctx, "addons.credentials.status", struct{}{}, &status)
		}
		p.a.sync(func() {
			if generation != p.addonLoadGeneration {
				return
			}
			if current := p.selectedAddon(); current != "" {
				selected = current
			}
			if e != nil {
				p.setAddonMessage(e.Error())
				p.packs = packs
				p.records = nil
				p.keyConfigured = status.Configured
				p.renderAddonOverview(selected)

				p.addonModel.PublishRowsReset()
				return
			}
			p.packs = packs
			p.records = records
			p.sources = map[string]string{}
			for id, r := range records {
				p.sources[id] = r.URL
			}
			p.keyConfigured = status.Configured
			p.renderAddonOverview(selected)
			if p.pack == nil || p.settingsDialog != owner {
				return
			}
			p.populateAddonSettings(selected)
			text := "No API key configured. Manual archive review is available."
			if status.Configured {
				text = "API key saved for this Windows user; not tested."
				if !status.LastTested.IsZero() {
					text = fmt.Sprintf("API key saved. Last test: %s. Successful: %t.", status.LastTested.Local().Format("02 Jan 15:04"), status.Valid)
				}
			}
			p.keyInfo.SetText(text)
		})
	}()
}
func (p *updatesPage) populateAddonSettings(selected string) {
	if p.pack == nil {
		return
	}
	names := []string{}
	index := 0
	for i, v := range p.packs {
		names = append(names, fmt.Sprintf("%s — library %s", v.Name, versionText(v.Version)))
		if v.UUID == selected {
			index = i
		}
	}
	p.pack.SetModel(names)
	if len(names) > 0 {
		p.pack.SetCurrentIndex(index)
		p.selectPack()
	}
}
func versionText(v [3]int) string { return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2]) }
func (p *updatesPage) selectPack() {
	id := p.selectedAddon()
	if id == "" || p.source == nil {
		return
	}
	r := p.records[id]
	p.source.SetText(r.URL)
	p.notes.SetText(r.Notes)
	pid := ""
	if r.ProjectID > 0 {
		pid = strconv.FormatInt(r.ProjectID, 10)
	}
	p.projectID.SetText(pid)
	idx := 0
	if r.Channel == "beta" {
		idx = 1
	} else if r.Channel == "alpha" {
		idx = 2
	}
	p.channel.SetCurrentIndex(idx)
	p.addonFiles = nil
	p.releases.SetModel([]string{"Check source to list releases"})
	p.releases.SetCurrentIndex(0)
	item := p.packs[p.pack.CurrentIndex()]
	var b strings.Builder
	for _, server := range []string{"creative", "survival"} {
		s := item.Servers[server]
		state := "disabled"
		if s.Enabled {
			state = versionText(s.Version)
			if !s.VersionKnown {
				state = "unknown (explicit version selection required)"
			}
		}
		fmt.Fprintf(&b, "%s: %s  ", server, state)
		if s.Pending != nil {
			fmt.Fprintf(&b, "(pending %s) ", versionText(s.PendingVersion))
		}
	}
	if r.CheckState != "" {
		fmt.Fprintf(&b, "\nLast check: %s — %s", r.LastChecked.Local().Format("02 Jan 15:04"), r.CheckState)
	} else {
		b.WriteString("\nUpdate status unknown. Save a source or review a downloaded archive.")
	}
	p.sourceInfo.SetText(b.String())
}
func (p *updatesPage) saveSource() {
	id := p.selectedAddon()
	if id == "" {
		return
	}
	r := p.records[id]
	r.URL = strings.TrimSpace(p.source.Text())
	r.Notes = p.notes.Text()
	r.Channel = []string{"stable", "beta", "alpha"}[p.channel.CurrentIndex()]
	r.Provider = "manual"
	r.ProjectID = 0
	if raw := strings.TrimSpace(p.projectID.Text()); raw != "" {
		n, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || n <= 0 {
			p.setAddonMessage("Enter a numeric positive project ID.")
			return
		}
		r.ProjectID = n
		r.Provider = "curseforge"
	} else if strings.Contains(r.URL, "curseforge.com/") {
		r.Provider = "curseforge"
	}
	r.CheckState = ""
	r.LastChecked = time.Time{}
	p.addonCall("Saving source settings...", "addons.source.save", map[string]any{"UUID": id, "Source": r}, nil, func() { p.loadSources() })
}
func (p *updatesPage) manualChecked() {
	p.addonCall("Recording manual review...", "addons.manual.checked", map[string]string{"UUID": p.selectedAddon(), "Notes": p.notes.Text()}, nil, func() { p.loadSources() })
}

type addonResult struct {
	AcknowledgedFileID int64
	Project            addonupdates.Project
	Files              []addonupdates.ReleaseFile
}

func (p *updatesPage) checkAddon() {
	var result addonResult
	p.addonCall("Checking CurseForge releases...", "addons.check", map[string]string{"UUID": p.selectedAddon()}, &result, func() {
		p.addonFiles = result.Files
		names := []string{}
		for _, f := range result.Files {
			label := fmt.Sprintf("%s — %s — file %d", f.DisplayName, f.FileDate.Local().Format("02 Jan 2006"), f.ID)
			if f.ID == result.AcknowledgedFileID {
				label += " (previously staged)"
			}
			names = append(names, label)
		}
		if len(names) == 0 {
			names = []string{"No available releases for this channel"}
		}
		if p.releases != nil {
			p.releases.SetModel(names)
			p.releases.SetCurrentIndex(0)
		}
		p.refreshAddonOverview()
		p.setAddonMessage("Release list fetched. A listed release is a candidate; archive review verifies UUIDs, versions, dependencies and engine requirements.")
	})
}
func (p *updatesPage) checkAllAddons() {
	if p.addonBusy {
		return
	}
	ids := []string{}
	for _, pack := range p.packs {
		if p.records[pack.UUID].Provider == "curseforge" {
			ids = append(ids, pack.UUID)
		}
	}
	if len(ids) == 0 {
		p.setAddonMessage("No CurseForge sources linked.")
		return
	}
	p.addonBusy = true
	p.setAddonMessage("Checking linked sources...")
	go func() {
		var b strings.Builder
		for _, id := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			var out addonResult
			e := p.a.client.Call(ctx, "addons.check", map[string]string{"UUID": id}, &out)
			cancel()
			if e != nil {
				fmt.Fprintf(&b, "%s: %s\r\n", id, e)
			} else {
				fmt.Fprintf(&b, "%s: %d candidate releases; select the pack and check to review.\r\n", out.Project.Name, len(out.Files))
			}
			text := b.String()
			p.a.sync(func() { p.setAddonMessage(text) })
		}
		p.a.sync(func() { p.addonBusy = false; p.loadSources() })
	}()
}
func (p *updatesPage) addonTargets() []string {
	var ids []string
	if p.creativeTarget.Checked() {
		ids = append(ids, "creative")
	}
	if p.survivalTarget.Checked() {
		ids = append(ids, "survival")
	}
	return ids
}
func (p *updatesPage) localAddon() {
	if p.addonBusy {
		return
	}
	dlg := &walk.FileDialog{Title: "Review a Bedrock add-on update", Filter: "Bedrock archives (*.mcaddon;*.mcpack;*.zip)|*.mcaddon;*.mcpack;*.zip"}
	ok, e := dlg.ShowOpen(p.addonOwner())
	if e != nil || !ok {
		return
	}
	p.previewAddon("addons.preview", map[string]any{"UUID": p.selectedAddon(), "Path": dlg.FilePath, "Servers": p.addonTargets()})
}
func (p *updatesPage) downloadAddon() {
	i := p.releases.CurrentIndex()
	if i < 0 || i >= len(p.addonFiles) {
		p.setAddonMessage("Check a source and select a release first.")
		return
	}
	p.previewAddon("addons.download", map[string]any{"UUID": p.selectedAddon(), "FileID": p.addonFiles[i].ID, "Servers": p.addonTargets()})
}
func (p *updatesPage) previewAddon(method string, params any) {
	var preview contentpacks.UpdatePreview
	p.addonCall("Preparing archive review; no deployed files are changed...", method, params, &preview, func() {
		var b strings.Builder
		fmt.Fprintf(&b, "Stage these exact versions for %s?\r\n\r\n", strings.Join(preview.Servers, ", "))
		for _, change := range preview.Changes {
			fmt.Fprintf(&b, "%s (%s) %s\r\n%s\r\n", change.Name, change.Type, versionText(change.Version), change.UUID)
			for _, server := range preview.Servers {
				fmt.Fprintf(&b, "  %s: %s\r\n", server, change.Actions[server])
			}
		}
		if preview.FileID > 0 {
			fmt.Fprintf(&b, "\r\nCurseForge project %d, file %d\r\n", preview.ProjectID, preview.FileID)
		}
		fmt.Fprintf(&b, "\r\nArchive SHA-256: %s\r\n\r\nRunning server folders are preserved. Apply later from Content Packs while stopped. Add-on compatibility beyond manifest requirements is not guaranteed.", preview.SHA256)
		if walk.MsgBox(p.addonOwner(), "Review add-on update", b.String(), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			p.setAddonMessage("Review cancelled. No update staged.")
			return
		}
		var result struct {
			Staged  bool
			Warning string
		}
		p.addonCall("Staging reviewed versions...", "addons.stage", map[string]string{"PreviewID": preview.ID}, &result, func() {
			p.setAddonMessage("Update staged. Use Content Packs to apply when stopped; the other server retains its deployed versions.")
			if result.Warning != "" {
				p.setAddonMessage(result.Warning)
			}
			p.loadSources()
		})
	})
}
