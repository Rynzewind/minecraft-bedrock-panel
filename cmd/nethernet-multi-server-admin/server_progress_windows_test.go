package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"runtime"
	"testing"
)

func TestServerProgressPopupBlocksMainWindowAndCannotCloseWhileBusy(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Dispose()
	if err := owner.SetLayout(walk.NewVBoxLayout()); err != nil {
		t.Fatal(err)
	}
	view, err := newServerProgressDialog(owner, "Creative")
	if err != nil {
		t.Fatal(err)
	}
	defer view.dlg.Dispose()
	view.dlg.Starting().Attach(func() {
		if win.IsWindowEnabled(owner.Handle()) {
			t.Error("main window remained enabled")
		}
		if view.closeButton.Enabled() || !view.bar.MarqueeMode() {
			t.Error("busy controls incorrect")
		}
		view.dlg.Cancel()
		if view.dlg.IsDisposed() {
			t.Error("busy popup closed")
			return
		}
		view.finished = true
		view.closeButton.SetEnabled(true)
		view.dlg.Accept()
	})
	view.dlg.Run()
	if !win.IsWindowEnabled(owner.Handle()) {
		t.Error("main window stayed disabled after popup closed")
	}
}

func TestProgressAutoClosesOnlyOnSuccess(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Dispose()
	owner.SetLayout(walk.NewVBoxLayout())
	view, err := newServerProgressDialog(owner, "Auto close test")
	if err != nil {
		t.Fatal(err)
	}
	defer view.dlg.Dispose()
	view.dlg.Starting().Attach(func() {
		view.closeOnSuccess()
		if win.IsWindowEnabled(owner.Handle()) {
			t.Error("busy progress closed")
		}
		view.finished = true
		view.failed = true
		view.closeOnSuccess()
		if win.IsWindowEnabled(owner.Handle()) {
			t.Error("failed progress closed")
		}
		view.failed = false
		view.closeOnSuccess()
	})
	view.dlg.Run()
	if !win.IsWindowEnabled(owner.Handle()) {
		t.Error("successful progress did not close and unlock owner")
	}
}
