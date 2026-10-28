package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// Memory DCs report system DPI, which may differ from a per-monitor window.
// Build the backing canvas with the window DPI and copy the result in pixels.
func paintAtWindowDPI(target *walk.Canvas, size walk.Size, dpi int, paint walk.PaintFunc) error {
	if size.Width <= 0 || size.Height <= 0 {
		return nil
	}
	bmp, e := walk.NewBitmapForDPI(size, dpi)
	if e != nil {
		return e
	}
	defer bmp.Dispose()
	canvas, e := walk.NewCanvasFromImage(bmp)
	if e != nil {
		return e
	}
	e = paint(canvas, walk.Rectangle{Width: walk.IntTo96DPI(size.Width, dpi), Height: walk.IntTo96DPI(size.Height, dpi)})
	canvas.Dispose()
	if e != nil {
		return e
	}
	return target.DrawImagePixels(bmp, walk.Point{})
}
func windowDPICustomWidget(spec CustomWidget) CustomWidget {
	assigned := spec.AssignTo
	if assigned == nil {
		assigned = new(*walk.CustomWidget)
		spec.AssignTo = assigned
	}
	paint := spec.Paint
	spec.Paint = nil
	spec.PaintMode = PaintNoErase
	spec.PaintPixels = func(c *walk.Canvas, _ walk.Rectangle) error {
		w := *assigned
		if w == nil || w.IsDisposed() {
			return nil
		}
		return paintAtWindowDPI(c, w.ClientBoundsPixels().Size(), w.DPI(), paint)
	}
	return spec
}
