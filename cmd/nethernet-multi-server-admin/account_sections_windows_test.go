package main

import (
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/service"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestAccountCountsSeparateHelpers(t *testing.T) {
	state := BuildViewState(service.Status{Accounts: []service.PublicAccount{
		{ID: "h", Role: accountstore.RoleHelper}, {ID: "b", Role: accountstore.RoleBroadcaster}, {ID: "legacy"}, {ID: "u", Role: accountstore.RoleUnassigned},
	}})
	helpers, others := splitAccountRows(state.Accounts)
	if len(helpers) != 2 || len(others) != 2 || !strings.Contains(state.StatusText, "2 accounts - 2 helpers") {
		t.Fatalf("incorrect split: %+v", state)
	}
}

func TestAccountSectionsDefaultAndSelection(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	a.state = BuildViewState(service.Status{Accounts: []service.PublicAccount{{ID: "b", Role: accountstore.RoleBroadcaster, Order: 0}, {ID: "h", Role: accountstore.RoleHelper, Order: 1}}})
	helpers, others := splitAccountRows(a.state.Accounts)
	a.accountModel.set(helpers)
	a.broadcasterModel.set(others)
	a.navigation.SetCurrentIndex(2)
	a.mw.Starting().Attach(func() {
		time.AfterFunc(150*time.Millisecond, func() {
			a.mw.Synchronize(func() {
				if !a.accounts.Visible() || a.broadcasters.Visible() {
					t.Error("wrong default expansion")
				}
				a.accounts.SetCurrentIndex(0)
				if row, ok := a.accountRow(); !ok || row.ID != "h" {
					t.Error("helper selection maps to wrong account")
				}
				a.toggleAccountSection(true)
				if !a.broadcasters.Visible() {
					t.Error("broadcasters did not expand")
				}
				a.broadcasters.SetCurrentIndex(0)
				if row, ok := a.accountRow(); !ok || row.ID != "b" {
					t.Error("broadcaster selection maps to wrong account")
				}
				if a.accounts.CurrentIndex() != -1 {
					t.Error("helper selection not cleared")
				}
				a.updateActions()
				if !a.edit.Enabled() {
					t.Error("broadcaster actions disabled")
				}
				a.toggleAccountSection(true)
				if _, ok := a.accountRow(); ok {
					t.Error("collapsed account remained selected")
				}
				a.mw.Close()
			})
		})
	})
	a.mw.Run()
}
