package main

import (
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/serverupdate"
	"time"
)

func updateProgressSteps(phase string, done, failed bool) []minecraftStep {
	labels := []string{"Download and validate official server software", "Save worlds and stop servers", "Create and verify recovery backup", "Install server software", "Restart servers and check connections"}
	indices := map[string]int{"downloading": 0, "stopping": 1, "backup": 2, "installing": 3, "starting": 4, "complete": 5}
	active, ok := indices[phase]
	if !ok {
		active = 0
	}
	out := make([]minecraftStep, len(labels))
	for i, text := range labels {
		state := 0
		if phase == "complete" && done && !failed || i < active {
			state = 2
		} else if i == active {
			state = 1
		}
		out[i] = minecraftStep{Text: text, State: state}
	}
	return out
}
func newUpdateProgressDialog(owner walk.Form) (*serverProgressDialog, error) {
	v := &serverProgressDialog{}
	spec := themedDialog{AssignTo: &v.dlg, Title: "Minecraft server update", MinSize: Size{Width: 480, Height: 320}, Size: Size{Width: 700, Height: 620}, Layout: VBox{}, Children: []Widget{minecraftBanner("UPDATE PROGRESS", "Worlds, settings and recovery backups are protected", true), Label{AssignTo: &v.stage, Text: "Preparing update...", MinSize: Size{Height: 48}}, ProgressBar{AssignTo: &v.bar, MaxValue: 100}, windowDPICustomWidget(CustomWidget{AssignTo: &v.steps, MinSize: Size{Height: 142}, MaxSize: Size{Height: 142}, PaintMode: PaintBuffered, Paint: func(c *walk.Canvas, r walk.Rectangle) error { return paintMinecraftSteps(c, r, v.stepState, v.failed) }}), TextLabel{Text: "The current phase and elapsed time update below. Backup copying may take several minutes."}, TextEdit{AssignTo: &v.log, ReadOnly: true, MinSize: Size{Height: 80}}, Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &v.closeButton, Text: "Close", Enabled: false, OnClicked: func() { v.dlg.Accept() }}}}}}
	footer := spec.Children[len(spec.Children)-1]
	content := spec.Children[:len(spec.Children)-1]
	spec.Children = []Widget{ScrollView{Layout: VBox{}, StretchFactor: 1, Children: content}, footer}
	if e := spec.Create(owner); e != nil {
		return nil, e
	}
	v.dlg.Starting().Attach(func() { fitProgressDialogToWorkArea(v.dlg) })
	v.dlg.Closing().Attach(func(c *bool, _ walk.CloseReason) {
		if !v.finished {
			*c = true
		}
	})
	_ = v.bar.SetMarqueeMode(true)
	v.stepState = updateProgressSteps("downloading", false, false)
	return v, nil
}
func (v *serverProgressDialog) renderUpdate(s serverupdate.Status, started time.Time, history string) {
	v.stage.SetText(fmt.Sprintf("%s\r\nElapsed: %s", updatePhaseTitle(s.Phase), time.Since(started).Round(time.Second)))
	v.failed = s.Done && s.Phase != "complete"
	if s.Phase != "failed" {
		v.stepState = updateProgressSteps(s.Phase, s.Done, v.failed)
	}
	v.steps.Invalidate()
	v.log.SetText(history + "\r\n" + s.Message + "\r\nBackup: " + s.Backup)
	if s.Done {
		v.finished = true
		_ = v.bar.SetMarqueeMode(false)
		if !v.failed {
			v.bar.SetValue(100)
		}
		v.closeButton.SetEnabled(true)
	}
}
func updatePhaseTitle(phase string) string {
	switch phase {
	case "downloading":
		return "Downloading and validating"
	case "stopping":
		return "Saving worlds and stopping servers"
	case "backup":
		return "Creating verified backup"
	case "installing":
		return "Installing server software"
	case "starting":
		return "Starting servers and checking readiness"
	case "complete":
		return "Update complete"
	case "failed":
		return "Update needs attention"
	}
	return "Checking update status"
}
func (p *updatesPage) runningProgress() {
	if p.progress != nil {
		return
	}
	view, e := newUpdateProgressDialog(p.a.mw)
	if e != nil {
		p.showError(e)
		return
	}
	p.progress = view
	p.busy = true
	go p.watch()
	view.dlg.Run()
	p.progress = nil
	view.dlg.Dispose()
}
