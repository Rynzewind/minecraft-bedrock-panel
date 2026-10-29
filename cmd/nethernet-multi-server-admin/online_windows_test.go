package main

import (
	"context"
	"fmt"
	"runtime"
	"testing"
)

func TestOnlineIsSeparateAndDefaultsDisabledWithoutRoutingConsent(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	if a.navigation.Pages().Len() != 7 || a.navigation.Pages().At(6).Title() != "Online" {
		t.Fatal("Online tab missing")
	}
	if a.online == nil || a.online.tabs.Pages().Len() != 4 {
		t.Fatal("Online subtabs missing")
	}
	if a.online.allowUnknownFriendsRoute == nil || a.online.allowUnknownFriendsRoute.Checked() {
		t.Fatal("explicit routing consent control missing or preselected")
	}
	for _, c := range a.online.cards {
		if c.enabled.Checked() {
			t.Fatal("Online enabled by default")
		}
	}
	if minecraftTabIcon("Online") != "online" {
		t.Fatal("Online icon missing")
	}
}

func TestBroadcasterEnrollmentRequiresRoleCapableHelper(t *testing.T) {
	if e := requireBroadcasterRoleCapability(func(context.Context, string, any, any) error { return fmt.Errorf("unknown method") }); e == nil {
		t.Fatal("older helper accepted")
	}
	if e := requireBroadcasterRoleCapability(func(context.Context, string, any, any) error { return nil }); e == nil {
		t.Fatal("missing capability accepted")
	}
	if e := requireBroadcasterRoleCapability(func(_ context.Context, _ string, _ any, out any) error {
		out.(*accountRoleCapabilities).BroadcasterRoles = true
		return nil
	}); e != nil {
		t.Fatal(e)
	}
}
