package main

import (
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"syscall"
	"unsafe"
)

func installAdaptiveServerRows(table *walk.TableView, model *serverTableModel) {
	skin := &minecraftSkin{kind: "serverRows", rowImages: map[win.HWND]win.HIMAGELIST{}}
	mcInstall(table.Handle(), skin)
	updating := false
	skin.reflow = func() {
		if updating || table.IsDisposed() {
			return
		}
		updating = true
		defer func() { updating = false }()
		// A count change may remove scrollbars. Recheck the usable body height
		// after updating native metrics, without queueing or rebuilding the table.
		changed := false
		for layoutPass := 0; layoutPass < 3; layoutPass++ {
			var children []win.HWND
			available := 0
			widest := int32(0)
			for child := win.GetWindow(table.Handle(), win.GW_CHILD); child != 0; child = win.GetWindow(child, win.GW_HWNDNEXT) {
				buf := make([]uint16, 80)
				win.GetClassName(child, &buf[0], len(buf))
				if syscall.UTF16ToString(buf) != "SysListView32" {
					continue
				}
				mcInstall(child, &minecraftSkin{kind: "serverRowChild", reflow: skin.reflow})
				// The parent skin owns these metric image lists; prevent the native
				// children from also destroying them during window disposal.
				if style := win.GetWindowLong(child, win.GWL_STYLE); style&win.LVS_SHAREIMAGELISTS == 0 {
					win.SetWindowLong(child, win.GWL_STYLE, style|win.LVS_SHAREIMAGELISTS)
				}
				children = append(children, child)
				var rect, header win.RECT
				win.GetClientRect(child, &rect)
				if rect.Right < 10 || rect.Right <= widest {
					continue
				}
				widest = rect.Right
				hh := win.HWND(win.SendMessage(child, win.LVM_GETHEADER, 0, 0))
				win.GetWindowRect(hh, &header)
				available = int(rect.Bottom - (header.Bottom - header.Top))
				// The first item includes native header/scrollbar offsets that
				// can settle after a font/layout change. Use the actual body.
				if model.RowCount() > 0 {
					first := win.RECT{Left: win.LVIR_BOUNDS}
					if win.SendMessage(child, win.LVM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&first))) != 0 && first.Top >= 0 && first.Top < rect.Bottom {
						available = int(rect.Bottom - first.Top)
					}
				}
			}
			if available <= 0 {
				return
			}
			minimum := 30 * table.DPI() / 96
			height := minimum
			if count := model.RowCount(); count > 0 {
				height = (available - count) / count
				if height < minimum {
					height = minimum
				}
			}
			if height == skin.rowHeight {
				settled := true
				if model.RowCount() > 0 {
					for _, child := range children {
						first := win.RECT{Left: win.LVIR_BOUNDS}
						if win.SendMessage(child, win.LVM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&first))) != 0 && int(first.Bottom-first.Top) != height {
							settled = false
						}
					}
				}
				if settled {
					break
				}
			}
			changed = true
			skin.rowHeight = height
			for _, child := range children {
				desired := height - 1
				// Native report rows take their minimum height from the small image
				// list. An empty spacer list keeps metrics stable when custom drawing
				// selects the normal body font; no font changes during painting.
				for pass := 0; pass < 3; pass++ {
					fresh := win.ImageList_Create(1, int32(desired), win.ILC_COLOR32, 0, 1)
					if fresh == 0 {
						break
					}
					win.SendMessage(child, win.LVM_SETIMAGELIST, win.LVSIL_SMALL, uintptr(fresh))
					if previous := skin.rowImages[child]; previous != 0 {
						win.ImageList_Destroy(previous)
					}
					skin.rowImages[child] = fresh
					if model.RowCount() == 0 {
						break
					}
					rect := win.RECT{Left: win.LVIR_BOUNDS}
					win.SendMessage(child, win.LVM_GETITEMRECT, 0, uintptr(unsafe.Pointer(&rect)))
					actual := int(rect.Bottom - rect.Top)
					if actual == height || actual <= 0 {
						break
					}
					desired += height - actual
					if desired < 1 {
						desired = 1
					}
				}
			}
		}
		if changed {
			table.Invalidate()
		}
	}
	model.RowsReset().Attach(skin.reflow)
	skin.reflow()
}
