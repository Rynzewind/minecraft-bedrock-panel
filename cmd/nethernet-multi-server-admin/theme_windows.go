package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"image"
	"image/color"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

var mc = struct{ background, panel, alternate, text, muted, green, red, amber, border walk.Color }{
	walk.RGB(20, 26, 31), walk.RGB(29, 36, 42), walk.RGB(35, 43, 49), walk.RGB(230, 235, 238), walk.RGB(154, 166, 175), walk.RGB(73, 224, 117), walk.RGB(232, 112, 108), walk.RGB(235, 193, 83), walk.RGB(73, 85, 93),
}
var mcUser = syscall.NewLazyDLL("user32.dll")
var mcFill = mcUser.NewProc("FillRect")
var mcMoveMemory = syscall.NewLazyDLL("kernel32.dll").NewProc("RtlMoveMemory")
var mcText = mcUser.NewProc("GetWindowTextW")
var mcGDI = syscall.NewLazyDLL("gdi32.dll")
var mcCreateBrush = mcGDI.NewProc("CreateSolidBrush")
var mcThemeAPI = syscall.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme")
var mcInit sync.Once
var mcBackground, mcPanel, mcLog *walk.SolidColorBrush
var mcTexture *walk.BitmapBrush
var mcTile *walk.Bitmap
var mcBody, mcMono *walk.Font
var mcIcons map[string]*walk.Bitmap
var mcBrushes = map[walk.Color]uintptr{}
var mcSkins = map[win.HWND]*minecraftSkin{}
var mcProc = syscall.NewCallback(minecraftWndProc)

const mcProgressTimer = 0x4e4e

type minecraftSkin struct {
	animationTick   uint64
	tabHeaderHeight int
	previous        uintptr
	kind            string
	button          *walk.PushButton
	tabs            *walk.TabWidget
	hover           bool
	check           *walk.CheckBox
	rowHeight       int
	reflow          func()
	rowImages       map[win.HWND]win.HIMAGELIST
}

func minecraftSkinned(h win.HWND) bool { return mcSkins[h] != nil }
func minecraftButtonTone(text string) walk.Color {
	text = strings.ToLower(text)
	if strings.Contains(text, "restart") {
		return walk.RGB(89, 69, 24)
	}
	if strings.Contains(text, "stop") || strings.Contains(text, "remove") || strings.Contains(text, "uninstall") {
		return walk.RGB(88, 39, 39)
	}
	if strings.Contains(text, "start") || strings.Contains(text, "create") || strings.Contains(text, "apply") || strings.Contains(text, "save") {
		return walk.RGB(29, 77, 46)
	}
	return walk.RGB(44, 54, 63)
}
func initMinecraftResources() {
	mcInit.Do(func() {
		mcBackground, _ = walk.NewSolidColorBrush(mc.background)
		mcPanel, _ = walk.NewSolidColorBrush(mc.panel)
		mcLog, _ = walk.NewSolidColorBrush(walk.RGB(13, 18, 22))
		mcBody, _ = walk.NewFont("Segoe UI", 10, 0)
		mcMono, _ = walk.NewFont("Consolas", 10, 0)
		tile := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				v := uint8((x*17 + y*23 + (x/8)*13) % 5)
				base := uint8(22)
				if x%16 == 0 || y%8 == 0 {
					base = 16
				}
				tile.SetRGBA(x, y, color.RGBA{base + v, base + 5 + v, base + 9 + v, 255})
			}
		}
		bmp, _ := walk.NewBitmapFromImage(tile)
		mcTile = bmp
		if bmp != nil {
			mcTexture, _ = walk.NewBitmapBrush(bmp)
		}
		mcIcons = map[string]*walk.Bitmap{}
		for _, name := range []string{"emerald", "grass", "pickaxe", "chest", "players", "book", "update"} {
			im := minecraftIcon(name)
			b, _ := walk.NewBitmapFromImage(im)
			mcIcons[name] = b
		}
	})
}

