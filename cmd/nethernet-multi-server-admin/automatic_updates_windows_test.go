package main

import (
	"nethernet-multi-server/internal/autoupdate"
	"runtime"
	"testing"
)

func TestAutomaticUpdateSwitchesReflectIndependentSavedSettings(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	p := a.updates
	p.applyAutomatic(autoupdate.Settings{Servers: true, Addons: false, Message: "fixture"})
	if !p.autoServers.Checked() || p.autoAddons.Checked() || !p.autoServers.Enabled() || !p.autoAddons.Enabled() {
		t.Fatal("saved settings not reflected")
	}
	p.applyAutomatic(autoupdate.Settings{Servers: false, Addons: true})
	if p.autoServers.Checked() || !p.autoAddons.Checked() {
		t.Fatal("switches were bundled")
	}
}
