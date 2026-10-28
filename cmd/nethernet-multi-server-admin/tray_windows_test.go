package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFleetControlsAvailableWhenDisconnected(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	a.updateManagementActions()
	if !a.startAll.Enabled() || !a.stopAll.Enabled() || !a.restartAll.Enabled() {
		t.Fatal("bulk controls disabled while helper is offline")
	}
	a.fleetBusy.Store(true)
	a.updateManagementActions()
	if a.startAll.Enabled() || a.stopAll.Enabled() || a.restartAll.Enabled() {
		t.Fatal("overlapping operation enabled")
	}
}
func TestTrayCloseHidesPanelWithoutStoppingIt(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	if err := a.setupTray(); err != nil {
		t.Fatal(err)
	}
	defer a.tray.Dispose()
	a.mw.Show()
	win.SendMessage(a.mw.Handle(), win.WM_SYSCOMMAND, win.SC_CLOSE, 0)
	if a.mw.IsDisposed() || a.mw.Visible() {
		t.Fatal("close did not minimize to tray")
	}
	if a.tray.ContextMenu().Actions().Len() < 6 {
		t.Fatal("tray controls missing")
	}
	a.quitting = true
	a.mw.Close()
	_ = walk.CloseReasonUser
}

func TestStartupPanelCanRunHidden(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	if a.mw.Visible() {
		t.Fatal("startup panel appeared before tray initialization")
	}
}
func TestAdminInstanceRejectsDuplicateLaunch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Admin.exe")
	first, ok, e := adminInstance(path, true)
	if e != nil || !ok {
		t.Fatal(e)
	}
	defer windows.CloseHandle(first)
	_, ok, e = adminInstance(path, true)
	if e != nil || ok {
		t.Fatalf("duplicate accepted: %v", e)
	}
}
