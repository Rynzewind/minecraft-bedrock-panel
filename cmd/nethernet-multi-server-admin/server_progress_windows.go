package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"net"
	"nethernet-multi-server/internal/supervisor"
	"strings"
	"time"
)

// An owned modal dialog disables the main window, including actions on other
// tabs. Closing it cannot interrupt a world save or an addon transaction.
type serverProgressDialog struct {
	operationStage string
	dlg            *walk.Dialog
	bar            *walk.ProgressBar
	stage          *walk.Label
	log            *walk.TextEdit
	closeButton    *walk.PushButton
	finished       bool
	steps          *walk.CustomWidget
	stepState      []minecraftStep
	failed         bool
}

func (view *serverProgressDialog) closeOnSuccess() {
	if view.finished && !view.failed && !view.dlg.IsDisposed() {
		view.dlg.Accept()
	}
}

func newServerProgressDialog(owner walk.Form, name string) (*serverProgressDialog, error) {
	var dlg *walk.Dialog
	var bar *walk.ProgressBar
	var stage *walk.Label
	var log *walk.TextEdit
	var closeButton *walk.PushButton
	view := &serverProgressDialog{}
	spec := themedDialog{AssignTo: &dlg, Title: "Server progress - " + name, MinSize: Size{Width: 760, Height: 510}, Layout: VBox{}, Children: []Widget{minecraftBanner("SERVER PROGRESS", "Please wait while the selected operation finishes", true),
		Label{AssignTo: &stage, Text: "Preparing server operation...", MinSize: Size{Height: 55}},
		ProgressBar{AssignTo: &bar, MaxValue: 100},
		windowDPICustomWidget(CustomWidget{AssignTo: &view.steps, MinSize: Size{Height: 112}, MaxSize: Size{Height: 112}, PaintMode: PaintBuffered, Paint: func(c *walk.Canvas, r walk.Rectangle) error {
			return paintMinecraftSteps(c, r, view.stepState, view.failed)
		}}),
		Label{Text: "Latest Minecraft output:"},
		TextEdit{AssignTo: &log, ReadOnly: true, MinSize: Size{Height: 65}},
		Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &closeButton, Text: "Close", Enabled: false, OnClicked: func() { dlg.Accept() }}}},
	}}
	if err := spec.Create(owner); err != nil {
		return nil, err
	}
	dlg.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		if !view.finished {
			*canceled = true
		}
	})
	_ = bar.SetMarqueeMode(true)

	view.dlg = dlg
	view.bar = bar
	view.stage = stage
	view.log = log
	view.closeButton = closeButton
	return view, nil
}

type serverOperation struct {
	row    ServerRow
	method string
	work   func(context.Context) error
}

func (a *adminWindow) runServerOperation(row ServerRow, method string, work func(context.Context) error) {
	a.runServerOperations(row.Name, []serverOperation{{row: row, method: method, work: work}}, nil)
}

func (a *adminWindow) runServerOperations(title string, operations []serverOperation, skipped []string) {
	view, err := newServerProgressDialog(a.mw, title)
	if err != nil {
		a.appendEvent(err.Error())
		return
	}
	defer view.dlg.Dispose()
	started := time.Now()
	go func() {
		var failures []string
		var completed []string
		for i, op := range operations {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			notify := func(text, latest string) {
				a.sync(func() {
					if view.dlg.IsDisposed() {
						return
					}
					view.stage.SetText(fmt.Sprintf("%s (%d of %d)\r\n%s", op.row.Name, i+1, len(operations), text))
					view.stepState = minecraftProgressSteps(op.method, text)
					view.steps.Invalidate()
					view.log.SetText(latest)
				})
			}
			err := a.monitorServerOperation(ctx, op, notify)
			timedOut := ctx.Err() != nil
			cancel()
			if err != nil {
				failures = append(failures, op.row.Name+": "+err.Error())
			} else {
				completed = append(completed, op.row.Name)
			}
			if timedOut {
				for _, remaining := range operations[i+1:] {
					skipped = append(skipped, remaining.row.Name+" (earlier operation timed out)")
				}
				break
			}
		}
		a.sync(func() {
			if view.dlg.IsDisposed() {
				return
			}
			view.finished = true
			view.failed = len(failures) > 0 || len(skipped) > 0
			if !view.failed {
				for i := range view.stepState {
					view.stepState[i].State = 2
				}
			}
			view.steps.Invalidate()
			_ = view.bar.SetMarqueeMode(false)
			summary := fmt.Sprintf("Completed in %s", time.Since(started).Truncate(time.Second))
			if len(completed) > 0 {
				summary += "\r\nSucceeded: " + strings.Join(completed, ", ")
			}
			if len(skipped) > 0 {
				summary += "\r\nSkipped: " + strings.Join(skipped, ", ")
			}
			if len(failures) > 0 {
				summary = "Needs attention - " + summary + "\r\n" + strings.Join(failures, "\r\n")
			} else {
				view.bar.SetValue(100)
			}
			view.stage.SetText(summary)
			a.appendEvent(summary)
			view.closeButton.SetEnabled(true)
			_ = view.closeButton.SetFocus()
			a.refresh()
			view.closeOnSuccess()
		})
	}()
	view.dlg.Run()
}

func (a *adminWindow) monitorServerOperation(ctx context.Context, op serverOperation, notify func(string, string)) error {
	started := time.Now()
	notify(serverProgressText(op.method, supervisor.Status{}, 0, false), "")
	command := make(chan error, 1)
	go func() { command <- op.work(ctx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	commandDone := false
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("Progress monitoring timed out. The server may still be saving/applying changes; inspect its state before retrying. No process was force-stopped.")
		case err := <-command:
			command = nil
			if err != nil {
				if ctx.Err() != nil {
					continue
				}
				return err
			}
			commandDone = true
		case <-ticker.C:
		}
		// Saving settings and applying packs have no additional process readiness step.
		if commandDone && (op.method == "packs.apply" || op.method == "settings.save") {
			return nil
		}
		query, qcancel := context.WithTimeout(ctx, 2*time.Second)
		var progress supervisor.Progress
		err := a.client.Call(query, "server.progress", map[string]string{"server_id": op.row.ID}, &progress)
		qcancel()
		if err != nil {
			notify(fmt.Sprintf("Waiting for helper progress - %s elapsed\r\n%s", time.Since(started).Truncate(time.Second), err), "")
			continue
		}
		s := progress.Status
		if commandDone && (s.State == supervisor.Failed || ((op.method == "server.start" || op.method == "server.restart") && s.State == supervisor.Stopped)) {
			notify("Minecraft stopped before becoming ready", lastProgressLogLine(progress.Log))
			return fmt.Errorf("Minecraft did not stay running: %s", s.LastError)
		}
		ready := false
		if commandDone && (op.method == "server.start" || op.method == "server.restart") && s.State == supervisor.Running && s.Backend != "" {
			probe, pcancel := context.WithTimeout(ctx, 750*time.Millisecond)
			conn, e := (&net.Dialer{}).DialContext(probe, "tcp4", s.Backend)
			pcancel()
			if e == nil {
				_ = conn.Close()
				ready = true
			}
		}
		text := serverProgressText(op.method, s, time.Since(started), false)
		if !commandDone && s.Phase == "running" {
			text = fmt.Sprintf("Waiting for command acknowledgement - %s elapsed", time.Since(started).Truncate(time.Second))
		}
		notify(text, lastProgressLogLine(progress.Log))
		if commandDone && serverOperationComplete(op.method, s, ready) {
			return nil
		}
	}
}

func (a *adminWindow) allServerAction(method string) { a.fleetAction(method, false) }
