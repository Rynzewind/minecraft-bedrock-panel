package main

import (
	"errors"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"runtime"
	"testing"
)

func TestTrayFailureIsNonfatalAndRetriesWithoutLeakingControls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	defer a.disposeTray()
	attempts := 0
	a.trayCreate = func(f walk.Form) (*walk.NotifyIcon, error) {
		attempts++
		if attempts <= 2 {
			return nil, errors.New("Shell_NotifyIcon")
		}
		return walk.NewNotifyIcon(f)
	}
	a.startTrayRecovery()
	if a.mw.IsDisposed() || a.tray != nil || !a.startAll.Enabled() {
		t.Fatal("tray failure stopped panel/control startup")
	}
	if len(a.trayActions) != 0 {
		t.Fatal("failed attempt leaked controls")
	}
	win.SendMessage(a.mw.Handle(), win.WM_TIMER, trayRetryTimer, 0)
	win.SendMessage(a.mw.Handle(), win.WM_TIMER, trayRetryTimer, 0)
	if a.tray == nil || attempts != 3 || len(a.trayActions) != 4 {
		t.Fatalf("retry failed: attempts=%d actions=%d", attempts, len(a.trayActions))
	}
	win.SendMessage(a.mw.Handle(), win.WM_TIMER, trayRetryTimer, 0)
	if attempts != 3 {
		t.Fatal("successful tray kept retrying")
	}
}
func TestPersistentTrayFailureMakesHiddenPanelAccessible(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	defer a.disposeTray()
	a.trayCreate = func(walk.Form) (*walk.NotifyIcon, error) { return nil, errors.New("Shell_NotifyIcon") }
	a.startTrayRecovery()
	for i := 0; i < 15; i++ {
		win.SendMessage(a.mw.Handle(), win.WM_TIMER, trayRetryTimer, 0)
	}
	if !a.mw.Visible() || a.mw.IsDisposed() {
		t.Fatal("panel stranded hidden without tray")
	}
}
func TestTrayFailureAfterConstructionDisposesMenu(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	var menu *walk.Menu
	a.trayCreate = func(f walk.Form) (*walk.NotifyIcon, error) {
		tray, err := walk.NewNotifyIcon(f)
		if err != nil {
			return nil, err
		}
		menu = tray.ContextMenu()
		tray.Dispose()
		return tray, nil
	}
	if err := a.setupTray(); err == nil {
		t.Fatal("registration modification unexpectedly succeeded")
	}
	if menu == nil || !menu.IsDisposed() {
		t.Fatal("failed tray attempt leaked its menu")
	}
	if a.tray != nil || len(a.trayActions) != 0 {
		t.Fatal("failed tray was published")
	}
}