// Original compact pixel art: no extracted addon textures or font installation.
func minecraftIcon(name string) image.Image {
	im := image.NewRGBA(image.Rect(0, 0, 32, 32))
	rect := func(x, y, w, h int, c color.RGBA) {
		for yy := y; yy < y+h; yy++ {
			for xx := x; xx < x+w; xx++ {
				im.SetRGBA(xx, yy, c)
			}
		}
	}
	dark := color.RGBA{24, 31, 34, 255}
	green := color.RGBA{58, 198, 91, 255}
	light := color.RGBA{127, 245, 155, 255}
	brown := color.RGBA{124, 81, 42, 255}
	switch name {
	case "update":
		rect(13, 3, 6, 15, light)
		rect(8, 13, 16, 5, green)
		rect(11, 18, 10, 4, green)
		rect(14, 22, 4, 3, green)
		rect(4, 26, 24, 4, brown)
	case "grass":
		rect(3, 5, 26, 24, dark)
		rect(5, 9, 22, 18, brown)
		rect(5, 6, 22, 8, green)
		for i := 0; i < 5; i++ {
			rect(5+i*4, 12, 2, 4, green)
			rect(7+i*4, 19+(i%2)*3, 3, 2, color.RGBA{160, 110, 61, 255})
		}
	case "pickaxe":
		for i := 0; i < 9; i++ {
			rect(6+i*2, 23-i*2, 4, 4, brown)
		}
		rect(5, 4, 21, 5, color.RGBA{46, 210, 204, 255})
		rect(23, 7, 5, 12, color.RGBA{25, 145, 151, 255})
		rect(6, 4, 15, 2, color.RGBA{148, 248, 230, 255})
	case "chest":
		rect(3, 7, 26, 22, dark)
		rect(5, 9, 22, 18, brown)
		rect(5, 14, 22, 3, color.RGBA{59, 39, 24, 255})
		rect(14, 14, 5, 7, color.RGBA{217, 190, 106, 255})
	case "players":
		rect(10, 3, 12, 12, color.RGBA{188, 148, 114, 255})
		rect(8, 17, 16, 13, color.RGBA{57, 159, 165, 255})
		rect(10, 3, 12, 4, brown)
	case "book":
		rect(7, 4, 20, 25, dark)
		rect(6, 4, 18, 23, color.RGBA{164, 104, 66, 255})
		rect(10, 6, 12, 18, color.RGBA{220, 207, 160, 255})
		rect(12, 10, 8, 2, brown)
		rect(12, 15, 8, 2, brown)
	default:
		rect(10, 2, 13, 4, light)
		rect(5, 6, 22, 20, green)
		rect(10, 26, 13, 4, color.RGBA{19, 112, 56, 255})
		rect(5, 6, 5, 16, light)
		rect(23, 8, 4, 18, color.RGBA{20, 130, 65, 255})
		rect(11, 9, 9, 9, color.RGBA{95, 235, 135, 255})
	}
	return im
}
func mcFillRect(dc win.HDC, r win.RECT, c walk.Color) {
	b := mcBrushes[c]
	if b == 0 {
		b, _, _ = mcCreateBrush.Call(uintptr(c))
		mcBrushes[c] = b
	}
	mcFill.Call(uintptr(dc), uintptr(unsafe.Pointer(&r)), b)
}
func mcRect(x, y, w, h int) win.RECT {
	return win.RECT{Left: int32(x), Top: int32(y), Right: int32(x + w), Bottom: int32(y + h)}
}
func mcCaption(dc win.HDC, h win.HWND, text string, r win.RECT, c walk.Color, flags uint32) {
	f := win.SendMessage(h, win.WM_GETFONT, 0, 0)
	if f != 0 {
		old := win.SelectObject(dc, win.HGDIOBJ(f))
		defer win.SelectObject(dc, old)
	}
	old := win.SetTextColor(dc, win.COLORREF(c))
	defer win.SetTextColor(dc, old)
	mode := win.SetBkMode(dc, win.TRANSPARENT)
	defer win.SetBkMode(dc, mode)
	win.DrawTextEx(dc, syscall.StringToUTF16Ptr(text), -1, &r, flags|win.DT_NOPREFIX, nil)
}
func mcNativeText(h win.HWND) string {
	buf := make([]uint16, 2048)
	mcText.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}
