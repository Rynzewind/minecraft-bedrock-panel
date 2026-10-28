package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/walk"
	"github.com/lxn/win"
)

func accountListHeight(rows, rowHeight, headerHeight, scrollbarHeight, borderHeight int) int {
	return max(1, min(rows, 8))*rowHeight + headerHeight + scrollbarHeight + borderHeight
}

func installContentSizedAccountList(table *walk.TableView, group *walk.GroupBox, model walk.TableModel) {
	updating := false
	lastHeight := 0
	resize := func() {
		if updating || table.IsDisposed() || group.IsDisposed() {
			return
		}
		updating = true
		defer func() { updating = false }()
		rowHeight, headerHeight := table.IntFrom96DPI(20), table.IntFrom96DPI(30)
		for child := win.GetWindow(table.Handle(), win.GW_CHILD); child != 0; child = win.GetWindow(child, win.GW_HWNDNEXT) {
			var name [80]uint16
			win.GetClassName(child, &name[0], len(name))
			if syscall.UTF16ToString(name[:]) != "SysListView32" {
				continue
			}
			var header win.RECT
			if win.GetWindowRect(win.HWND(win.SendMessage(child, win.LVM_GETHEADER, 0, 0)), &header) && header.Bottom > header.Top {
				headerHeight = int(header.Bottom - header.Top)
			}
			row := win.RECT{Left: win.LVIR_BOUNDS}
			if model.RowCount() > 0 && win.SendMessage(child, win.LVM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&row))) != 0 && row.Bottom > row.Top {
				rowHeight = int(row.Bottom - row.Top)
			}
		}
		pixels := accountListHeight(model.RowCount(), rowHeight, headerHeight, int(win.GetSystemMetricsForDpi(win.SM_CYHSCROLL, uint32(table.DPI()))), table.IntFrom96DPI(4))
		height := (pixels*96 + table.DPI() - 1) / table.DPI()
		if height == lastHeight {
			return
		}
		lastHeight = height
		table.SetMinMaxSize(walk.Size{Height: height}, walk.Size{Height: height})
		// Use the actual caption, action bar and layout margins rather than
		// allocating the remaining tab height to an almost empty account pool.
		minimum := walk.CreateLayoutItemsForContainer(group).MinSize()
		// Walk's group layout consumes two extra native pixels at its bottom.
		// Round up when converting back to logical units at high display DPI.
		groupHeight := ((minimum.Height+2)*96 + group.DPI() - 1) / group.DPI()
		group.SetMinMaxSize(walk.Size{}, walk.Size{Height: groupHeight})
	}
	model.RowsReset().Attach(resize)
	table.SizeChanged().Attach(resize)
	resize()
}
