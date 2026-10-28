package main

import (
	"crypto/sha256"
	"fmt"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

func adminInstance(exe string, startup bool) (windows.Handle, bool, error) {
	absolute, err := filepath.Abs(exe)
	if err != nil {
		return 0, false, err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(absolute)))
	handle, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr(fmt.Sprintf(`Local\NetherNet.Admin.%x`, sum[:12])))
	if err != nil && err != windows.ERROR_ALREADY_EXISTS {
		return 0, false, err
	}
	if err == windows.ERROR_ALREADY_EXISTS {
		if !startup {
			showExistingAdmin(absolute)
		}
		windows.CloseHandle(handle)
		return 0, false, nil
	}
	return handle, true, nil
}
func showExistingAdmin(exe string) {
	callback := syscall.NewCallback(func(hwnd win.HWND, _ uintptr) uintptr {
		var pid uint32
		win.GetWindowThreadProcessId(hwnd, &pid)
		if pid == uint32(os.Getpid()) {
			return 1
		}
		h, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if e != nil {
			return 1
		}
		defer windows.CloseHandle(h)
		buf := make([]uint16, 32768)
		size := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &size) != nil || !strings.EqualFold(windows.UTF16ToString(buf[:size]), exe) {
			return 1
		}
		title := mcNativeText(hwnd)
		if !strings.HasPrefix(title, "NetherNet Multi-Server Administrator") {
			return 1
		}
		win.ShowWindow(hwnd, win.SW_MAXIMIZE)
		win.SetForegroundWindow(hwnd)
		return 0
	})
	syscall.NewLazyDLL("user32.dll").NewProc("EnumWindows").Call(callback, 0)
}
func (a *adminWindow) showPanel() {
	a.mw.Show()
	win.ShowWindow(a.mw.Handle(), win.SW_MAXIMIZE)
	win.SetForegroundWindow(a.mw.Handle())
}
func (a *adminWindow) setupTray() error {
	if a.tray != nil {
		return nil
	}
	icon := a.mw.Icon()
	create := a.trayCreate
	if create == nil {
		create = walk.NewNotifyIcon
	}
	tray, err := create(a.mw)
	if err != nil {
		// Walk can fail after NIM_ADD but before completing construction. Remove
		// only this window's default icon so the next attempt can register afresh.
		nid := win.NOTIFYICONDATA{HWnd: a.mw.Handle()}
		nid.CbSize = uint32(unsafe.Sizeof(nid) - unsafe.Sizeof(win.HICON(0)))
		win.Shell_NotifyIcon(win.NIM_DELETE, &nid)
		return err
	}
	success := false
	defer func() {
		if !success {
			tray.Dispose()
			tray.ContextMenu().Dispose()
		}
	}()
	var actions []*walk.Action
	if err = tray.SetIcon(icon); err != nil {
		return err
	}
	if err = tray.SetToolTip("NetherNet Minecraft servers"); err != nil {
		return err
	}
	add := func(text string, fn func(), control bool) error {
		action := walk.NewAction()
		action.SetText(text)
		action.Triggered().Attach(fn)
		if control {
			actions = append(actions, action)
		}
		return tray.ContextMenu().Actions().Add(action)
	}
	if err = add("Open Panel", a.showPanel, false); err != nil {
		return err
	}
	for _, spec := range []struct{ text, method string }{{"Start All Servers", "start-all"}, {"Stop All Servers", "stop-all"}, {"Restart All Servers", "restart-all"}} {
		spec := spec
		if err = add(spec.text, func() { a.fleetAction(spec.method, false) }, true); err != nil {
			return err
		}
	}
	if err = add("Hide Panel", func() { a.mw.Hide() }, false); err != nil {
		return err
	}
	if err = add("Exit Panel (leave servers running)", func() {
		if a.fleetBusy.Load() {
			return
		}
		if walk.MsgBox(a.mw, "Exit panel", "Leave the servers running and exit the panel? Automatic crash recovery pauses until the panel opens again.", walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) == walk.DlgCmdYes {
			a.quitting = true
			a.mw.Close()
		}
	}, true); err != nil {
		return err
	}
	tray.MouseUp().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			a.showPanel()
		}
	})
	if err = tray.SetVisible(true); err != nil {
		return err
	}
	a.tray = tray
	a.trayActions = actions
	success = true
	a.installTrayHook()
	a.updateManagementActions()
	return nil
}

const trayRetryTimer = 0x4e54

func (a *adminWindow) installTrayHook() {
	if a.trayPrevious != 0 {
		return
	}
	trayOwners[a.mw.Handle()] = a
	a.trayPrevious = win.SetWindowLongPtr(a.mw.Handle(), win.GWLP_WNDPROC, trayProc)
}
func (a *adminWindow) startTrayRecovery() {
	a.installTrayHook()
	a.retryTray()
}
func (a *adminWindow) retryTray() {
	if a.tray != nil || a.mw.IsDisposed() {
		return
	}
	if err := a.setupTray(); err != nil {
		a.trayFailures++
		if a.trayFailures == 1 {
			a.appendEvent("Windows tray is not ready; server startup continues while registration retries. " + strings.Split(err.Error(), "\n")[0])
		}
		if a.trayFailures == 15 {
			a.appendEvent("Tray still unavailable. Opening the panel so server controls remain accessible.")
			a.showPanel()
		}
		if win.SetTimer(a.mw.Handle(), trayRetryTimer, 2000, 0) == 0 {
			a.appendEvent("Tray retry timer unavailable. Opening panel.")
			a.showPanel()
		}
		return
	}
	win.KillTimer(a.mw.Handle(), trayRetryTimer)
	if a.trayFailures > 0 {
		a.appendEvent("Windows tray connected successfully.")
	}
}
func (a *adminWindow) disposeTray() {
	win.KillTimer(a.mw.Handle(), trayRetryTimer)
	if a.tray != nil {
		a.tray.Dispose()
		a.tray.ContextMenu().Dispose()
		a.tray = nil
		a.trayActions = nil
	}
}

var trayOwners = map[win.HWND]*adminWindow{}
var trayProc uintptr

func init() { trayProc = syscall.NewCallback(trayWindowProc) }

func trayWindowProc(h win.HWND, message uint32, w, l uintptr) uintptr {
	a := trayOwners[h]
	if a == nil {
		return win.DefWindowProc(h, message, w, l)
	}
	previous := a.trayPrevious
	if message == win.WM_TIMER && w == trayRetryTimer {
		a.retryTray()
		return 0
	}
	if message == win.WM_NCDESTROY {
		win.KillTimer(h, trayRetryTimer)
		delete(trayOwners, h)
	}
	if message == win.WM_QUERYENDSESSION {
		a.quitting = true
	}
	if message == win.WM_SYSCOMMAND && w&0xfff0 == win.SC_CLOSE && !a.quitting && a.tray != nil {
		a.mw.Hide()
		return 0
	}
	return win.CallWindowProc(previous, h, message, w, l)
}