func mcBevel(dc win.HDC, r win.RECT, fill, border walk.Color, pressed bool) {
	mcFillRect(dc, r, walk.RGB(7, 11, 14))
	r.Left++
	r.Top++
	r.Right--
	r.Bottom--
	mcFillRect(dc, r, border)
	r.Left += 2
	r.Top += 2
	r.Right -= 2
	r.Bottom -= 2
	mcFillRect(dc, r, fill)
	high := walk.RGB(88, 101, 108)
	if pressed {
		high = walk.RGB(12, 20, 24)
	}
	mcFillRect(dc, win.RECT{Left: r.Left, Top: r.Top, Right: r.Right, Bottom: r.Top + 3}, high)
	mcFillRect(dc, win.RECT{Left: r.Left, Top: r.Top, Right: r.Left + 3, Bottom: r.Bottom}, high)
	low := walk.RGB(13, 19, 22)
	if pressed {
		low = border
	}
	mcFillRect(dc, win.RECT{Left: r.Right - 3, Top: r.Top + 3, Right: r.Right, Bottom: r.Bottom}, low)
	mcFillRect(dc, win.RECT{Left: r.Left + 3, Top: r.Bottom - 3, Right: r.Right, Bottom: r.Bottom}, low)
}
func mcInstall(h win.HWND, s *minecraftSkin) {
	if h == 0 || mcSkins[h] != nil {
		return
	}
	s.previous = win.SetWindowLongPtr(h, win.GWLP_WNDPROC, mcProc)
	mcSkins[h] = s
	win.InvalidateRect(h, nil, true)
}
func minecraftWndProc(h win.HWND, msg uint32, w, l uintptr) uintptr {
	s := mcSkins[h]
	if s == nil {
		return win.DefWindowProc(h, msg, w, l)
	}
	if msg == win.WM_NCDESTROY {
		if s.kind == "progress" {
			win.KillTimer(h, mcProgressTimer)
		}
		for _, images := range s.rowImages {
			win.ImageList_Destroy(images)
		}
		old := s.previous
		delete(mcSkins, h)
		return win.CallWindowProc(old, h, msg, w, l)
	}
	if s.kind == "progress" {
		if msg == win.PBM_SETMARQUEE {
			result := win.CallWindowProc(s.previous, h, msg, w, l)
			if w != 0 {
				win.SetTimer(h, mcProgressTimer, 80, 0)
			} else {
				win.KillTimer(h, mcProgressTimer)
			}
			win.InvalidateRect(h, nil, false)
			return result
		}
		if msg == win.WM_TIMER && w == mcProgressTimer {
			if win.GetWindowLong(h, win.GWL_STYLE)&win.PBS_MARQUEE != 0 {
				s.animationTick++
				win.InvalidateRect(h, nil, false)
			}
			return 0
		}
	}
	if s.kind == "serverRows" || s.kind == "serverRowChild" {
		result := win.CallWindowProc(s.previous, h, msg, w, l)
		if (msg == win.WM_SIZE || msg == win.WM_WINDOWPOSCHANGED) && s.reflow != nil {
			s.reflow()
		}
		return result
	}
	// Common controls can keep a stale default tab height until a stock paint.
	// Buffered skin painting must not expose that frame merely to update layout.
	if s.kind == "tabs" && msg == win.TCM_SETITEMSIZE {
		s.tabHeaderHeight = int((l >> 16) & 0xffff)
	}
	if s.kind == "tabs" && msg == win.TCM_ADJUSTRECT && l != 0 && s.tabHeaderHeight > 0 {
		var rect win.RECT
		mcMoveMemory.Call(uintptr(unsafe.Pointer(&rect)), l, unsafe.Sizeof(rect))
		top := rect.Top
		result := win.CallWindowProc(s.previous, h, msg, w, l)
		mcMoveMemory.Call(uintptr(unsafe.Pointer(&rect)), l, unsafe.Sizeof(rect))
		required := int32(s.tabHeaderHeight + 4)
		if w == 0 && rect.Top < top+required {
			rect.Top = top + required
		}
		if w != 0 && rect.Top > top-required {
			rect.Top = top - required
		}
		mcMoveMemory.Call(l, uintptr(unsafe.Pointer(&rect)), unsafe.Sizeof(rect))
		return result
	}
	if msg == win.WM_ERASEBKGND {
		return 1
	}
	if s.kind == "tabs" && (msg == win.WM_PAINT || msg == win.WM_PRINTCLIENT) {
		dc := win.HDC(w)
		var ps win.PAINTSTRUCT
		if msg == win.WM_PAINT {
			dc = win.BeginPaint(h, &ps)
			defer win.EndPaint(h, &ps)
		}
		if dc != 0 {
			mcPaintTabsBuffered(h, dc, s)
		}
		return 0
	}
	if msg == win.WM_PAINT || msg == win.WM_PRINTCLIENT {
		dc := win.HDC(w)
		var ps win.PAINTSTRUCT
		if msg == win.WM_PAINT {
			dc = win.BeginPaint(h, &ps)
			defer win.EndPaint(h, &ps)
		}
		if dc != 0 {
			mcPaintSkin(h, dc, s)
		}
		return 0
	}
	if msg == win.WM_MOUSEMOVE && !s.hover {
		s.hover = true
		track := win.TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(win.TRACKMOUSEEVENT{})), DwFlags: win.TME_LEAVE, HwndTrack: h}
		win.TrackMouseEvent(&track)
		win.InvalidateRect(h, nil, false)
	}
	if msg == win.WM_MOUSELEAVE {
		s.hover = false
		win.InvalidateRect(h, nil, false)
	}
	result := win.CallWindowProc(s.previous, h, msg, w, l)
	switch msg {
	case win.WM_ENABLE, win.WM_SETFOCUS, win.WM_KILLFOCUS, win.WM_LBUTTONDOWN, win.WM_LBUTTONUP, win.WM_KEYDOWN, win.WM_KEYUP, win.WM_TIMER, win.WM_SETTEXT:
		win.InvalidateRect(h, nil, false)
	}
	return result
}
func mcPaintSkin(h win.HWND, dc win.HDC, s *minecraftSkin) {
	var r win.RECT
	win.GetClientRect(h, &r)
	w, hgt := int(r.Right), int(r.Bottom)
	switch s.kind {
	case "checkbox":
		mcFillRect(dc, r, mc.panel)
		size := 13 * int(win.GetDpiForWindow(h)) / 96
		y := (hgt - size) / 2
		mcBevel(dc, mcRect(0, y, size, size), mc.background, mc.border, false)
		if s.check.Checked() {
			mcCaption(dc, h, "✓", mcRect(0, y, size, size), mc.green, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE)
		}
		r.Left = int32(size + 3*int(win.GetDpiForWindow(h))/96)
		mcCaption(dc, h, s.check.Text(), r, mc.text, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE)
	case "group":
		title := mcNativeText(h)
		caption := minecraftGroupCaptionRect(mcMeasureCaption(dc, h, title), w, int(win.GetDpiForWindow(h)))
		line := max(1, int(caption.Bottom)/2)
		mcFillRect(dc, mcRect(0, line, w, 1), mc.border)
		mcFillRect(dc, mcRect(0, hgt-1, w, 1), mc.border)
		mcFillRect(dc, mcRect(0, line, 1, max(0, hgt-line)), mc.border)
		mcFillRect(dc, mcRect(w-1, line, 1, max(0, hgt-line)), mc.border)
		background := caption
		background.Left -= int32(4 * int(win.GetDpiForWindow(h)) / 96)
		mcFillRect(dc, background, mc.panel)
		mcCaption(dc, h, title, caption, mc.muted, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS)
	case "button":
		text := s.button.Text()
		tone := minecraftButtonTone(text)
		edge := mc.border
		ink := mc.text
		pressed := win.SendMessage(h, win.BM_GETSTATE, 0, 0)&win.BST_PUSHED != 0
		if !s.button.Enabled() {
			tone = mc.panel
			ink = walk.RGB(103, 114, 121)
		} else if s.hover || win.GetFocus() == h {
			edge = mc.green
		}
		mcBevel(dc, r, tone, edge, pressed)
		if pressed {
			r.Top += 2
			r.Left += 2
		}
		r.Left += 8
		r.Right -= 8
		switch {
		case strings.Contains(strings.ToLower(text), "restart"):
			text = "↻  " + text
		case strings.HasPrefix(strings.ToLower(text), "start"):
			text = "▶  " + text
		case strings.HasPrefix(strings.ToLower(text), "stop"):
			text = "■  " + text
		}
		mcCaption(dc, h, text, r, ink, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS)
		if win.GetFocus() == h {
			focus := mcRect(5, 5, w-10, hgt-10)
			win.DrawFocusRect(dc, &focus)
		}
	case "progress":
		mcBevel(dc, r, walk.RGB(10, 17, 12), mc.border, true)
		count := (w - 12) / (20 * int(win.GetDpiForWindow(h)) / 96)
		if count < 1 {
			return
		}
		marquee := win.GetWindowLong(h, win.GWL_STYLE)&win.PBS_MARQUEE != 0
		value := int(win.SendMessage(h, win.PBM_GETPOS, 0, 0))
		max := int(win.SendMessage(h, win.PBM_GETRANGE, 0, 0))
		if max <= 0 {
			max = 100
		}
		phase := int(s.animationTick%uint64(count+4)) - 3
		for i := 0; i < count; i++ {
			c := walk.RGB(39, 57, 20)
			bright := (!marquee && i*max < count*value) || (marquee && i >= phase && i < phase+4)
			if bright {
				c = walk.RGB(109, 223, 43)
			}
			x := 6 + i*(w-12)/count
			cw := (w-12)/count - 3
			mcFillRect(dc, mcRect(x, 5, cw, hgt-10), c)
			if bright {
				mcFillRect(dc, mcRect(x, 5, cw, 2), walk.RGB(181, 249, 86))
			}
		}
	case "tabs":
		mcFillRect(dc, r, mc.background)
		for i := 0; i < s.tabs.Pages().Len(); i++ {
			var item win.RECT
			win.SendMessage(h, win.TCM_GETITEMRECT, uintptr(i), uintptr(unsafe.Pointer(&item)))
			item.Top += 2
			item.Right -= 3
			edge := mc.border
			tone := mc.panel
			ink := mc.muted
			if i == s.tabs.CurrentIndex() {
				edge = mc.green
				tone = walk.RGB(32, 49, 40)
				ink = mc.text
			}
			mcBevel(dc, item, tone, edge, false)
			if i == s.tabs.CurrentIndex() {
				mcFillRect(dc, win.RECT{Left: item.Left + 4, Top: item.Bottom - 4, Right: item.Right - 4, Bottom: item.Bottom - 2}, mc.green)
			}
			iconName := minecraftTabIcon(s.tabs.Pages().At(i).Title())
			size := 26 * int(win.GetDpiForWindow(h)) / 96
			padding := 12 * int(win.GetDpiForWindow(h)) / 96
			if iconName != "" && int(item.Right-item.Left) >= size+3*padding+mcMeasureCaption(dc, h, s.tabs.Pages().At(i).Title()).Width {
				mcPixelIcon(dc, iconName, int(item.Left)+padding, int(item.Top)+(int(item.Bottom-item.Top)-size)/2, size)
				item.Left += int32(size + padding)
			}
			mcCaption(dc, h, s.tabs.Pages().At(i).Title(), item, ink, win.DT_CENTER|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS)
		}
	case "header":
		mcFillRect(dc, r, mc.panel)
		n := int(win.SendMessage(h, win.HDM_GETITEMCOUNT, 0, 0))
		for i := 0; i < n; i++ {
			var cell win.RECT
			win.SendMessage(h, win.HDM_GETITEMRECT, uintptr(i), uintptr(unsafe.Pointer(&cell)))
			buf := make([]uint16, 256)
			item := win.HDITEM{Mask: win.HDI_TEXT, PszText: &buf[0], CchTextMax: 256}
			win.SendMessage(h, win.HDM_GETITEM, uintptr(i), uintptr(unsafe.Pointer(&item)))
			mcBevel(dc, cell, mc.panel, mc.border, false)
			cell.Left += 9
			mcCaption(dc, h, syscall.UTF16ToString(buf), cell, mc.muted, win.DT_LEFT|win.DT_VCENTER|win.DT_SINGLELINE|win.DT_END_ELLIPSIS)
		}
	}
}
func applyMinecraftTheme(root walk.Container) {
	initMinecraftResources()
	root.SetFont(mcBody)
	panelBrush, _ := walk.NewSolidColorBrush(mc.panel)
	logBrush, _ := walk.NewSolidColorBrush(walk.RGB(13, 18, 22))
	root.AsWindowBase().AddDisposable(panelBrush)
	root.AsWindowBase().AddDisposable(logBrush)
	var visit func(walk.Window)
	visit = func(w walk.Window) {
		w.SetBackground(panelBrush)
		if t, ok := w.(interface{ SetTextColor(walk.Color) }); ok {
			t.SetTextColor(mc.text)
		}
		switch v := w.(type) {
		case *walk.PushButton:
			v.SetMinMaxSize(walk.Size{Height: 34}, walk.Size{})
			mcInstall(v.Handle(), &minecraftSkin{kind: "button", button: v})
		case *walk.CheckBox:
			mcInstall(v.Handle(), &minecraftSkin{kind: "checkbox", check: v})
		case *walk.TextEdit:
			v.SetBackground(logBrush)
			v.SetTextColor(mc.text)
			v.SetFont(mcMono)
		case *walk.LineEdit:
			v.SetBackground(logBrush)
			v.SetTextColor(mc.text)
		case *walk.ProgressBar:
			v.SetMinMaxSize(walk.Size{Height: 24}, walk.Size{})
			mcInstall(v.Handle(), &minecraftSkin{kind: "progress"})
		case *walk.TableView:
			v.SetBackground(logBrush)
			v.SetAlternatingRowBG(false)
			v.SetCellStyler(minecraftTableStyler{table: v})
		case *walk.TabWidget:
			child := win.GetWindow(v.Handle(), win.GW_CHILD)
			for child != 0 {
				buf := make([]uint16, 64)
				win.GetClassName(child, &buf[0], len(buf))
				if syscall.UTF16ToString(buf) == "SysTabControl32" {
					mcInstall(child, &minecraftSkin{kind: "tabs", tabs: v})
					win.SetWindowLong(child, win.GWL_STYLE, win.GetWindowLong(child, win.GWL_STYLE)|win.TCS_FIXEDWIDTH)
					resize := func() {
						width := (v.ClientBoundsPixels().Width - 10) / v.Pages().Len()
						if width > 0 {
							win.SendMessage(child, win.TCM_SETITEMSIZE, 0, uintptr(uint32(width)|uint32(40*v.DPI()/96)<<16))
							// Refresh Walk's cached non-client height after changing the
							// native tab height, otherwise layout can cover the labels.
							v.Pages().At(0).SetTitle(v.Pages().At(0).Title())
							v.RequestLayout()
							var page win.RECT
							win.GetClientRect(child, &page)
							win.SendMessage(child, win.TCM_ADJUSTRECT, 0, uintptr(unsafe.Pointer(&page)))
							for i := 0; i < v.Pages().Len(); i++ {
								v.Pages().At(i).SetBoundsPixels(walk.Rectangle{X: int(page.Left), Y: int(page.Top), Width: int(page.Right - page.Left), Height: int(page.Bottom - page.Top)})
							}
							win.InvalidateRect(child, nil, true)
						}
					}
					resize()
					v.SizeChanged().Attach(resize)
					v.CurrentIndexChanged().Attach(func() { win.InvalidateRect(child, nil, true) })
					break
				}
				child = win.GetWindow(child, win.GW_HWNDNEXT)
			}
			for i := 0; i < v.Pages().Len(); i++ {
				visit(v.Pages().At(i))
			}
		}
		if c, ok := w.(walk.Container); ok {
			for i := 0; i < c.Children().Len(); i++ {
				visit(c.Children().At(i))
			}
		}
	}
	for i := 0; i < root.Children().Len(); i++ {
		visit(root.Children().At(i))
	}
	if mcTexture != nil {
		background, _ := walk.NewBitmapBrush(mcTile)
		root.SetBackground(background)
	}
	// Native table headers are implementation children, not Walk widgets.
	var scan func(win.HWND)
	scan = func(parent win.HWND) {
		for h := win.GetWindow(parent, win.GW_CHILD); h != 0; h = win.GetWindow(h, win.GW_HWNDNEXT) {
			buf := make([]uint16, 80)
			win.GetClassName(h, &buf[0], len(buf))
			class := syscall.UTF16ToString(buf)
			if class == "SysHeader32" {
				mcInstall(h, &minecraftSkin{kind: "header"})
			}
			if class == "Button" && win.GetWindowLong(h, win.GWL_STYLE)&0xf == win.BS_GROUPBOX {
				mcInstall(h, &minecraftSkin{kind: "group"})
			}
			if class == "SysListView32" {
				win.SendMessage(h, win.LVM_SETBKCOLOR, 0, uintptr(mcLog.Color()))
				win.SendMessage(h, win.LVM_SETTEXTBKCOLOR, 0, uintptr(mcLog.Color()))
				win.SendMessage(h, win.LVM_SETTEXTCOLOR, 0, uintptr(mc.text))
			}
			scan(h)
		}
	}
	scan(root.Handle())
}

