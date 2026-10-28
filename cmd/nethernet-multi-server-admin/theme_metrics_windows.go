package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"syscall"
)

func mcMeasureCaption(dc win.HDC, h win.HWND, text string) walk.Size {
	f := win.SendMessage(h, win.WM_GETFONT, 0, 0)
	if f != 0 {
		old := win.SelectObject(dc, win.HGDIOBJ(f))
		defer win.SelectObject(dc, old)
	}
	var r win.RECT
	win.DrawTextEx(dc, syscall.StringToUTF16Ptr(text), -1, &r, win.DT_CALCRECT|win.DT_SINGLELINE|win.DT_NOPREFIX, nil)
	return walk.Size{Width: int(r.Right - r.Left), Height: int(r.Bottom - r.Top)}
}
func minecraftGroupCaptionRect(text walk.Size, width, dpi int) win.RECT {
	pad := max(1, 4*dpi/96)
	left := 15 * dpi / 96
	return mcRect(left, 0, max(0, min(text.Width+pad, width-left-pad)), text.Height+pad)
}
func minecraftTabIcon(title string) string {
	switch title {
	case "Servers":
		return "grass"
	case "Content Packs", "Add-ons":
		return "chest"
	case "Accounts":
		return "players"
	case "Player History":
		return "book"
	case "Migration / Snapshots", "Snapshots":
		return "emerald"
	case "Updates":
		return "update"
	}
	return ""
}
