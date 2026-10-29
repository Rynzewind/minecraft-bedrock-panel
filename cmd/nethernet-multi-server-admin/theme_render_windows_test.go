package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"image"
	"image/png"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

func TestRenderMinecraftTheme(t *testing.T) {
	dir := os.Getenv("NN_THEME_RENDER_DIR")
	if dir == "" {
		t.Skip("explicit preview export only")
	}
	os.MkdirAll(dir, 0755)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	a := &adminWindow{}
	if e := a.create(); e != nil {
		t.Fatal(e)
	}
	defer a.mw.Dispose()
	a.mw.SetBoundsPixels(walk.Rectangle{X: 0, Y: 0, Width: 1600, Height: 950})
	a.serverModel.set([]ServerRow{BuildServerRow(supervisor.Status{ID: "creative", Name: "Creative", State: supervisor.Running, PID: 8420, Players: 2, Pending: true}), BuildServerRow(supervisor.Status{ID: "survival", Name: "Survival", State: supervisor.Running, PID: 9216, Players: 1})})
	a.status.SetText("Helper connected - visual preview")
	a.serverSummary.SetText("Creative: 2  |  Survival: 1  |  Active: 3")
	a.serverLog.SetText("[Server] Starting Minecraft Bedrock server...\r\n[Server] Level Name: Creative\r\n[Server] Server started.\r\n[Server] Player joined Creative\r\n[Server] Pending addon changes will apply at next restart.")
	a.events.SetText("Creative server started successfully\r\nSurvival server started successfully\r\nAddon changes staged for Creative - restart required")
	save := func(window walk.Window, name string) {
		t.Logf("render %s bounds=%+v", name, window.SizePixels())
		im, e := captureThemeWindow(window)
		if e != nil {
			t.Error(e)
			return
		}
		f, e := os.Create(filepath.Join(dir, name))
		if e != nil {
			t.Error(e)
			return
		}
		defer f.Close()
		if e = png.Encode(f, im); e != nil {
			t.Error(e)
		}
	}
	time.AfterFunc(700*time.Millisecond, func() {
		a.mw.Synchronize(func() {
			for hwnd, skin := range mcSkins {
				if skin.kind == "tabs" {
					for i := 0; i < skin.tabs.Pages().Len(); i++ {
						var r win.RECT
						win.SendMessage(hwnd, win.TCM_GETITEMRECT, uintptr(i), uintptr(unsafe.Pointer(&r)))
						t.Logf("tab %d rect %+v page %+v", i, r, skin.tabs.Pages().At(i).BoundsPixels())
					}
				}
			}
			save(a.mw, "actual-server-panel.png")
			view, e := newServerProgressDialog(a.mw, "All servers")
			if e != nil {
				t.Error(e)
				a.mw.Close()
				return
			}
			defer view.dlg.Dispose()
			view.stage.SetText("Creative (1 of 2)\r\nApplying addon/settings changes and creating backups - 1m24s elapsed")
			view.stepState = minecraftProgressSteps("server.restart", "Applying addon/settings changes and creating backups")
			view.log.SetText("[Server] Stopping server...\r\n[Server] World saved.")
			time.AfterFunc(650*time.Millisecond, func() {
				view.dlg.Synchronize(func() { save(view.dlg, "actual-progress-popup.png"); view.finished = true; view.dlg.Accept() })
			})
			view.dlg.Run()
			a.mw.Close()
		})
	})
	a.mw.Run()
}

func captureThemeWindow(window walk.Window) (*image.RGBA, error) {
	var r win.RECT
	win.GetClientRect(window.Handle(), &r)
	w, h := int(r.Right), int(r.Bottom)
	dc := win.GetDC(window.Handle())
	defer win.ReleaseDC(window.Handle(), dc)
	mem := win.CreateCompatibleDC(dc)
	defer win.DeleteDC(mem)
	header := win.BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})), BiWidth: int32(w), BiHeight: -int32(h), BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB}
	var bits unsafe.Pointer
	bmp := win.CreateDIBSection(dc, &header, win.DIB_RGB_COLORS, &bits, 0, 0)
	defer win.DeleteObject(win.HGDIOBJ(bmp))
	old := win.SelectObject(mem, win.HGDIOBJ(bmp))
	defer win.SelectObject(mem, old)
	win.BitBlt(mem, 0, 0, int32(w), int32(h), dc, 0, 0, win.SRCCOPY)
	raw := unsafe.Slice((*byte)(bits), w*h*4)
	im := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(raw); i += 4 {
		im.Pix[i] = raw[i+2]
		im.Pix[i+1] = raw[i+1]
		im.Pix[i+2] = raw[i]
		im.Pix[i+3] = 255
	}
	return im, nil
}
