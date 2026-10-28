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