type minecraftTableStyler struct{ table *walk.TableView }

func (s minecraftTableStyler) StyleCell(cell *walk.CellStyle) {
	cell.BackgroundColor = mc.panel
	if cell.Row()%2 == 1 {
		cell.BackgroundColor = mc.alternate
	}
	if s.table.CurrentIndex() == cell.Row() {
		cell.BackgroundColor = walk.RGB(32, 62, 43)
	}
	cell.TextColor = mc.text
	if model, ok := s.table.Model().(*serverTableModel); ok && cell.Row() < len(model.rows) && cell.Row() >= 0 {
		row := model.rows[cell.Row()]
		cell.Font = mcBody
		if cell.Col() == 0 {
			if canvas := cell.Canvas(); canvas != nil {
				bounds := cell.Bounds()
				brush, _ := walk.NewSolidColorBrush(cell.BackgroundColor)
				canvas.FillRectangle(brush, bounds)
				brush.Dispose()
				name := "pickaxe"
				if row.ID == "creative" {
					name = "grass"
				}
				size := 40
				if bounds.Height < 52 {
					size = 24
				}
				canvas.DrawImageStretched(mcIcons[name], walk.Rectangle{X: bounds.X + 10, Y: bounds.Y + (bounds.Height-size)/2, Width: size, Height: size})
				canvas.DrawText(model.Value(cell.Row(), 0).(string), mcBody, mc.text, walk.Rectangle{X: bounds.X + size + 20, Y: bounds.Y, Width: bounds.Width - size - 24, Height: bounds.Height}, walk.TextLeft|walk.TextVCenter|walk.TextSingleLine|walk.TextEndEllipsis)
			}
		}
		if cell.Col() == 1 {
			switch row.State {
			case "running":
				cell.TextColor = mc.green
			case "failed":
				cell.TextColor = mc.red
			default:
				cell.TextColor = mc.amber
			}
		}
		if cell.Col() == 4 && row.Pending != "" {
			cell.TextColor = mc.amber
		}
	}
}

