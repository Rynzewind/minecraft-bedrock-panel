package main

import (
	"github.com/lxn/walk"
	"runtime"
	"testing"
)

func TestCustomPaintingUsesWindowDPINotTargetCanvasDPI(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, pair := range [][2]int{{96, 288}, {288, 96}, {144, 288}} {
		windowDPI, canvasDPI := pair[0], pair[1]
		bmp, e := walk.NewBitmapForDPI(walk.Size{Width: 800, Height: 160}, canvasDPI)
		if e != nil {
			t.Fatal(e)
		}
		target, e := walk.NewCanvasFromImage(bmp)
		if e != nil {
			t.Fatal(e)
		}
		called := false
		e = paintAtWindowDPI(target, walk.Size{Width: 800, Height: 160}, windowDPI, func(c *walk.Canvas, r walk.Rectangle) error {
			called = true
			if c.DPI() != windowDPI {
				t.Errorf("paint DPI %d != window DPI %d (target %d)", c.DPI(), windowDPI, canvasDPI)
			}
			if r.Height != walk.IntTo96DPI(160, windowDPI) {
				t.Errorf("paint bounds %+v", r)
			}
			return nil
		})
		target.Dispose()
		bmp.Dispose()
		if e != nil || !called {
			t.Fatalf("paint: %v called=%v", e, called)
		}
	}
}
