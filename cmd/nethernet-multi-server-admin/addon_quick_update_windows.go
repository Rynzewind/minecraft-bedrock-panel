package main

import (
	"context"
	"fmt"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/supervisor"
	"sort"
	"strings"
	"time"

	"github.com/lxn/walk"
)

type quickAddonRequest struct {
	UUID, Name string
	Targets    []string
}

type quickAddonPreview struct {
	quickAddonRequest
	Preview contentpacks.UpdatePreview
}

// quickUpdateIDs returns only archive-verified, outdated entries. Entries from
// the same project are represented once because a Bedrock .mcaddon commonly
// carries its matching resource and behaviour packs together.
func (p *updatesPage) quickUpdateIDs(selectedOnly bool) []quickAddonRequest {
	selected := p.selectedAddon()
	seenProjects := map[string]bool{}
	var out []quickAddonRequest
	rows := p.addonModel.rows
	if !selectedOnly && p.addonAllRows != nil {
		rows = p.addonAllRows
	}
	for _, row := range rows {
		if selectedOnly && row.UUID != selected {
			continue
		}
		if row.Status != "Outdated" {
			continue
		}
		source := p.records[row.UUID]
		if !source.LatestCheckOK || source.LatestFileID <= 0 || source.VerifiedFileID != source.LatestFileID {
			continue
		}
		projectKey := source.Provider + ":" + fmt.Sprint(source.ProjectID)
		if source.ProjectID == 0 {
			projectKey += ":" + row.UUID
		}
		if seenProjects[projectKey] {
			continue
		}
		var targets []string
		for _, pack := range p.packs {
			if pack.UUID != row.UUID {
				continue
			}
			for _, id := range []string{"creative", "survival"} {
				if pack.Servers[id].Enabled {
					targets = append(targets, id)
				}
			}
			break
		}
		if len(targets) == 0 {
			continue
		}
		seenProjects[projectKey] = true
		out = append(out, quickAddonRequest{UUID: row.UUID, Name: row.Name, Targets: targets})
	}
	return out
}

func (p *updatesPage) quickUpdateSelected()    { p.runQuickUpdate(p.quickUpdateIDs(true)) }
func (p *updatesPage) quickUpdateAllVerified() { p.runQuickUpdate(p.quickUpdateIDs(false)) }

func (p *updatesPage) runQuickUpdate(requests []quickAddonRequest) {
	if p.addonBusy {
		return
	}
	if len(requests) == 0 {
		p.setAddonMessage("Select a red, verified add-on, or use Update all when verified updates are available.")
		return
	}
	var names []string
	for _, request := range requests {
		names = append(names, request.Name+" ("+strings.Join(request.Targets, ", ")+")")
	}
	intro := "NetherNet will download the newest official archives, validate their checksums and manifests, then stage them. No world or deployed pack is changed until you choose Restart and apply.\r\n\r\n" + strings.Join(names, "\r\n")
	if walk.MsgBox(p.addonOwner(), "Prepare add-on updates", intro, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	p.addonBusy = true
	p.setQuickButtons(false)
	p.setAddonMessage("Preparing verified add-on updates; no deployed files are changing...")
	go p.prepareQuickUpdates(requests)
}

func (p *updatesPage) prepareQuickUpdates(requests []quickAddonRequest) {
	previews := make([]quickAddonPreview, 0, len(requests))
	for _, request := range requests {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		var check addonResult
		err := p.a.client.Call(ctx, "addons.check", map[string]string{"UUID": request.UUID}, &check)
		cancel()
		if err != nil {
			p.finishQuickUpdate(nil, fmt.Errorf("%s: %w", request.Name, err))
			return
		}
		if len(check.Files) == 0 {
			p.finishQuickUpdate(nil, fmt.Errorf("%s no longer has a compatible official release", request.Name))
			return
		}
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Minute)
		var preview contentpacks.UpdatePreview
		err = p.a.client.Call(ctx, "addons.download", map[string]any{"UUID": request.UUID, "FileID": check.Files[0].ID, "Servers": request.Targets}, &preview)
		cancel()
		if err != nil {
			p.finishQuickUpdate(nil, fmt.Errorf("%s: %w", request.Name, err))
			return
		}
		previews = append(previews, quickAddonPreview{quickAddonRequest: request, Preview: preview})
	}
	p.finishQuickUpdate(previews, nil)
}

