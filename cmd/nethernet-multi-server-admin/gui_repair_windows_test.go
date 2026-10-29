package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"nethernet-multi-server/internal/serverupdate"
	"runtime"
	"testing"
	"time"
)

func TestGroupCaptionBoundsIncludeScaledFont(t *testing.T) {
	for _, dpi := range []int{96, 144, 192, 288} {
		text := walk.Size{Width: 140 * dpi / 96, Height: 18 * dpi / 96}
		r := minecraftGroupCaptionRect(text, 800*dpi/96, dpi)
		if int(r.Bottom-r.Top) < text.Height || r.Right > int32(800*dpi/96) {
			t.Fatalf("caption clipped at %d DPI: %+v for %+v", dpi, r, text)
		}
	}
}
func TestUpdateProgressStages(t *testing.T) {
	for phase, index := range map[string]int{"downloading": 0, "stopping": 1, "backup": 2, "installing": 3, "starting": 4} {
		steps := updateProgressSteps(phase, false, false)
		if len(steps) != 5 || steps[index].State != 1 {
			t.Fatalf("%s: %+v", phase, steps)
		}
	}
	for _, s := range updateProgressSteps("complete", true, false) {
		if s.State != 2 {
			t.Fatal("complete stage remains active")
		}
	}
}
func TestUpdateSubtabsAndWrappedDescriptions(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	if a.updates.tabs.Pages().Len() != 2 || a.updates.tabs.Pages().At(0).Title() != "Servers" || a.updates.tabs.Pages().At(1).Title() != "Add-ons" {
		t.Fatal("update subtabs missing")
	}
	for i := 0; i < 2; i++ {
		if _, ok := a.updates.tabs.Pages().At(i).Children().At(0).(*walk.ScrollView); !ok {
			t.Fatal("subtab cannot scroll")
		}
	}
}
func TestUpdateProgressFinishesWithoutStartingServerWork(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, e := walk.NewMainWindow()
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Dispose()
	owner.SetLayout(walk.NewVBoxLayout())
	view, e := newUpdateProgressDialog(owner)
	if e != nil {
		t.Fatal(e)
	}
	defer view.dlg.Dispose()
	if view.closeButton.Enabled() {
		t.Fatal("close enabled before save completes")
	}
	view.renderUpdate(serverupdate.Status{Phase: "backup", Message: "Verifying world backup"}, time.Now(), "download verified")
	if view.finished {
		t.Fatal("backup marked complete")
	}
	view.renderUpdate(serverupdate.Status{Phase: "complete", Done: true}, time.Now(), "done")
	if !view.finished || !view.closeButton.Enabled() {
		t.Fatal("completion not shown")
	}
}
func TestUpdateProgressKeepsCloseVisibleOnShortWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, e := walk.NewMainWindow()
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Dispose()
	owner.SetLayout(walk.NewVBoxLayout())
	view, e := newUpdateProgressDialog(owner)
	if e != nil {
		t.Fatal(e)
	}
	defer view.dlg.Dispose()
	view.dlg.SetBounds(walk.Rectangle{Width: 500, Height: 360})
	time.AfterFunc(300*time.Millisecond, func() {
		view.dlg.Synchronize(func() {
			assertActionFitsAncestorClients(t, view.closeButton, view.dlg)
			if _, ok := view.dlg.Children().At(0).(*walk.ScrollView); !ok {
				t.Error("progress details have no scrolling fallback")
			}
			view.finished = true
			view.dlg.Accept()
		})
	})
	view.dlg.Run()
}
func TestMaximizedAccountActionsStayNextToTheirList(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	a.accountModel.set(make([]AccountRow, 4))
	a.navigation.SetCurrentIndex(2)
	a.mw.Starting().Attach(func() { win.ShowWindow(a.mw.Handle(), win.SW_MAXIMIZE) })
	time.AfterFunc(350*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			var action, table win.RECT
			// The section header now sits between the action bar and table.
			// Keep its table adjacent when the panel is maximized.
			win.GetWindowRect(a.helperToggle.Handle(), &action)
			win.GetWindowRect(a.accounts.Handle(), &table)
			if table.Top-action.Bottom > int32(a.mw.IntFrom96DPI(24)) {
				t.Errorf("account list is separated from actions by %d pixels", table.Top-action.Bottom)
			}
			a.mw.Close()
		})
	})
	a.mw.Run()
}
func TestMaximizedActivityFitsClientArea(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	a.mw.Starting().Attach(func() { win.ShowWindow(a.mw.Handle(), win.SW_MAXIMIZE) })
	time.AfterFunc(400*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			var item, client win.RECT
			var origin win.POINT
			win.GetWindowRect(a.events.Handle(), &item)
			win.GetClientRect(a.mw.Handle(), &client)
			win.ClientToScreen(a.mw.Handle(), &origin)
			if item.Bottom > origin.Y+client.Bottom {
				t.Errorf("Activity clipped: bottom %d > client %d, DPI %d", item.Bottom, origin.Y+client.Bottom, a.mw.DPI())
			}
			a.mw.Close()
		})
	})
	a.mw.Run()
}
