package main

import (
	"nethernet-multi-server/internal/addonupdates"
	"nethernet-multi-server/internal/contentpacks"
	"runtime"
	"testing"
	"time"
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
	dlg, e := p.createAddonSettings()
	if e != nil {
		t.Fatal(e)
	}
	defer dlg.Dispose()
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

func TestAddonOverviewUsesRestartAmber(t *testing.T) {
	if addonStatusColor("Unknown") != mc.amber || addonStatusColor("Outdated") != mc.red || addonStatusColor("Up to date") != mc.green {
		t.Fatal("incorrect overview status colours")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	if a.updates.addonTable == nil || a.updates.settingsCog == nil {
		t.Fatal("overview or cog missing")
	}
	if a.updates.apiKey != nil {
		t.Fatal("settings remain on main page")
	}
	dlg, e := a.updates.createAddonSettings()
	if e != nil {
		t.Fatal(e)
	}
	defer dlg.Dispose()
	if a.updates.apiKey == nil || !a.updates.apiKey.PasswordMode() {
		t.Fatal("settings lost key masking")
	}
}

func TestAddonOverviewSelectionSurvivesSettings(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	p := a.updates
	p.packs = []contentpacks.InventoryItem{{StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{UUID: "first", Name: "First"}}}, {StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{UUID: "second", Name: "Second"}}}}
	p.renderAddonOverview("second")
	if got := p.selectedAddon(); got != "second" {
		t.Fatalf("selected %q", got)
	}
	dlg, e := p.createAddonSettings()
	if e != nil {
		t.Fatal(e)
	}
	defer dlg.Dispose()
	if got := p.selectedAddon(); got != "second" {
		t.Fatalf("settings selected %q", got)
	}
}
func TestAddonOverviewWithoutKeyRemainsUnknown(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	p := a.updates
	v := [3]int{2, 0, 0}
	p.packs = []contentpacks.InventoryItem{{StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{UUID: "pack", Version: v}}}}
	p.records = map[string]addonupdates.Source{"pack": {LatestCheckOK: true, LatestFileID: 1, VerifiedFileID: 1, VerifiedVersion: &v, LastChecked: time.Now()}}
	p.renderAddonOverview("pack")
	if got := p.addonModel.rows[0].Status; got != "Unknown" {
		t.Fatalf("without API key: %s", got)
	}
}
