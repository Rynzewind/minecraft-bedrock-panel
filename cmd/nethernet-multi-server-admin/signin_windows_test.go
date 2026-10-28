package main

import (
	"context"
	"errors"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"runtime"
	"testing"
	"time"
)

func TestCancelledSignInDoesNotCountAsAccepted(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, err := walk.NewMainWindow()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Dispose()
	owner.SetLayout(walk.NewVBoxLayout())
	// The dialog runs its own message loop. Queue cancellation onto that UI thread.
	time.AfterFunc(250*time.Millisecond, func() {
		owner.Synchronize(func() {
			dlg := win.GetWindow(owner.Handle(), win.GW_ENABLEDPOPUP)
			if dlg == 0 || dlg == owner.Handle() {
				t.Error("sign-in dialog not found")
				return
			}
			win.SendMessage(dlg, win.WM_CLOSE, 0, 0)
		})
	})
	err = showSignInDialog(owner, enrollmentView{VerificationURI: "https://example.test", UserCode: "TEST", Expiry: time.Now().Add(time.Minute)}, "ExpectedAccount")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled sign-in returned %v; must not start completion", err)
	}
}
