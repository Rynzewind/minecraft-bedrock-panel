package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"golang.org/x/sys/windows"
	"nethernet-multi-server/internal/config"
	"nethernet-multi-server/internal/migration"
	"nethernet-multi-server/internal/setup"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"time"
)

type setupWindow struct {
	mw                            *walk.MainWindow
	archive, destination, runtime *walk.LineEdit
	creativeIP, survivalIP        *walk.LineEdit
	adopt, reserved               *walk.CheckBox
	networkSummary                *walk.Label
	network, startup              *walk.ComboBox
	stopped                       *walk.CheckBox
	message                       *walk.TextEdit
	restore, activate, launch     *walk.PushButton
	interfaces                    []setup.LANInterface
	userSID                       string
	busy                          bool
	repairMode                    bool
}

func runSetup() {
	w := &setupWindow{}
	if e := w.create(); e != nil {
		walk.MsgBox(nil, "Minecraft migration setup", e.Error(), walk.MsgBoxIconError)
		return
	}
	w.mw.Run()
}
func (w *setupWindow) create() error {
	user, e := windows.GetCurrentProcessToken().GetTokenUser()
	if e != nil {
		return e
	}
	w.userSID = user.User.Sid.String()
	w.interfaces = setup.Interfaces()
	var networkNames []string
	for _, n := range w.interfaces {
		networkNames = append(networkNames, n.Name+" — "+n.CIDR)
	}
	m := MainWindow{AssignTo: &w.mw, Title: "Minecraft Server Migration Setup", Icon: 2, Size: Size{Width: 860, Height: 780}, MinSize: Size{Width: 780, Height: 650}, Layout: VBox{}, Children: []Widget{
		Label{Text: "Restore your Creative and Survival servers, worlds and admin panel on this Windows PC."},
		Label{Text: "Restore runs as your Windows user. Windows configuration asks for administrator access afterwards."},
		GroupBox{Title: "1. Restore a snapshot", Layout: VBox{}, Children: []Widget{
			Composite{Layout: HBox{}, Children: []Widget{Label{Text: "Snapshot ZIP:"}, LineEdit{AssignTo: &w.archive}, PushButton{Text: "Browse...", OnClicked: w.browseArchive}}},
			Composite{Layout: HBox{}, Children: []Widget{Label{Text: "New server folder:"}, LineEdit{AssignTo: &w.destination, Text: `C:\MinecraftServers`}, PushButton{Text: "Choose parent...", OnClicked: w.browseDestination}}},
			Label{Text: "Select the trusted LAN connection used by your consoles:"}, ComboBox{AssignTo: &w.network, Model: networkNames, CurrentIndex: 0, OnCurrentIndexChanged: w.selectNetwork}, Composite{Layout: HBox{}, Children: []Widget{Label{Text: "Creative static IP:"}, LineEdit{AssignTo: &w.creativeIP}, Label{Text: "Survival static IP:"}, LineEdit{AssignTo: &w.survivalIP}}},
			Label{AssignTo: &w.networkSummary, Text: "Select an active adapter. Server addresses are persistent secondary IPs; the primary PC address is retained."},
			CheckBox{AssignTo: &w.reserved, Text: "I have excluded/reserved these static server IPs in the router DHCP pool and reviewed possible address conflicts."},
			CheckBox{AssignTo: &w.adopt, Text: "Also remove these server IPs on uninstall if they already exist on this adapter (explicit ownership adoption)."},
			PushButton{AssignTo: &w.restore, Text: "Verify and restore snapshot", OnClicked: w.restoreSnapshot},
		}},
		GroupBox{Title: "2. Configure and activate (also use this to retry setup)", Layout: VBox{}, Children: []Widget{
			Label{Text: "For a retry, enter the already restored folder above. It will not be overwritten."},
			Composite{Layout: HBox{}, Children: []Widget{Label{Text: "Optional Microsoft VC++ x64 installer:"}, LineEdit{AssignTo: &w.runtime}, PushButton{Text: "Browse...", OnClicked: w.browseRuntime}, PushButton{Text: "Microsoft download page", OnClicked: func() { openURL("https://learn.microsoft.com/en-us/cpp/windows/latest-supported-vc-redist/") }}}},
			ComboBox{AssignTo: &w.startup, Model: []string{"Manual start only", "Start when I sign into Windows", "Start at Windows boot (requires Windows password)"}, CurrentIndex: 1},
			CheckBox{AssignTo: &w.stopped, Text: "The previous Minecraft deployment is stopped; this PC can use its server identities."},
			PushButton{AssignTo: &w.activate, Text: "Configure Windows and activate", OnClicked: w.activateInstall},
		}},
		TextEdit{AssignTo: &w.message, ReadOnly: true, MinSize: Size{Height: 100}, Text: "Copy your snapshot ZIP and this installer to the new PC. Use a fresh folder for restore. Microsoft account sign-in needs internet; restoring files works offline.\r\nIf the Visual C++ runtime is missing, obtain the official x64 installer from Microsoft before moving offline."},
		PushButton{AssignTo: &w.launch, Text: "Open restored admin panel", OnClicked: w.launchPanel},
	}}
	if e = m.Create(); e != nil {
		return e
	}
	w.selectNetwork()
	w.mw.Closing().Attach(func(cancelled *bool, reason walk.CloseReason) {
		if w.busy {
			walk.MsgBox(w.mw, "Setup is running", "Wait for this operation to finish before closing setup.", walk.MsgBoxIconInformation)
			*cancelled = true
		}
	})
	return nil
}
func openURL(url string) {
	c := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", url)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = c.Start()
}
func (w *setupWindow) browseArchive() {
	d := walk.FileDialog{Title: "Choose migration snapshot", Filter: "Migration snapshot (*.zip)|*.zip"}
	if ok, e := d.ShowOpen(w.mw); e == nil && ok {
		w.archive.SetText(d.FilePath)
	}
}
func (w *setupWindow) browseDestination() {
	d := walk.FileDialog{Title: "Choose parent folder for MinecraftServers"}
	if ok, e := d.ShowBrowseFolder(w.mw); e == nil && ok {
		w.destination.SetText(filepath.Join(d.FilePath, "MinecraftServers"))
	}
}
func (w *setupWindow) browseRuntime() {
	d := walk.FileDialog{Title: "Choose official Microsoft VC++ x64 redistributable", Filter: "Windows executable (*.exe)|*.exe"}
	if ok, e := d.ShowOpen(w.mw); e == nil && ok {
		w.runtime.SetText(d.FilePath)
	}
}
func (w *setupWindow) setBusy(b bool, text string) {
	w.busy = b
	w.restore.SetEnabled(!b)
	w.activate.SetEnabled(!b)
	w.launch.SetEnabled(!b)
	w.message.SetText(text)
}
func (w *setupWindow) restoreSnapshot() {
	index := w.network.CurrentIndex()
	if index < 0 || index >= len(w.interfaces) {
		w.message.SetText("Select an active LAN interface first.")
		return
	}
	n := w.interfaces[index]
	opts := migration.RestoreOptions{ArchivePath: strings.TrimSpace(w.archive.Text()), Destination: strings.TrimSpace(w.destination.Text()), LANAddress: n.Address, BroadcastAddress: n.Broadcast, ServerAddresses: map[string]string{"creative": strings.TrimSpace(w.creativeIP.Text()), "survival": strings.TrimSpace(w.survivalIP.Text())}}
	w.setBusy(true, "Verifying and restoring the snapshot. No server will start during restore...")
	go func() {
		r, e := migration.Restore(context.Background(), opts)
		if e == nil {
			e = setup.InstallTools(r.Root)
		}
		w.mw.Synchronize(func() {
			if e != nil {
				w.setBusy(false, "Restore failed: "+e.Error())
				return
			}
			w.setBusy(false, fmt.Sprintf("Restored to %s.\r\n%d Microsoft account(s) need reauthentication in the admin panel.\r\nLAN advertising is disabled until you complete configuration and confirm the old deployment has stopped.", r.Root, r.AccountsRequiringSignIn))
		})
	}()
}
func (w *setupWindow) activateInstall() {
	root := strings.TrimSpace(w.destination.Text())
	cfg, e := config.Load(filepath.Join(root, "MultiServer", "config.json"))
	if e != nil {
		w.message.SetText("Select a restored folder: " + e.Error())
		return
	}
	modes := []string{"none", "logon", "boot"}
	index := w.startup.CurrentIndex()
	if index < 0 {
		return
	}
	if !w.reserved.Checked() {
		w.message.SetText("Review the static server IPs and confirm the router DHCP reservation/exclusion first.")
		return
	}
	ni := w.network.CurrentIndex()
	if ni < 0 || ni >= len(w.interfaces) {
		w.message.SetText("Select an active network adapter.")
		return
	}
	networkPlan, e := setup.NetworkForInstallation(root, w.interfaces[ni], map[string]string{"creative": strings.TrimSpace(w.creativeIP.Text()), "survival": strings.TrimSpace(w.survivalIP.Text())}, w.adopt.Checked())
	if e != nil {
		w.message.SetText(e.Error())
		return
	}
	p, e := setup.BuildPlan(setup.Options{Root: root, UserSID: w.userSID, Startup: modes[index], RuntimeInstaller: strings.TrimSpace(w.runtime.Text()), OldDeploymentStopped: w.stopped.Checked(), Network: networkPlan}, cfg)
	if e != nil {
		w.message.SetText(e.Error())
		return
	}
	if walk.MsgBox(w.mw, "Configure Windows", fmt.Sprintf("Create missing static server IPs, synchronise server bindings, configure Private LAN firewall access and %s startup for:\r\n%s\r\n\r\nThe previous deployment must be stopped. This installation will stop cleanly during configuration. Windows will request administrator access.\r\nContinue?", modes[index], root+"\r\n"+networkPreview(networkPlan)), walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	w.setBusy(true, "Configuring Windows. Complete the administrator prompt and, for boot startup, the Windows password prompt...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		running, e := setup.PrepareForNetworkChange(ctx, root)
		if e == nil {
			e = setup.InstallTools(root)
		}
		if e == nil {
			e = setup.ElevateApply(ctx, p)
		}
		reportText := ""
		if e == nil {
			_ = os.Remove(filepath.Join(root, "network-maintenance"))
			if !w.repairMode {
				running = nil
			} else if running == nil {
				running = []string{}
			}
			e = setup.StartRuntime(ctx, root, running)
			if e == nil && (!w.repairMode || len(running) > 0) {
				for attempt := 0; attempt < 20; attempt++ {
					report, checkError := setup.RuntimeReport(ctx, root)
					if checkError != nil {
						e = checkError
						break
					}
					failed := false
					reportText = ""
					for _, check := range report {
						reportText += check.Status + ": " + check.Check + " — " + check.Detail + "\r\n"
						if check.Status == "failed" {
							failed = true
						}
					}
					if !failed {
						break
					}
					if attempt == 19 {
						e = fmt.Errorf("network endpoint validation did not pass:\n%s", reportText)
						break
					}
					time.Sleep(time.Second)
				}
			}
		}
		w.mw.Synchronize(func() {
			if e != nil {
				w.setBusy(false, "Configuration needs attention:\r\n"+e.Error()+"\r\nCorrect the issue and retry this step; do not restore over this folder.")
				return
			}
			w.setBusy(false, reportText+"\r\nStatic server addresses and configuration have been applied. Open the panel, reauthenticate each Microsoft account, then check Xbox joins to both worlds. The selected startup task will take effect at the next sign-in or boot.\r\nPreviously stopped servers remain stopped during repair; start them in the panel when ready.")
		})
	}()
}
func (w *setupWindow) launchPanel() {
	root := strings.TrimSpace(w.destination.Text())
	multi := filepath.Join(root, "MultiServer")
	admin := filepath.Join(multi, "NetherNetMultiServerAdmin.exe")
	if _, e := os.Stat(admin); e != nil {
		w.message.SetText("Restore the snapshot first.")
		return
	}
	w.setBusy(true, "Opening administration without starting stopped Minecraft servers...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		e := setup.StartRuntime(ctx, root, []string{})
		if e == nil {
			command := exec.Command(admin)
			command.Dir = multi
			e = command.Start()
		}
		w.mw.Synchronize(func() {
			if e != nil {
				w.setBusy(false, e.Error())
			} else {
				w.setBusy(false, "Administration opened. Stopped Minecraft servers were left stopped. Complete configuration before starting servers or advertising.")
			}
		})
	}()
}
