package main

import (
	"runtime"
	"testing"
	"time"
)

func TestSetupWindowCreatesRestoreAndActivationControls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	w := &setupWindow{}
	if e := w.create(); e != nil {
		t.Fatal(e)
	}
	defer w.mw.Dispose()
	if w.archive == nil || w.destination == nil || w.restore == nil || w.activate == nil || w.stopped == nil || w.creativeIP == nil || w.survivalIP == nil || w.adopt == nil || w.reserved == nil {
		t.Fatal("missing setup controls")
	}
	if w.stopped.Checked() {
		t.Fatal("old deployment confirmation must be explicit")
	}
	if w.startup.CurrentIndex() != 1 {
		t.Fatal("unexpected startup selection")
	}
	time.AfterFunc(300*time.Millisecond, func() { w.mw.Synchronize(func() { w.mw.Close() }) })
	w.mw.Run()
}
