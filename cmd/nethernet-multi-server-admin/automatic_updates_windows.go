package main

import (
	"nethernet-multi-server/internal/autoupdate"
)

func (p *updatesPage) applyAutomatic(s autoupdate.Settings) {
	if p.autoServers == nil || p.autoSaving {
		return
	}
	p.autoApplying = true
	defer func() { p.autoApplying = false }()
	p.autoServers.SetChecked(s.Servers)
	p.autoAddons.SetChecked(s.Addons)
	p.autoServers.SetEnabled(true)
	p.autoAddons.SetEnabled(true)
	message := s.Message
	if !s.LastCheck.IsZero() {
		message += " Last check: " + s.LastCheck.Local().Format("02 Jan 15:04")
	}
	p.autoMessage.SetText(message)
}
func (p *updatesPage) saveAutomatic(server bool) {
	if p.autoApplying || p.autoSaving {
		return
	}
	p.autoSaving = true
	params := map[string]bool{}
	if server {
		params["Servers"] = p.autoServers.Checked()
	} else {
		params["Addons"] = p.autoAddons.Checked()
	}
	p.autoServers.SetEnabled(false)
	p.autoAddons.SetEnabled(false)
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		var s autoupdate.Settings
		err := p.a.client.Call(ctx, "updates.auto.save", params, &s)
		p.a.sync(func() {
			p.autoSaving = false
			if err != nil {
				p.autoMessage.SetText("Could not save automatic update setting: " + err.Error())
				p.autoServers.SetEnabled(true)
				p.autoAddons.SetEnabled(true)
				return
			}
			p.applyAutomatic(s)
		})
	}()
}