// A drop-in wrapper themes existing dialogs without changing their workflows.
type themedDialog Dialog

func (d themedDialog) Create(owner walk.Form) error {
	if err := Dialog(d).Create(owner); err != nil {
		return err
	}
	if d.AssignTo != nil && *d.AssignTo != nil {
		applyMinecraftTheme(*d.AssignTo)
	}
	return nil
}
func (d themedDialog) Run(owner walk.Form) (int, error) {
	var local *walk.Dialog
	if d.AssignTo == nil {
		d.AssignTo = &local
	}
	if err := d.Create(owner); err != nil {
		return 0, err
	}
	dlg := *d.AssignTo
	defer dlg.Dispose()
	return dlg.Run(), nil
}

var mcGlyphs = map[rune]string{
	'A': "01110/10001/10001/11111/10001/10001/10001", 'B': "11110/10001/10001/11110/10001/10001/11110", 'C': "01111/10000/10000/10000/10000/10000/01111", 'D': "11110/10001/10001/10001/10001/10001/11110", 'E': "11111/10000/10000/11110/10000/10000/11111", 'F': "11111/10000/10000/11110/10000/10000/10000", 'G': "01111/10000/10000/10111/10001/10001/01111", 'H': "10001/10001/10001/11111/10001/10001/10001", 'I': "11111/00100/00100/00100/00100/00100/11111", 'J': "00111/00010/00010/00010/10010/10010/01100", 'K': "10001/10010/10100/11000/10100/10010/10001", 'L': "10000/10000/10000/10000/10000/10000/11111", 'M': "10001/11011/10101/10101/10001/10001/10001", 'N': "10001/11001/10101/10011/10001/10001/10001", 'O': "01110/10001/10001/10001/10001/10001/01110", 'P': "11110/10001/10001/11110/10000/10000/10000", 'Q': "01110/10001/10001/10001/10101/10010/01101", 'R': "11110/10001/10001/11110/10100/10010/10001", 'S': "01111/10000/10000/01110/00001/00001/11110", 'T': "11111/00100/00100/00100/00100/00100/00100", 'U': "10001/10001/10001/10001/10001/10001/01110", 'V': "10001/10001/10001/10001/10001/01010/00100", 'W': "10001/10001/10001/10101/10101/10101/01010", 'X': "10001/10001/01010/00100/01010/10001/10001", 'Y': "10001/10001/01010/00100/00100/00100/00100", 'Z': "11111/00001/00010/00100/01000/10000/11111"}

