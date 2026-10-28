package main

import (
	"errors"
	"github.com/lxn/win"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestRepeatedPollingPreservesNativeRowsSelectionAndFonts(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	rows := []ServerRow{{ID: "creative", Name: "Creative", State: "running"}, {ID: "survival", Name: "Survival", State: "running"}}
	a.serverModel.set(rows)
	time.AfterFunc(400*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			defer a.mw.Close()
			a.servers.SetCurrentIndex(1)
			skin := mcSkins[a.servers.Handle()]
			fonts := map[win.HWND]win.HIMAGELIST{}
			for h, f := range skin.rowImages {
				fonts[h] = f
			}
			for i := 0; i < 25; i++ {
				if i%2 == 1 {
					rows[0].Players++
				}
				a.serverModel.set(rows)
				for h := range fonts {
					win.InvalidateRect(h, nil, false)
					win.UpdateWindow(h)
				}
				if a.servers.CurrentIndex() != 1 {
					t.Errorf("poll %d cleared selected server", i)
					break
				}
				for h, font := range fonts {
					if skin.rowImages[h] != font {
						t.Errorf("poll %d recreated row metrics during painting", i)
					}
					if count := win.SendMessage(h, 0x1004 /* LVM_GETITEMCOUNT */, 0, 0); count != 2 {
						t.Errorf("poll %d native count=%d", i, count)
					}
					rect := win.RECT{Left: win.LVIR_BOUNDS}
					if win.SendMessage(h, win.LVM_GETITEMRECT, 1, uintptr(unsafe.Pointer(&rect))) == 0 || rect.Bottom <= rect.Top {
						t.Errorf("poll %d lost second server rectangle", i)
					}
				}
			}
			a.applyDisconnected(errors.New("temporary server list failure"))
			if a.serverModel.RowCount() != 2 || a.servers.CurrentIndex() != 1 {
				t.Error("connection failure erased existing servers")
			}
		})
	})
	a.mw.Run()
}

func TestServerRowsShareAvailableListHeight(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	a.serverModel.set([]ServerRow{{ID: "one", Name: "One", State: "stopped"}, {ID: "two", Name: "Two", State: "stopped"}})
	time.AfterFunc(300*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			stage := 0
			check := func(count int) {
				stage++
				for child := win.GetWindow(a.servers.Handle(), win.GW_CHILD); child != 0; child = win.GetWindow(child, win.GW_HWNDNEXT) {
					buf := make([]uint16, 80)
					win.GetClassName(child, &buf[0], len(buf))
					if syscall.UTF16ToString(buf) != "SysListView32" {
						continue
					}
					var client win.RECT
					win.GetClientRect(child, &client)
					if client.Right < 10 {
						continue
					}
					var row win.RECT
					row.Left = win.LVIR_BOUNDS
					win.SendMessage(child, win.LVM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&row)))
					available := client.Bottom - row.Top
					got := row.Bottom - row.Top
					hit := win.LVHITTESTINFO{Pt: win.POINT{X: 50, Y: row.Top + int32(count-1)*got + got/2}}
					index := int32(win.SendMessage(child, win.LVM_HITTEST, 0, uintptr(unsafe.Pointer(&hit))))
					if index != int32(count-1) {
						t.Errorf("last-row hit test=%d want %d", index, count-1)
					}
					if got < available/int32(count)-2 || got > available/int32(count)+2 {
						t.Errorf("stage=%d row height=%d available=%d count=%d client=%+v row=%+v cached=%d childSkin=%v", stage, got, available, count, client, row, mcSkins[a.servers.Handle()].rowHeight, mcSkins[child] != nil)
					}
				}
			}
			check(2)
			bounds := a.servers.BoundsPixels()
			bounds.Height += 80
			a.servers.SetBoundsPixels(bounds)
			time.AfterFunc(150*time.Millisecond, func() {
				a.mw.Synchronize(func() {
					check(2)
					a.serverModel.set([]ServerRow{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}, {ID: "three", Name: "Three"}})
					check(3)
					a.serverModel.set([]ServerRow{{ID: "one", Name: "One"}})
					check(1)
					a.serverModel.set(nil)
					a.mw.Close()
				})
			})
		})
	})
	a.mw.Run()
}
