package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	"github.com/lxn/win"
	"nethernet-multi-server/internal/fleet"
	"nethernet-multi-server/internal/supervisor"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type fleetRuntime struct {
	engine   *fleet.Engine
	desired  bool
	recovery fleet.Recovery
}

func (a *adminWindow) initFleet() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	a.fleetState = &fleetRuntime{engine: fleet.NativeEngine(filepath.Dir(exe)), desired: true}
	return nil
}
func fleetMethod(method string) string {
	switch method {
	case "server.start":
		return "start-all"
	case "server.stop":
		return "stop-all"
	case "server.restart":
		return "restart-all"
	}
	return method
}
func (a *adminWindow) recordFleet(message string) {
	a.appendEvent(message)
	exe, err := os.Executable()
	if err == nil {
		if f, e := os.OpenFile(filepath.Join(filepath.Dir(exe), "panel-lifecycle.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); e == nil {
			fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), message)
			f.Close()
		}
	}
	if a.tray != nil {
		text := []rune("NetherNet: " + message)
		if len(text) > 100 {
			text = text[:100]
		}
		a.tray.SetToolTip(string(text))
	}
}
func (a *adminWindow) fleetAction(method string, silent bool) {
	if a.fleetState == nil || a.closed.Load() || !win.IsWindowEnabled(a.mw.Handle()) || a.fleetBusy.Load() {
		return
	}
	action := fleetMethod(method)
	if !silent && action != "start-all" && a.state.ActiveCount > 0 {
		if walk.MsgBox(a.mw, "Confirm all-server action", fmt.Sprintf("%d player(s) are connected. Save and stop all servers?", a.state.ActiveCount), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
			return
		}
	}
	if !a.fleetBusy.CompareAndSwap(false, true) {
		return
	}
	a.fleetState.desired = action != "stop-all"
	if !silent {
		a.fleetState.recovery.Reset()
	}
	a.updateManagementActions()
	var view *serverProgressDialog
	if !silent {
		var err error
		view, err = newServerProgressDialog(a.mw, "All servers")
		if err != nil {
			a.fleetBusy.Store(false)
			a.updateManagementActions()
			a.appendEvent(err.Error())
			return
		}
		defer view.dlg.Dispose()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		started := time.Now()
		if view != nil {
			go a.monitorFleetProgress(ctx, view, started)
		}
		err := a.fleetState.engine.Run(ctx, action, func(message string) {
			a.sync(func() {
				a.recordFleet(message)
				if view != nil && !view.dlg.IsDisposed() {
					view.operationStage = message
					view.stage.SetText(message)
					view.stepState = []minecraftStep{{Text: message, State: 1}}
					view.steps.Invalidate()
				}
			})
		})
		cancel()
		a.sync(func() {
			a.fleetBusy.Store(false)
			message := "All servers are ready"
			if action == "stop-all" {
				message = "All servers and helper stopped; automatic recovery paused"
			}
			if err != nil {
				message = "Needs attention: " + err.Error()
			}
			a.recordFleet(message)
			if view != nil && !view.dlg.IsDisposed() {
				view.finished = true
				view.failed = err != nil
				view.bar.SetMarqueeMode(false)
				if err == nil {
					view.bar.SetValue(100)
					view.stepState = []minecraftStep{{Text: message, State: 2}}
				}
				view.steps.Invalidate()
				view.stage.SetText(message)
				view.closeButton.SetEnabled(true)
				view.closeButton.SetFocus()
				view.closeOnSuccess()
			} else if err != nil && a.tray != nil {
				a.tray.ShowError("NetherNet needs attention", message)
			}
			a.updateManagementActions()
			a.refresh()
		})
	}()
	if view != nil {
		view.dlg.Run()
	}
}

func (a *adminWindow) monitorFleetProgress(ctx context.Context, view *serverProgressDialog, started time.Time) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		query, cancel := context.WithTimeout(ctx, 2*time.Second)
		var states []supervisor.Status
		err := a.client.Call(query, "servers.list", struct{}{}, &states)
		var lines []string
		if err == nil {
			for _, state := range states {
				var progress supervisor.Progress
				if a.client.Call(query, "server.progress", map[string]string{"server_id": state.ID}, &progress) == nil {
					lines = append(lines, fmt.Sprintf("%s: %s\r\n%s", state.Name, state.State, lastProgressLogLine(progress.Log)))
				}
			}
		}
		cancel()
		a.sync(func() {
			if view.dlg.IsDisposed() || view.finished {
				return
			}
			view.stage.SetText(fmt.Sprintf("%s\r\n%s elapsed", view.operationStage, time.Since(started).Truncate(time.Second)))
			if len(lines) > 0 {
				view.log.SetText(strings.Join(lines, "\r\n\r\n"))
			}
		})
	}
}
func (a *adminWindow) startRecovery() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.closed.Load() {
			return
		}
		if a.fleetBusy.Load() {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		var states []supervisor.Status
		err := a.client.Call(ctx, "servers.list", struct{}{}, &states)
		cancel()
		failed := err != nil
		for _, s := range states {
			if s.State == supervisor.Failed {
				failed = true
			}
		}
		if failed {
			a.sync(func() {
				if a.fleetState == nil || !a.fleetState.desired || a.fleetBusy.Load() || !win.IsWindowEnabled(a.mw.Handle()) {
					return
				}
				if !a.fleetState.recovery.Allow(time.Now()) {
					return
				}
				a.recordFleet("Unexpected failure detected; attempting bounded recovery")
				a.fleetAction("recover", true)
			})
		}
	}
}
