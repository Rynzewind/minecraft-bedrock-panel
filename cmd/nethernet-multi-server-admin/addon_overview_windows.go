package main

import (
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
)

type addonOverviewRow struct{ UUID, Name, Type, Library, Creative, Survival, Status, Checked string }
type addonOverviewModel struct {
	walk.TableModelBase
	rows []addonOverviewRow
}

func (m *addonOverviewModel) RowCount() int { return len(m.rows) }
func (m *addonOverviewModel) Value(r, c int) any {
	if r < 0 || r >= len(m.rows) {
		return ""
	}
	v := m.rows[r]
	switch c {
	case 0:
		return v.Name
	case 1:
		return v.Type
	case 2:
		return v.Library
	case 3:
		return v.Creative
	case 4:
		return v.Survival
	case 5:
		return v.Status
	case 6:
		return v.Checked
	}
	return ""
}
func addonStatusColor(status string) walk.Color {
	switch status {
	case "Up to date":
		return mc.green
	case "Outdated":
		return mc.red
	default:
		return mc.amber
	}
}
func (p *updatesPage) addonsTab() TabPage {
	return TabPage{Title: "Add-ons", Layout: VBox{}, Children: []Widget{
		Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{TextLabel{Text: "Installed add-ons", StretchFactor: 1}, PushButton{Text: "Check selected", OnClicked: p.checkAddon}, PushButton{Text: "Check all linked", OnClicked: p.checkAllAddons}, PushButton{Text: "Refresh", OnClicked: p.loadSources}, PushButton{AssignTo: &p.settingsCog, Text: "⚙ Settings", ToolTipText: "API key, source links, manual reviews and staging", OnClicked: p.openAddonSettings}}},
		TextLabel{Text: "Green: up to date • Red: outdated • Amber: unknown. Checks verify source manifests without installing anything. Last-check evidence expires after 24 hours."},
		TableView{AssignTo: &p.addonTable, Model: &p.addonModel, StretchFactor: 1, MinSize: Size{Height: 160}, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Add-on", Width: 290}, {Title: "Type", Width: 80}, {Title: "Library", Width: 90}, {Title: "Creative", Width: 140}, {Title: "Survival", Width: 140}, {Title: "Update status", Width: 120}, {Title: "Last checked", Width: 150}}},
		TextEdit{AssignTo: &p.addonLog, ReadOnly: true, MinSize: Size{Height: 55}, MaxSize: Size{Height: 85}, Text: "Select a pack to check its source. Configure sources and manual reviews using the cogwheel."},
	}}
}
func (p *updatesPage) setAddonMessage(text string) {
	if p.addonLog != nil {
		p.addonLog.SetText(text)
	}
	if p.settingsLog != nil {
		p.settingsLog.SetText(text)
	}
}
func (p *updatesPage) renderAddonOverview(selected string) {
	rows := []addonOverviewRow{}
	for _, pack := range p.packs {
		source := p.records[pack.UUID]
		if !p.keyConfigured {
			source.LatestCheckOK = false
		}
		states := []addonupdates.VersionState{}
		values := map[string]string{}
		for _, id := range []string{"creative", "survival"} {
			s := pack.Servers[id]
			states = append(states, addonupdates.VersionState{Enabled: s.Enabled, Known: s.VersionKnown, Version: s.Version})
			text := "Disabled"
			if s.Enabled {
				text = "Unknown"
				if s.VersionKnown {
					text = versionText(s.Version)
				}
			}
			if s.Pending != nil {
				text += " (pending)"
			}
			values[id] = text
		}
		checked := "Never"
		if !source.LastChecked.IsZero() {
			checked = source.LastChecked.Local().Format("02 Jan 2006 15:04")
		}
		rows = append(rows, addonOverviewRow{pack.UUID, pack.Name, string(pack.Type), versionText(pack.Version), values["creative"], values["survival"], addonupdates.OverviewStatus(pack.Version, states, source), checked})
	}
	p.addonModel.set(rows)
	if p.addonTable != nil {
		for i, r := range rows {
			if r.UUID == selected {
				p.addonTable.SetCurrentIndex(i)
				break
			}
		}
	}
}
func (p *updatesPage) refreshAddonOverview() {
	p.addonLoadGeneration++
	generation := p.addonLoadGeneration
	selected := p.selectedAddon()
	go func() {
		ctx, cancel := timeoutContext()
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
			p.keyConfigured = status.Configured
			if e != nil {
				p.setAddonMessage(e.Error())
				p.records = nil
				p.renderAddonOverview(selected)
				return
			}
			p.packs = packs
			p.records = records
			p.renderAddonOverview(selected)
		})
	}()
}
func (p *updatesPage) createAddonSettings() (*walk.Dialog, error) {
	if p.settingsDialog != nil {
		return p.settingsDialog, nil
	}
	selected := p.selectedAddon()
	page := p.addonSettingsTab()
	spec := themedDialog{AssignTo: &p.settingsDialog, Title: "Add-on update settings", MinSize: Size{Width: 600, Height: 380}, Size: Size{Width: 850, Height: 740}, Layout: VBox{}, Children: []Widget{ScrollView{Layout: VBox{}, StretchFactor: 1, Children: page.Children}, Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{Text: "Close", OnClicked: func() {
		if !p.addonBusy {
			p.settingsDialog.Accept()
		}
	}}}}}}
	if e := spec.Create(p.a.mw); e != nil {
		return nil, e
	}
	dlg := p.settingsDialog
	dlg.Starting().Attach(func() { fitProgressDialogToWorkArea(dlg) })
	dlg.Closing().Attach(func(cancel *bool, _ walk.CloseReason) { *cancel = p.addonBusy })
	p.populateAddonSettings(selected)
	return dlg, nil
}
func (p *updatesPage) openAddonSettings() {
	if p.addonBusy {
		p.setAddonMessage("Wait for the current check to finish before opening settings.")
		return
	}
	dlg, e := p.createAddonSettings()
	if e != nil {
		p.setAddonMessage(fmt.Sprint(e))
		return
	}
	p.loadSources()
	dlg.Run()
	dlg.Dispose()
	p.settingsDialog = nil
	p.apiKey = nil
	p.projectID = nil
	p.notes = nil
	p.keyInfo = nil
	p.channel = nil
	p.releases = nil
	p.creativeTarget = nil
	p.survivalTarget = nil
	p.pack = nil
	p.source = nil
	p.sourceInfo = nil
	p.settingsLog = nil
	p.refreshAddonOverview()
}

func (p *updatesPage) addonOwner() walk.Form {
	if p.settingsDialog != nil {
		return p.settingsDialog
	}
	return p.a.mw
}

func (m *addonOverviewModel) set(rows []addonOverviewRow) {
	same := len(rows) == len(m.rows)
	if same {
		for i, r := range rows {
			if r.UUID != m.rows[i].UUID {
				same = false
				break
			}
		}
	}
	if !same {
		m.rows = rows
		m.PublishRowsReset()
		return
	}
	for i, r := range rows {
		if r != m.rows[i] {
			m.rows[i] = r
			m.PublishRowChanged(i)
		}
	}
}