func (p *updatesPage) finishQuickUpdate(previews []quickAddonPreview, err error) {
	p.a.sync(func() {
		if err != nil {
			p.addonBusy = false
			p.setQuickButtons(true)
			p.setAddonMessage(err.Error())
			p.loadSources()
			return
		}
		var text strings.Builder
		text.WriteString("Stage these verified updates?\r\n\r\n")
		for _, item := range previews {
			fmt.Fprintf(&text, "%s for %s\r\n", item.Name, strings.Join(item.Targets, ", "))
			for _, change := range item.Preview.Changes {
				fmt.Fprintf(&text, "  %s %s\r\n", change.Name, versionText(change.Version))
			}
		}
		text.WriteString("\r\nWorlds and deployed packs remain unchanged until Restart and apply is confirmed.")
		if walk.MsgBox(p.addonOwner(), "Review staged add-on updates", text.String(), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			p.addonBusy = false
			p.setQuickButtons(true)
			p.setAddonMessage("Update review cancelled. Nothing was staged.")
			return
		}
		p.setAddonMessage("Staging verified add-on updates...")
		go p.stageQuickUpdates(previews)
	})
}

func (p *updatesPage) stageQuickUpdates(previews []quickAddonPreview) {
	targets := map[string]bool{}
	for _, item := range previews {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		// Staging another archive changes the pack-state fingerprint. Recreate this
		// already-approved preview immediately before staging so each operation is
		// still checked against the state it will actually modify.
		var fresh contentpacks.UpdatePreview
		err := p.a.client.Call(ctx, "addons.download", map[string]any{"UUID": item.UUID, "FileID": item.Preview.FileID, "Servers": item.Targets}, &fresh)
		cancel()
		if err != nil {
			p.finishStagedQuickUpdates(nil, fmt.Errorf("%s: %w", item.Name, err))
			return
		}
		if fresh.SHA256 != item.Preview.SHA256 {
			p.finishStagedQuickUpdates(nil, fmt.Errorf("%s changed after review; no further updates were staged", item.Name))
			return
		}
		ctx, cancel = context.WithTimeout(context.Background(), 8*time.Minute)
		var result struct{ Staged bool }
		err = p.a.client.Call(ctx, "addons.stage", map[string]string{"PreviewID": fresh.ID}, &result)
		cancel()
		if err != nil {
			p.finishStagedQuickUpdates(nil, fmt.Errorf("%s: %w", item.Name, err))
			return
		}
		for _, target := range item.Targets {
			targets[target] = true
		}
	}
	ids := make([]string, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	p.finishStagedQuickUpdates(ids, nil)
}

func (p *updatesPage) finishStagedQuickUpdates(targets []string, err error) {
	p.a.sync(func() {
		p.addonBusy = false
		p.setQuickButtons(true)
		p.loadSources()
		if err != nil {
			p.setAddonMessage("Some updates may be staged, but none were applied: " + err.Error())
			return
		}
		p.setAddonMessage("Updates staged. They are ready for the next restart.")
		question := "Restart and apply the staged updates to " + strings.Join(targets, " and ") + " now?\r\n\r\nEach affected world will stop cleanly, apply its backup-protected pack changes, then restart. This is available only when nobody is playing."
		if walk.MsgBox(p.addonOwner(), "Apply staged add-on updates", question, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			p.addonBusy = true
			p.setQuickButtons(false)
			p.setAddonMessage("Checking worlds before restart and apply...")
			go p.applyQuickUpdates(targets)
		}
	})
}

func (p *updatesPage) applyQuickUpdates(targets []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var statuses []supervisor.Status
	if err := p.a.client.Call(ctx, "servers.list", struct{}{}, &statuses); err != nil {
		p.completeQuickApply(err)
		return
	}
	byID := map[string]supervisor.Status{}
	for _, status := range statuses {
		byID[status.ID] = status
	}
	for _, id := range targets {
		status, ok := byID[id]
		if !ok {
			p.completeQuickApply(fmt.Errorf("%s server was not found", id))
			return
		}
		if status.Players > 0 {
			p.completeQuickApply(fmt.Errorf("%s still has %d player(s); updates remain safely staged", status.Name, status.Players))
			return
		}
		if status.State != supervisor.Running && status.State != supervisor.Stopped {
			p.completeQuickApply(fmt.Errorf("%s is %s; updates remain safely staged", status.Name, status.State))
			return
		}
	}
	for _, id := range targets {
		status := byID[id]
		method := "server.restart"
		if status.State == supervisor.Stopped {
			method = "server.start"
		}
		if err := p.a.client.Call(ctx, method, map[string]string{"server_id": id}, nil); err != nil {
			p.completeQuickApply(fmt.Errorf("%s: %w", status.Name, err))
			return
		}
	}
	p.completeQuickApply(nil)
}

func (p *updatesPage) completeQuickApply(err error) {
	p.a.sync(func() {
		p.addonBusy = false
		p.setQuickButtons(true)
		if err != nil {
			p.setAddonMessage(err.Error())
		} else {
			p.setAddonMessage("Staged add-on updates were applied and affected worlds restarted.")
		}
		p.a.refresh()
		p.loadSources()
	})
}

func (p *updatesPage) setQuickButtons(enabled bool) {
	if p.quickUpdate != nil {
		p.quickUpdate.SetEnabled(enabled)
	}
	if p.quickUpdateAll != nil {
		p.quickUpdateAll.SetEnabled(enabled)
	}
}