func minecraftPixelTitle(canvas *walk.Canvas, text string, x, y, scale int) {
	initMinecraftResources()
	shadow, _ := walk.NewSolidColorBrush(walk.RGB(5, 9, 12))
	defer shadow.Dispose()
	ink, _ := walk.NewSolidColorBrush(mc.text)
	defer ink.Dispose()
	green, _ := walk.NewSolidColorBrush(mc.green)
	defer green.Dispose()
	for i, ch := range text {
		brush := walk.Brush(ink)
		if text == "NETHERNET" && i >= 6 {
			brush = green
		}
		for yy, row := range strings.Split(mcGlyphs[ch], "/") {
			for xx, bit := range row {
				if bit == '1' {
					r := walk.Rectangle{X: x + i*6*scale + xx*scale, Y: y + yy*scale, Width: scale, Height: scale}
					drop := r
					drop.X += 2
					drop.Y += 3
					canvas.FillRectangle(shadow, drop)
					canvas.FillRectangle(brush, r)
				}
			}
		}
	}
}
func minecraftBanner(title, subtitle string, compact bool) CustomWidget {
	height := 80
	if compact {
		height = 65
	}
	return windowDPICustomWidget(CustomWidget{MinSize: Size{Height: height}, MaxSize: Size{Height: height}, PaintMode: PaintBuffered, InvalidatesOnResize: true, Paint: func(c *walk.Canvas, r walk.Rectangle) error {
		initMinecraftResources()
		if mcTexture != nil {
			c.FillRectangle(mcTexture, r)
		} else {
			c.FillRectangle(mcBackground, r)
		}
		if icon := mcIcons["grass"]; icon != nil {
			c.DrawImageStretched(icon, walk.Rectangle{X: 12, Y: 12, Width: 44, Height: 44})
		}
		size := 4
		if compact {
			size = 3
		}
		plaqueWidth := len(title)*6*size + 24
		plaque, _ := walk.NewSolidColorBrush(walk.RGB(47, 55, 58))
		shadow, _ := walk.NewSolidColorBrush(walk.RGB(7, 11, 14))
		light, _ := walk.NewSolidColorBrush(walk.RGB(100, 110, 111))
		c.FillRectangle(shadow, walk.Rectangle{X: 66, Y: 7, Width: plaqueWidth + 4, Height: 7*size + 14})
		c.FillRectangle(plaque, walk.Rectangle{X: 68, Y: 7, Width: plaqueWidth, Height: 7*size + 10})
		c.FillRectangle(light, walk.Rectangle{X: 68, Y: 7, Width: plaqueWidth, Height: 3})
		c.FillRectangle(light, walk.Rectangle{X: 68, Y: 7, Width: 3, Height: 7*size + 10})
		plaque.Dispose()
		shadow.Dispose()
		light.Dispose()
		minecraftPixelTitle(c, title, 80, 12, size)
		c.DrawText(subtitle, mcBody, mc.muted, walk.Rectangle{X: 72, Y: height - 29, Width: r.Width - 84, Height: 22}, walk.TextLeft|walk.TextSingleLine)
		edge, _ := walk.NewSolidColorBrush(mc.green)
		defer edge.Dispose()
		return c.FillRectangle(edge, walk.Rectangle{X: 0, Y: height - 2, Width: r.Width, Height: 2})
	}})
}

