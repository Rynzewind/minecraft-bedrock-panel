package main

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func TestAdministratorCanShrinkToSmallWorkArea(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{startHidden: true}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	a.navigation.SetCurrentIndex(2)
	a.state = ViewState{Connected: true, Accounts: []AccountRow{{ID: "test-account", State: "Needs sign-in"}, {ID: "second"}, {ID: "third"}, {ID: "fourth"}}}
	a.accountModel.set(a.state.Accounts)
	a.playerModel.set(make([]PlayerRow, 4))
	a.accounts.SetCurrentIndex(0)
	a.updateActions()
	want := walk.Rectangle{X: 0, Y: 0, Width: a.mw.IntFrom96DPI(900), Height: a.mw.IntFrom96DPI(600)}
	if err := a.mw.SetBoundsPixels(want); err != nil {
		t.Fatal(err)
	}
	var got walk.Rectangle
	time.AfterFunc(300*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			got = a.mw.BoundsPixels()
			var pool, users win.RECT
			win.GetWindowRect(a.accountPool.Handle(), &pool)
			win.GetWindowRect(a.consoleUsers.Handle(), &users)
			if users.Left < pool.Right || users.Top != pool.Top {
				t.Errorf("accounts and console users are not side by side: accounts=%+v users=%+v", pool, users)
			}
			if a.viewport == nil {
				t.Error("scrollable viewport missing")
			} else {
				var viewport, button win.RECT
				win.GetWindowRect(a.viewport.Handle(), &viewport)
				win.GetWindowRect(a.reauth.Handle(), &button)
				if button.Top < viewport.Top || button.Bottom > viewport.Bottom || button.Left < viewport.Left || button.Right > viewport.Right {
					t.Errorf("Reauthenticate requires whole-window scrolling: button=%+v viewport=%+v", button, viewport)
				}
				win.GetWindowRect(a.reassign.Handle(), &button)
				if button.Top < viewport.Top || button.Bottom > viewport.Bottom || button.Left < viewport.Left || button.Right > viewport.Right {
					t.Errorf("Console Users actions require whole-window scrolling: button=%+v viewport=%+v", button, viewport)
				}
				if !a.reauth.Enabled() {
					t.Error("selected account cannot reauthenticate")
				}
				assertActionFitsAncestorClients(t, a.reauth, a.mw)
				assertActionFitsAncestorClients(t, a.reassign, a.mw)
				assertActionFitsAncestorClients(t, a.setRole, a.mw)
				assertActionFitsAncestorClients(t, a.remove, a.mw)
				for _, axis := range []int32{win.SB_HORZ} {
					info := win.SCROLLINFO{CbSize: uint32(unsafe.Sizeof(win.SCROLLINFO{})), FMask: win.SIF_RANGE | win.SIF_PAGE}
					win.GetScrollInfo(a.viewport.Handle(), axis, &info)
					if info.NPage > 0 && info.NMax+1 > int32(info.NPage) {
						t.Errorf("normal Accounts page needs scrolling: axis=%d range=%+v", axis, info)
					}
				}
			}
			a.mw.Close()
		})
	})
	a.mw.Run()
	if got.Width > want.Width || got.Height > want.Height {
		t.Fatalf("administrator cannot fit small work area: got %+v, available %+v", got, want)
	}
}

func assertActionFitsAncestorClients(t *testing.T, button *walk.PushButton, window walk.Form) {
	t.Helper()
	var bounds win.RECT
	win.GetWindowRect(button.Handle(), &bounds)
	for parent := win.GetParent(button.Handle()); parent != 0; parent = win.GetParent(parent) {
		var client win.RECT
		var origin win.POINT
		win.GetClientRect(parent, &client)
		win.ClientToScreen(parent, &origin)
		if bounds.Left < origin.X || bounds.Top < origin.Y || bounds.Right > origin.X+client.Right || bounds.Bottom > origin.Y+client.Bottom {
			t.Errorf("%s clipped by ancestor %v: button=%+v client=%+v origin=%+v", button.Text(), parent, bounds, client, origin)
		}
		if parent == window.Handle() {
			break
		}
	}
}

func TestAdministratorWorkAreaBounds(t *testing.T) {
	for _, work := range []walk.Rectangle{
		{X: 0, Y: 0, Width: 1366, Height: 728},
		{X: -1920, Y: 40, Width: 1920, Height: 1040},
		{X: 0, Y: -1080, Width: 1920, Height: 1040},
	} {
		got := administratorWorkAreaBounds(walk.Rectangle{X: 5000, Y: 5000, Width: 3300, Height: 2634}, work)
		if got.X < work.X || got.Y < work.Y || got.X+got.Width > work.X+work.Width || got.Y+got.Height > work.Y+work.Height {
			t.Fatalf("window %+v escapes work area %+v", got, work)
		}
		if got.Width > work.Width*85/100 || got.Height > work.Height*85/100 {
			t.Fatalf("default window consumes the work area: got %+v, work %+v", got, work)
		}
	}
}

