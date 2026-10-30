package main

import (
	"nethernet-multi-server/internal/contentpacks"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAddonGroupControlsCountAndClearHiddenSelection(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	p := a.updates
	p.packs = []contentpacks.InventoryItem{{StoredPack: contentpacks.StoredPack{Pack: contentpacks.Pack{UUID: "one", Name: "Airplanes"}}}}
	p.renderAddonOverview("one")
	if !strings.Contains(p.addonGroupButtons[1].Text(), "Unknown (1)") {
		t.Fatal("wrong group count")
	}
	if len(p.addonModel.rows) != 1 {
		t.Fatal("groups should default expanded")
	}
	a.mw.Starting().Attach(func() {
		time.AfterFunc(100*time.Millisecond, func() {
			a.mw.Synchronize(func() {
				p.toggleAddonGroup(1)
				if len(p.addonModel.rows) != 0 || p.addonTable.CurrentIndex() != -1 {
					t.Error("hidden row remained selectable")
				}
				if !strings.HasPrefix(p.addonGroupButtons[1].Text(), "Show") {
					t.Error("collapsed group not labelled")
				}
				p.renderAddonOverview("")
				if len(p.addonModel.rows) != 0 {
					t.Error("refresh expanded collapsed group")
				}
				p.toggleAddonGroup(1)
				if len(p.addonModel.rows) != 1 {
					t.Error("group did not expand")
				}
				a.mw.Close()
			})
		})
	})
	a.mw.Run()
}