func paintMinecraftSteps(c *walk.Canvas, r walk.Rectangle, steps []minecraftStep, failed bool) error {
	initMinecraftResources()
	c.FillRectangle(mcPanel, r)
	for i, step := range steps {
		ink := mc.muted
		mark := "□"
		if step.State == 2 {
			ink = mc.green
			mark = "✓"
		}
		if step.State == 1 {
			ink = mc.green
			mark = "▶"
			if failed {
				ink = mc.red
				mark = "!"
			}
		}
		c.DrawText(mark, mcBody, ink, walk.Rectangle{X: 8, Y: 6 + i*26, Width: 24, Height: 24}, walk.TextVCenter|walk.TextSingleLine)
		c.DrawText(step.Text, mcBody, ink, walk.Rectangle{X: 38, Y: 6 + i*26, Width: r.Width - 46, Height: 24}, walk.TextVCenter|walk.TextSingleLine)
	}
	return nil
}

// Render original sprites as crisp rectangles in native control coordinates.
func mcPixelIcon(dc win.HDC, name string, x, y, size int) {
	im := minecraftIcon(name)
	for yy := 0; yy < 32; yy++ {
		for xx := 0; xx < 32; xx++ {
			r, g, b, a := im.At(xx, yy).RGBA()
			if a == 0 {
				continue
			}
			left, top := x+xx*size/32, y+yy*size/32
			right, bottom := x+(xx+1)*size/32, y+(yy+1)*size/32
			if right > left && bottom > top {
				mcFillRect(dc, mcRect(left, top, right-left, bottom-top), walk.RGB(byte(r>>8), byte(g>>8), byte(b>>8)))
			}
		}
	}
}

// Keep the native tab's layout/cache work, but never expose its stock frame.
// The final blit uses the paint DC's clip, preserving the page child windows.
func mcPaintTabsBuffered(h win.HWND, dc win.HDC, s *minecraftSkin) {
	var bounds win.RECT
	win.GetClientRect(h, &bounds)
	if bounds.Right <= 0 || bounds.Bottom <= 0 {
		return
	}
	memory := win.CreateCompatibleDC(dc)
	if memory == 0 {
		mcPaintSkin(h, dc, s)
		return
	}
	defer win.DeleteDC(memory)
	bitmap := win.CreateCompatibleBitmap(dc, bounds.Right, bounds.Bottom)
	if bitmap == 0 {
		mcPaintSkin(h, dc, s)
		return
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))
	previous := win.SelectObject(memory, win.HGDIOBJ(bitmap))
	defer win.SelectObject(memory, previous)
	mcFillRect(memory, bounds, mc.background)
	win.CallWindowProc(s.previous, h, win.WM_PRINTCLIENT, uintptr(memory), win.PRF_CLIENT)
	mcPaintSkin(h, memory, s)
	win.BitBlt(dc, 0, 0, bounds.Right, bounds.Bottom, memory, 0, 0, win.SRCCOPY)
}
