package main

import (
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// Keep the outer window on the monitor; the scrollable content retains access
// to controls when display scaling leaves less room than the preferred layout.
func fitAdministratorToWorkArea(window *walk.MainWindow) {
	var info win.MONITORINFO
	info.CbSize = uint32(unsafe.Sizeof(info))
	if !win.GetMonitorInfo(win.MonitorFromWindow(window.Handle(), win.MONITOR_DEFAULTTONEAREST), &info) {
		return
	}
	work := walk.Rectangle{X: int(info.RcWork.Left), Y: int(info.RcWork.Top), Width: int(info.RcWork.Right - info.RcWork.Left), Height: int(info.RcWork.Bottom - info.RcWork.Top)}
	window.SetMinMaxSizePixels(walk.Size{Width: min(window.IntFrom96DPI(640), work.Width*85/100), Height: min(window.IntFrom96DPI(420), work.Height*85/100)}, walk.Size{})
	window.SetBoundsPixels(administratorWorkAreaBounds(window.BoundsPixels(), work))
}

func administratorWorkAreaBounds(bounds, work walk.Rectangle) walk.Rectangle {
	bounds.Width = min(bounds.Width, work.Width*85/100)
	bounds.Height = min(bounds.Height, work.Height*85/100)
	bounds.X = max(work.X, min(bounds.X, work.X+work.Width-bounds.Width))
	bounds.Y = max(work.Y, min(bounds.Y, work.Y+work.Height-bounds.Height))
	return bounds
}
func fitProgressDialogToWorkArea(window *walk.Dialog) {
	var info win.MONITORINFO
	info.CbSize = uint32(unsafe.Sizeof(info))
	if !win.GetMonitorInfo(win.MonitorFromWindow(window.Handle(), win.MONITOR_DEFAULTTONEAREST), &info) {
		return
	}
	work := walk.Rectangle{X: int(info.RcWork.Left), Y: int(info.RcWork.Top), Width: int(info.RcWork.Right - info.RcWork.Left), Height: int(info.RcWork.Bottom - info.RcWork.Top)}
	window.SetMinMaxSizePixels(walk.Size{Width: min(window.IntFrom96DPI(480), work.Width*85/100), Height: min(window.IntFrom96DPI(320), work.Height*85/100)}, walk.Size{})
	window.SetBoundsPixels(administratorWorkAreaBounds(window.BoundsPixels(), work))
}

// Add-on settings contains a table, API controls and a staging review.  It is
// a working surface rather than a small confirmation dialog, so give it a
// substantial share of the active monitor on high-resolution displays.
func fitAddonSettingsDialogToWorkArea(window *walk.Dialog) {
	var info win.MONITORINFO
	info.CbSize = uint32(unsafe.Sizeof(info))
	if !win.GetMonitorInfo(win.MonitorFromWindow(window.Handle(), win.MONITOR_DEFAULTTONEAREST), &info) {
		return
	}
	work := walk.Rectangle{X: int(info.RcWork.Left), Y: int(info.RcWork.Top), Width: int(info.RcWork.Right - info.RcWork.Left), Height: int(info.RcWork.Bottom - info.RcWork.Top)}
	minimum := walk.Size{Width: min(window.IntFrom96DPI(640), work.Width*85/100), Height: min(window.IntFrom96DPI(440), work.Height*85/100)}
	window.SetMinMaxSizePixels(minimum, walk.Size{})
	bounds := window.BoundsPixels()
	bounds.Width = min(max(bounds.Width, work.Width*72/100), work.Width*92/100)
	bounds.Height = min(max(bounds.Height, work.Height*78/100), work.Height*92/100)
	bounds.X = work.X + (work.Width-bounds.Width)/2
	bounds.Y = work.Y + (work.Height-bounds.Height)/2
	window.SetBoundsPixels(bounds)
}
