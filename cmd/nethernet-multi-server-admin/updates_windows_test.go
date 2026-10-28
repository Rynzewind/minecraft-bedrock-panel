package main

import (
	"runtime"
	"testing"
)

func TestUpdatesHasDedicatedTabAndCompactControls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	if a.navigation.Pages().Len() != 6 || a.navigation.Pages().At(5).Title() != "Updates" {
		t.Fatal("Updates tab missing")
	}
	if a.updates == nil || a.updates.install.Enabled() || a.updates.recover.Enabled() {
		t.Fatal("unsafe update actions enabled before check")
	}
	if minecraftIcon("update") == nil {
		t.Fatal("update icon missing")
	}
}

func TestAddonSettingsAreMaskedAndRequireExplicitTargets(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	p := a.updates
	if p.apiKey == nil || !p.apiKey.PasswordMode() {
		t.Fatal("key field is not masked")
	}
	if len(p.addonTargets()) != 0 {
		t.Fatal("server target preapproved")
	}
	if p.tabs.Pages().Len() != 2 || p.tabs.Pages().At(1).Title() != "Add-ons" {
		t.Fatal("Add-ons subtab missing")
	}
	if p.addonLog == nil || p.keyInfo == nil || p.projectID == nil || p.channel == nil {
		t.Fatal("update settings missing")
	}
}
