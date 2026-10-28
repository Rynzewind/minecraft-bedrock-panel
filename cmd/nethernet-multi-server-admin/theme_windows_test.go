package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestMinecraftButtonSkinPreservesNativeClickAndDisabledState(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, e := walk.NewMainWindow()
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Dispose()
	owner.SetLayout(walk.NewVBoxLayout())
	b, e := walk.NewPushButton(owner)
	if e != nil {
		t.Fatal(e)
	}
	b.SetText("Start All Servers")
	b.SetBounds(walk.Rectangle{Width: 220, Height: 40})
	clicks := 0
	b.Clicked().Attach(func() { clicks++ })
	applyMinecraftTheme(owner)
	if !minecraftSkinned(b.Handle()) {
		t.Fatal("button skin not installed")
	}
	win.SendMessage(b.Handle(), win.BM_CLICK, 0, 0)
	if clicks != 1 {
		t.Fatal("native click did not reach action")
	}
	b.SetEnabled(false)
	if b.Enabled() {
		t.Fatal("disabled state lost")
	}
}
func TestMinecraftActionColoursRemainDistinct(t *testing.T) {
	if minecraftButtonTone("Start All Servers") == minecraftButtonTone("Stop All Servers") {
		t.Fatal("start and stop have same tone")
	}
	if minecraftButtonTone("Restart") == minecraftButtonTone("Stop") {
		t.Fatal("restart and stop have same tone")
	}
}
func TestMinecraftTabContentDoesNotCoverNavigation(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	time.AfterFunc(250*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			for hwnd, skin := range mcSkins {
				if skin.kind == "tabs" && skin.tabs == a.navigation {
					var item, tab, page win.RECT
					win.GetWindowRect(hwnd, &tab)
					win.GetWindowRect(a.navigation.Pages().At(0).Handle(), &page)
					win.SendMessage(hwnd, win.TCM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&item)))
					if page.Top < tab.Top+item.Bottom-2 {
						t.Errorf("page overlaps tabs: pageTop=%d tabBottom=%d", page.Top, tab.Top+item.Bottom)
					}
				}
			}
			a.mw.Close()
		})
	})
	a.mw.Run()
}
func TestMinecraftBrushesSurviveClosingAnotherWindow(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner, e := walk.NewMainWindow()
	if e != nil {
		t.Fatal(e)
	}
	owner.SetLayout(walk.NewVBoxLayout())
	applyMinecraftTheme(owner)
	owner.Dispose()
	bmp, e := walk.NewBitmap(walk.Size{Width: 12, Height: 12})
	if e != nil {
		t.Fatal(e)
	}
	defer bmp.Dispose()
	canvas, e := walk.NewCanvasFromImage(bmp)
	if e != nil {
		t.Fatal(e)
	}
	defer canvas.Dispose()
	if e = canvas.FillRectangle(mcPanel, walk.Rectangle{Width: 12, Height: 12}); e != nil {
		t.Fatalf("shared paint brush was invalidated: %v", e)
	}
}

// Native tab layout must still run, but its intermediate paint must never
// reach the visible window before the Minecraft skin has finished.
func TestMinecraftTabsDoNotExposeNativeIntermediatePaint(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if err := a.create(); err != nil {
		t.Fatal(err)
	}
	defer a.mw.Dispose()
	a.mw.SetBoundsPixels(walk.Rectangle{Width: 1280, Height: 900})
	time.AfterFunc(250*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			defer a.mw.Close()
			for hwnd, skin := range mcSkins {
				if skin.kind != "tabs" || skin.tabs != a.navigation {
					continue
				}
				previous := skin.previous
				visibleNativePaints, bufferedNativePaints := 0, 0
				spy := syscall.NewCallback(func(h win.HWND, msg uint32, w, l uintptr) uintptr {
					if msg == win.WM_PAINT && w == 0 {
						visibleNativePaints++
					}
					if msg == win.WM_PRINTCLIENT && w != 0 {
						bufferedNativePaints++
					}
					return win.CallWindowProc(previous, h, msg, w, l)
				})
				skin.previous = spy
				for i := 0; i < a.navigation.Pages().Len(); i++ {
					a.navigation.SetCurrentIndex(i)
					win.InvalidateRect(hwnd, nil, false)
					win.SendMessage(hwnd, win.WM_PAINT, 0, 0)
				}
				skin.previous = previous
				if visibleNativePaints != 0 {
					t.Errorf("native tab frames were painted directly to the window %d times", visibleNativePaints)
				}
				if bufferedNativePaints == 0 {
					t.Error("native tab layout was not painted into the offscreen buffer")
				}
				var item win.RECT
				win.SendMessage(hwnd, win.TCM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&item)))
				if item.Right <= item.Left || item.Bottom <= item.Top {
					t.Errorf("native tab layout lost: %+v", item)
				}
				return
			}
			t.Error("navigation tab skin not found")
		})
	})
	a.mw.Run()
}