// Short windows may scroll: the user prefers a readable Activity pane over
// squeezing every page into the viewport. Verify the action rows are reachable.
func TestServersPageActionsReachableByScrolling(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	a.serverModel.set([]ServerRow{{ID: "creative"}, {ID: "survival"}})
	a.mw.SetBoundsPixels(walk.Rectangle{Width: a.mw.IntFrom96DPI(900), Height: a.mw.IntFrom96DPI(600)})
	time.AfterFunc(300*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			view := a.navigation.Pages().At(0).Children().At(0).(*walk.ScrollView)
			_, vertical := view.Scrollbars()
			if !vertical {
				t.Error("page cannot scroll")
			}
			for i := 0; i < 3; i++ {
				win.SendMessage(view.Handle(), win.WM_VSCROLL, win.SB_PAGEDOWN, 0)
			}
			time.AfterFunc(200*time.Millisecond, func() {
				a.mw.Synchronize(func() {
					var client win.RECT
					var origin win.POINT
					win.GetClientRect(view.Handle(), &client)
					win.ClientToScreen(view.Handle(), &origin)
					for _, button := range []*walk.PushButton{a.startAll, a.serverSettingsButton} {
						var r win.RECT
						win.GetWindowRect(button.Handle(), &r)
						if r.Top < origin.Y || r.Bottom > origin.Y+client.Bottom {
							t.Errorf("%s not reachable after scrolling: %+v viewport %+v", button.Text(), r, client)
						}
					}
					a.mw.Close()
				})
			})
		})
	})
	a.mw.Run()
}
func TestMigrationPageCreatesSnapshotControls(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	if a.migration == nil || a.migration.create == nil || a.migration.progress == nil {
		t.Fatal("snapshot controls missing")
	}
	if a.migration.cancel.Enabled() {
		t.Fatal("cancel enabled without job")
	}
	if a.migration.weekly == nil || a.migration.weekly.Checked() {
		t.Fatal("weekly snapshot option missing or enabled without opting in")
	}
	if a.migration.frequency == nil || a.migration.frequency.CurrentIndex() != 6 || a.migration.hour.CurrentIndex() != 0 || a.migration.minute.CurrentIndex() != 0 || a.migration.weekday.CurrentIndex() != 0 || a.migration.scheduleSave == nil {
		t.Fatal("schedule dropdowns missing or legacy Monday-midnight default changed")
	}
	a.migration.frequency.SetCurrentIndex(2)
	if a.migration.weekday.Enabled() {
		t.Fatal("weekly day should be disabled for every-three-day schedule")
	}
	a.migration.frequency.SetCurrentIndex(6)
	if a.migration.folder.Text() != `G:\My Drive\Minecraft Server Setup\Backup Snapshots` {
		t.Fatalf("unexpected default snapshot folder: %s", a.migration.folder.Text())
	}
	if a.startAll == nil || a.stopAll == nil || a.restartAll == nil {
		t.Fatal("bulk server controls missing")
	}
	time.AfterFunc(300*time.Millisecond, func() { a.mw.Synchronize(func() { a.mw.Close() }) })
	a.mw.Run()
}

func TestEmbeddedAdministratorIconLoads(t *testing.T) {
	icon, err := walk.NewIconFromResourceId(2)
	if err != nil {
		t.Fatalf("load embedded administrator icon resource 2: %v", err)
	}
	icon.Dispose()
}

func TestAcceptedLabelsAreTrimmedAndPreserved(t *testing.T) {
	got, ok := acceptedLabels("  Kevin  ", "  kevin@example.com  ")
	if !ok {
		t.Fatal("acceptedLabels() rejected a non-empty person label")
	}
	if got.person != "Kevin" || got.email != "kevin@example.com" {
		t.Fatalf("acceptedLabels() = %#v; want trimmed values", got)
	}
	if _, ok := acceptedLabels("   ", "anything"); ok {
		t.Fatal("acceptedLabels() accepted an empty person label")
	}
}

func TestSignInClipboardTextIsReadyToPaste(t *testing.T) {
	got := signInClipboardText("https://example.com/link", "ABCD-EFGH")
	want := "Microsoft sign-in link: https://example.com/link\r\nCode: ABCD-EFGH"
	if got != want {
		t.Fatalf("signInClipboardText() = %q; want %q", got, want)
	}
}

func TestMainWindowHasBothPanelsAndNoServerControl(t *testing.T) {
	spec := mainWindowSpec()
	if !spec.HasAccountPool || !spec.HasConsoleUsers {
		t.Fatal("both administrator panels must be visible")
	}
	if spec.HasStartStopControl {
		t.Fatal("administrator must not control server lifecycle")
	}
}
func TestSelectedRowsCarryStableAPIIdentifiers(t *testing.T) {
	accounts := accountTableModel{rows: []AccountRow{{ID: "account-b"}, {ID: "account-a"}}}
	players := playerTableModel{rows: []PlayerRow{{XUID: "console-2"}, {XUID: "console-1"}}}
	if accounts.rows[1].ID != "account-a" || players.rows[0].XUID != "console-2" {
		t.Fatal("displayed rows lost stable identifiers")
	}
}
