package main

import (
	"context"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"github.com/lxn/win"
	"nethernet-multi-server/internal/adminapi"
	"nethernet-multi-server/internal/contentpacks"
	"nethernet-multi-server/internal/online"
	"nethernet-multi-server/internal/serversettings"
	"nethernet-multi-server/internal/service"
	"nethernet-multi-server/internal/sessionhistory"
	"nethernet-multi-server/internal/supervisor"
)

type accountTableModel struct {
	walk.TableModelBase
	rows []AccountRow
}

func (m *accountTableModel) RowCount() int { return len(m.rows) }
func (m *accountTableModel) Value(row, col int) any {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case 0:
		return r.PersonLabel
	case 1:
		return r.EmailLabel
	case 2:
		return r.Gamertag
	case 3:
		return r.State
	case 4:
		return r.CurrentPlayer
	case 5:
		return r.Backend
	case 6:
		return r.Role
	}
	return ""
}
func (m *accountTableModel) set(rows []AccountRow) {
	m.rows = append([]AccountRow(nil), rows...)
	m.PublishRowsReset()
}

type playerTableModel struct {
	walk.TableModelBase
	rows []PlayerRow
}

func (m *playerTableModel) RowCount() int { return len(m.rows) }
func (m *playerTableModel) Value(row, col int) any {
	if row < 0 || row >= len(m.rows) {
		return ""
	}
	r := m.rows[row]
	switch col {
	case 0:
		return r.Gamertag
	case 1:
		return r.AccountDisplay
	case 2:
		return r.Role
	case 3:
		return r.State
	case 4:
		return r.LastSeen
	}
	return ""
}
func (m *playerTableModel) set(rows []PlayerRow) {
	m.rows = append([]PlayerRow(nil), rows...)
	m.PublishRowsReset()
}

type enrollmentView struct {
	EnrollmentID            string    `json:"enrollment_id"`
	UserCode                string    `json:"user_code"`
	VerificationURI         string    `json:"verification_uri"`
	VerificationURIComplete string    `json:"verification_uri_complete"`
	Expiry                  time.Time `json:"expiry"`
}
type windowSpec struct{ HasAccountPool, HasConsoleUsers, HasStartStopControl bool }

func mainWindowSpec() windowSpec { return windowSpec{HasAccountPool: true, HasConsoleUsers: true} }

type adminWindow struct {
	online                                                             *onlinePage
	trayCreate                                                         func(walk.Form) (*walk.NotifyIcon, error)
	trayFailures                                                       int
	trayPrevious                                                       uintptr
	fleetBusy                                                          atomic.Bool
	fleetState                                                         *fleetRuntime
	tray                                                               *walk.NotifyIcon
	trayActions                                                        []*walk.Action
	quitting                                                           bool
	startHidden                                                        bool
	updates                                                            *updatesPage
	migration                                                          *migrationPage
	mw                                                                 *walk.MainWindow
	status, serverSummary                                              *walk.Label
	accounts, players                                                  *walk.TableView
	broadcasters                                                       *walk.TableView
	broadcasterModel                                                   accountTableModel
	helperToggle, broadcasterToggle                                    *walk.PushButton
	accountPool, consoleUsers                                          *walk.GroupBox
	servers, packs, sessions                                           *walk.TableView
	navigation                                                         *walk.TabWidget
	viewport                                                           *walk.ScrollView
	events                                                             *walk.TextEdit
	serverLog                                                          *walk.TextEdit
	add, edit, reauth, remove, reassign, unpair, setRole, onlineAccess *walk.PushButton
	start, stop, restart                                               *walk.PushButton
	startAll, stopAll, restartAll                                      *walk.PushButton
	serverSettingsButton                                               *walk.PushButton
	uninstallPackButton                                                *walk.PushButton
	applyCreative, applySurvival                                       *walk.PushButton
	cancelCreative, cancelSurvival                                     *walk.PushButton
	accountModel                                                       accountTableModel
	playerModel                                                        playerTableModel
	serverModel                                                        serverTableModel
	packModel                                                          packTableModel
	sessionModel                                                       sessionTableModel
	client                                                             pipeClient
	state                                                              ViewState
	closed                                                             atomic.Bool
	refreshSeq                                                         atomic.Uint64
	eventMu                                                            sync.Mutex
	lastError                                                          string
}

func runAdmin() {
	startup := false
	for _, arg := range os.Args[1:] {
		if arg == "--startup" {
			startup = true
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	instance, first, err := adminInstance(exe, startup)
	if err != nil {
		walk.MsgBox(nil, "NetherNet", err.Error(), walk.MsgBoxIconError)
		return
	}
	if !first {
		return
	}
	defer windows.CloseHandle(instance)
	a := &adminWindow{startHidden: startup}
	if err := a.create(); err != nil {
		walk.MsgBox(nil, "NetherNet Multi-Server Admin", err.Error(), walk.MsgBoxIconError)
		return
	}
	defer a.mw.Dispose()
	if err := a.initFleet(); err != nil {
		walk.MsgBox(a.mw, "NetherNet", err.Error(), walk.MsgBoxIconError)
		return
	}
	a.startTrayRecovery()
	defer a.disposeTray()
	a.mw.Starting().Attach(func() { a.fleetAction("start-all", a.startHidden) })
	a.refresh()
	a.updates.readStatus()
	a.updates.loadSources()
	go a.migration.resume()
	go a.migration.pollScheduleInfo()
	go a.poll()
	go a.startRecovery()
	a.mw.Starting().Attach(func() {
		if !a.startHidden {
			win.ShowWindow(a.mw.Handle(), win.SW_MAXIMIZE)
		}
	})
	a.mw.Run()
	a.closed.Store(true)
}
func (a *adminWindow) create() error {
	accountPool := GroupBox{AssignTo: &a.accountPool, Title: "Account Pool", StretchFactor: 1, Layout: VBox{}, Children: []Widget{TableView{AssignTo: &a.accounts, Model: &a.accountModel, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Person", Width: 120}, {Title: "Email label", Width: 180}, {Title: "Xbox gamertag", Width: 135}, {Title: "Status", Width: 85}, {Title: "Assigned / active user", Width: 135}, {Title: "Server", Width: 75}, {Title: "Account role", Width: 120}}, OnCurrentIndexChanged: func() { a.accountSelectionChanged(false) }}, Composite{Layout: HBox{}, Children: []Widget{PushButton{AssignTo: &a.add, Text: "Add account", OnClicked: a.addAccount}, PushButton{Text: "Add broadcaster", OnClicked: a.addBroadcasterAccount}, PushButton{Text: "Account role...", OnClicked: a.setAccountRole}, PushButton{AssignTo: &a.onlineAccess, Text: "Online access...", ToolTipText: "Grant selected account access to Creative and/or Survival Friends connections", OnClicked: a.grantSelectedAccountOnlineAccess}, PushButton{AssignTo: &a.edit, Text: "Edit labels", OnClicked: a.editLabels}, PushButton{AssignTo: &a.reauth, Text: "Reauthenticate", OnClicked: a.reauthenticate}, PushButton{AssignTo: &a.remove, Text: "Remove", OnClicked: a.removeAccount}}}}}
	consoleUsers := GroupBox{AssignTo: &a.consoleUsers, Title: "Console Users", StretchFactor: 1, Layout: VBox{}, Children: []Widget{TableView{AssignTo: &a.players, Model: &a.playerModel, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Console gamertag", Width: 130}, {Title: "Assigned account", Width: 220}, {Title: "Role (all servers)", Width: 110}, {Title: "Status", Width: 80}, {Title: "Last seen", Width: 125}}, OnCurrentIndexChanged: a.updateActions}, Composite{Layout: HBox{}, Children: []Widget{PushButton{AssignTo: &a.reassign, Text: "Assign / Reassign", OnClicked: a.reassignPlayer}, PushButton{AssignTo: &a.unpair, Text: "Unpair", OnClicked: a.unpairPlayer}, PushButton{AssignTo: &a.setRole, Text: "Set role...", ToolTipText: "Set the selected console user role on all servers", OnClicked: a.setPlayerRole}}}}}
	// Avoid nested default padding around account actions.
	accountPool.Layout = VBox{Margins: Margins{Left: 6, Top: 6, Right: 6, Bottom: 6}, Spacing: 6}
	consoleUsers.Layout = VBox{Margins: Margins{Left: 6, Top: 6, Right: 8, Bottom: 6}, Spacing: 6}
	for _, group := range []*GroupBox{&accountPool, &consoleUsers} {
		bar := group.Children[1].(Composite)
		bar.Layout = VBox{MarginsZero: true, Spacing: 6}
		var rows []Widget
		for start := 0; start < len(bar.Children); start += 2 {
			end := min(start+2, len(bar.Children))
			rows = append(rows, Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: bar.Children[start:end]})
		}
		bar.Children = rows
		group.Children[1] = bar
	}
	// Keep account actions visible above the lists at compact sizes.
	accountPool.Children[0], accountPool.Children[1] = accountPool.Children[1], accountPool.Children[0]
	consoleUsers.Children[0], consoleUsers.Children[1] = consoleUsers.Children[1], consoleUsers.Children[0]
	helperTable := accountPool.Children[1].(TableView)
	broadcasterTable := helperTable
	broadcasterTable.AssignTo = &a.broadcasters
	broadcasterTable.Model = &a.broadcasterModel
	broadcasterTable.Visible = false
	broadcasterTable.OnCurrentIndexChanged = func() { a.accountSelectionChanged(true) }
	accountPool.Children = []Widget{accountPool.Children[0],
		PushButton{AssignTo: &a.helperToggle, Text: "Helpers (expanded)", OnClicked: func() { a.toggleAccountSection(false) }}, helperTable,
		PushButton{AssignTo: &a.broadcasterToggle, Text: "Broadcasters (collapsed)", OnClicked: func() { a.toggleAccountSection(true) }}, broadcasterTable, VSpacer{}}
	consoleUsers.Children = append(consoleUsers.Children, VSpacer{})
	accountsPage := TabPage{Title: "Accounts", Layout: VBox{}, Children: []Widget{HSplitter{Children: []Widget{accountPool, consoleUsers}}}}
	serversPage := TabPage{Title: "Servers", Layout: VBox{}, Children: []Widget{TableView{MinSize: Size{Height: 120}, AssignTo: &a.servers, Model: &a.serverModel, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Server", Width: 260}, {Title: "State", Width: 120}, {Title: "Players", Width: 70}, {Title: "PID", Width: 90}, {Title: "Changes", Width: 130}}, OnCurrentIndexChanged: a.updateManagementActions}, Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: []Widget{PushButton{AssignTo: &a.start, Text: "Start", Background: SolidColorBrush{Color: walk.RGB(120, 200, 120)}, OnClicked: func() { a.serverAction("server.start") }}, PushButton{AssignTo: &a.stop, Text: "Stop", Background: SolidColorBrush{Color: walk.RGB(220, 120, 120)}, OnClicked: func() { a.serverAction("server.stop") }}, PushButton{AssignTo: &a.restart, Text: "Restart", Background: SolidColorBrush{Color: walk.RGB(235, 190, 90)}, OnClicked: func() { a.serverAction("server.restart") }}, PushButton{AssignTo: &a.serverSettingsButton, Text: "Settings...", ToolTipText: "Server Settings", OnClicked: a.openServerSettings}}}, Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: []Widget{PushButton{AssignTo: &a.startAll, Text: "Start All Servers", OnClicked: func() { a.allServerAction("server.start") }}, PushButton{AssignTo: &a.stopAll, Text: "Stop All Servers", OnClicked: func() { a.allServerAction("server.stop") }}, PushButton{AssignTo: &a.restartAll, Text: "Restart All Servers", OnClicked: func() { a.allServerAction("server.restart") }}}}, TextEdit{AssignTo: &a.serverLog, ReadOnly: true, MinSize: Size{Height: 32}, MaxSize: Size{Height: 48}}}}
	packsPage := TabPage{Title: "Content Packs", Layout: VBox{}, Children: []Widget{
		TextLabel{Text: "Import add-ons downloaded separately, then enable them for either server. Changes are safely staged until Apply or Restart."},
		TableView{AssignTo: &a.packs, Model: &a.packModel, MultiSelection: true, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Pack", Width: 190}, {Title: "Type", Width: 75}, {Title: "Version", Width: 75}, {Title: "UUID", Width: 245}, {Title: "Creative", Width: 150}, {Title: "Survival", Width: 150}}, OnCurrentIndexChanged: a.updateManagementActions, OnSelectedIndexesChanged: a.updateManagementActions},
		Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Import add-ons...", OnClicked: a.importPack}, PushButton{AssignTo: &a.uninstallPackButton, Text: "Uninstall selected", OnClicked: a.uninstallPack}, PushButton{Text: "Enable Creative", OnClicked: func() { a.stagePack("creative", true) }}, PushButton{Text: "Disable Creative", OnClicked: func() { a.stagePack("creative", false) }}, PushButton{Text: "Enable Survival", OnClicked: func() { a.stagePack("survival", true) }}, PushButton{Text: "Disable Survival", OnClicked: func() { a.stagePack("survival", false) }}}},
		Composite{Layout: HBox{}, Children: []Widget{PushButton{Text: "Require packs: Creative", OnClicked: func() { a.stagePolicy("creative", true) }}, PushButton{Text: "Optional packs: Creative", OnClicked: func() { a.stagePolicy("creative", false) }}, PushButton{Text: "Require packs: Survival", OnClicked: func() { a.stagePolicy("survival", true) }}, PushButton{Text: "Optional packs: Survival", OnClicked: func() { a.stagePolicy("survival", false) }}}},
		Composite{Layout: HBox{}, Children: []Widget{PushButton{AssignTo: &a.applyCreative, Text: "Apply Creative now", OnClicked: func() { a.applyPacks("creative") }}, PushButton{AssignTo: &a.cancelCreative, Text: "Cancel Creative pending", OnClicked: func() { a.cancelPending("creative") }}, PushButton{AssignTo: &a.applySurvival, Text: "Apply Survival now", OnClicked: func() { a.applyPacks("survival") }}, PushButton{AssignTo: &a.cancelSurvival, Text: "Cancel Survival pending", OnClicked: func() { a.cancelPending("survival") }}, Label{Text: "Running servers apply changes at their next restart."}}},
	}}
	// Long action bars get compact rows; page scrollviews handle short displays.
	for _, index := range []int{2, 3, 4} {
		bar := packsPage.Children[index].(Composite)
		bar.Layout = VBox{MarginsZero: true, Spacing: 6}
		var rows []Widget
		limit := 3
		if index != 2 {
			limit = 2
		}
		for start := 0; start < len(bar.Children); start += limit {
			end := min(start+limit, len(bar.Children))
			children := bar.Children[start:end]
			if len(children) == 1 {
				if label, ok := children[0].(Label); ok {
					rows = append(rows, TextLabel{Text: label.Text})
					continue
				}
			}
			rows = append(rows, Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, Children: children})
		}
		bar.Children = rows
		packsPage.Children[index] = bar
	}
	sessionsPage := TabPage{Title: "Player History", Layout: VBox{}, Children: []Widget{
		TextLabel{Text: "Persistent join and leave history. SIMULTANEOUS DROP marks different players disconnecting within five seconds."},
		TableView{AssignTo: &a.sessions, Model: &a.sessionModel, AlternatingRowBG: true, LastColumnStretched: true, Columns: []TableViewColumn{{Title: "Time", Width: 170}, {Title: "Event", Width: 90}, {Title: "Console player", Width: 140}, {Title: "Assigned account", Width: 190}, {Title: "Server", Width: 170}, {Title: "Duration", Width: 80}, {Title: "Reason", Width: 250}, {Title: "Alert", Width: 160}}},
	}}
	w := MainWindow{Visible: !a.startHidden, AssignTo: &a.mw, Title: "NetherNet Multi-Server Administrator", Icon: 2, MinSize: Size{Width: 640, Height: 420}, Size: Size{Width: 1280, Height: 760}, Layout: VBox{}, Children: []Widget{minecraftBanner("NETHERNET", "Minecraft Server Administrator", false), Composite{Layout: HBox{}, Children: []Widget{Label{AssignTo: &a.status, Text: "Connecting to server..."}, HSpacer{}, Label{AssignTo: &a.serverSummary, Text: "Creative: 0  |  Survival: 0  |  Active: 0"}, PushButton{Text: "Refresh", OnClicked: a.refresh}}}, TabWidget{AssignTo: &a.navigation, StretchFactor: 1, Pages: []TabPage{scrollableAdminPage(serversPage), scrollableAdminPage(packsPage), scrollableAdminPage(accountsPage, &a.viewport), scrollableAdminPage(sessionsPage), scrollableAdminPage(a.migrationTab()), scrollableAdminPage(a.updatesTab()), scrollableAdminPage(a.onlineTab())}}, GroupBox{Title: "Activity", MinSize: Size{Height: 160}, MaxSize: Size{Height: 160}, Layout: VBox{}, Children: []Widget{TextEdit{AssignTo: &a.events, ReadOnly: true, MinSize: Size{Height: 100}, Text: "Account passwords and authentication tokens are never shown here.\r\n"}}}}}
	if err := w.Create(); err != nil {
		return err
	}
	applyMinecraftTheme(a.mw)
	a.navigation.CurrentIndexChanged().Attach(func() {
		if a.navigation.CurrentIndex() == 6 && a.online != nil {
			a.online.load()
		}
	})
	installContentSizedAccountList(a.accounts, a.accountPool, &a.accountModel)
	installContentSizedAccountList(a.broadcasters, a.accountPool, &a.broadcasterModel)
	installContentSizedAccountList(a.players, a.consoleUsers, &a.playerModel)
	a.mw.Starting().Attach(func() {
		fitAdministratorToWorkArea(a.mw)
	})
	installAdaptiveServerRows(a.servers, &a.serverModel)
	a.updateActions()
	return nil
}
func (a *adminWindow) poll() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if a.closed.Load() {
			return
		}
		a.refresh()
	}
}
func (a *adminWindow) refresh() {
	seq := a.refreshSeq.Add(1)
	go func() {
		ctx, cancel := timeoutContext()
		defer cancel()
		var status service.Status
		err := a.client.Call(ctx, "status.list", struct{}{}, &status)
		var serverStatuses []supervisor.Status
		var storedPacks []contentpacks.InventoryItem
		var sessionEvents []sessionhistory.Event
		var onlineStatuses []online.Status
		if err == nil {
			err = a.client.Call(ctx, "servers.list", struct{}{}, &serverStatuses)
			_ = a.client.Call(ctx, "packs.list", struct{}{}, &storedPacks)
			_ = a.client.Call(ctx, "sessions.list", map[string]int{"limit": 1000}, &sessionEvents)
			_ = a.client.Call(ctx, "online.status", struct{}{}, &onlineStatuses)
		}
		if a.closed.Load() || seq != a.refreshSeq.Load() {
			return
		}
		a.sync(func() {
			if err != nil {
				a.applyDisconnected(err)
				return
			}
			selectedAccountID, selectedPlayerXUID, selectedServerID, selectedPackUUIDs := a.selectedStableIDs()
			a.lastError = ""
			a.state = BuildViewState(status)
			a.status.SetText(a.state.StatusText)
			a.serverSummary.SetText(fmt.Sprintf("Creative: %d  |  Survival: %d  |  Active: %d", a.state.CreativeActive, a.state.SurvivalActive, a.state.ActiveCount))
			helpers, broadcasters := splitAccountRows(a.state.Accounts)
			a.accountModel.set(helpers)
			a.broadcasterModel.set(broadcasters)
			a.playerModel.set(a.state.Players)
			a.serverModel.set(BuildServerRows(serverStatuses))
			a.packModel.set(BuildPackRows(storedPacks))
			if a.updates != nil && a.updates.addonTable != nil {
				selected := a.updates.selectedAddon()
				a.updates.packs = storedPacks
				a.updates.renderAddonOverview(selected)
			}
			a.sessionModel.set(BuildSessionRows(sessionEvents))
			if a.online != nil {
				a.online.applyStatuses(onlineStatuses)
			}
			_ = a.accounts.SetCurrentIndex(accountRowIndex(a.accountModel.rows, selectedAccountID))
			_ = a.broadcasters.SetCurrentIndex(accountRowIndex(a.broadcasterModel.rows, selectedAccountID))
			_ = a.players.SetCurrentIndex(playerRowIndex(a.playerModel.rows, selectedPlayerXUID))
			_ = a.servers.SetCurrentIndex(serverRowIndex(a.serverModel.rows, selectedServerID))
			_ = a.packs.SetSelectedIndexes(packIndexesByUUID(a.packModel.rows, selectedPackUUIDs))
			a.updateActions()
			a.updateManagementActions()
		})
	}()
}

func (a *adminWindow) selectedStableIDs() (accountID, playerXUID, serverID string, packUUIDs []string) {
	if row, ok := a.accountRow(); ok {
		accountID = row.ID
	}
	if i := a.players.CurrentIndex(); i >= 0 && i < len(a.playerModel.rows) {
		playerXUID = a.playerModel.rows[i].XUID
	}
	if i := a.servers.CurrentIndex(); i >= 0 && i < len(a.serverModel.rows) {
		serverID = a.serverModel.rows[i].ID
	}
	packUUIDs = selectedPackUUIDs(a.packModel.rows, a.packs.SelectedIndexes())
	return
}

func (a *adminWindow) selectedServer() (ServerRow, bool) {
	i := a.servers.CurrentIndex()
	if i < 0 || i >= len(a.serverModel.rows) {
		return ServerRow{}, false
	}
	return a.serverModel.rows[i], true
}
func (a *adminWindow) selectedPack() (PackRow, bool) {
	indexes := a.packs.SelectedIndexes()
	if len(indexes) != 1 || indexes[0] < 0 || indexes[0] >= len(a.packModel.rows) {
		return PackRow{}, false
	}
	return a.packModel.rows[indexes[0]], true
}

func (a *adminWindow) selectedPacks() []PackRow {
	indexes := a.packs.SelectedIndexes()
	rows := make([]PackRow, 0, len(indexes))
	for _, index := range indexes {
		if index >= 0 && index < len(a.packModel.rows) {
			rows = append(rows, a.packModel.rows[index])
		}
	}
	return rows
}
func (a *adminWindow) updateManagementActions() {
	if a.start == nil {
		return
	}
	for _, button := range []*walk.PushButton{a.startAll, a.stopAll, a.restartAll} {
		if button != nil {
			button.SetEnabled(!a.fleetBusy.Load())
		}
	}
	for _, action := range a.trayActions {
		action.SetEnabled(!a.fleetBusy.Load() && win.IsWindowEnabled(a.mw.Handle()))
	}
	row, ok := a.selectedServer()
	a.start.SetEnabled(ok && row.CanStart)
	a.stop.SetEnabled(ok && row.CanStop)
	a.restart.SetEnabled(ok && row.CanRestart)
	a.serverSettingsButton.SetEnabled(ok && row.State != string(supervisor.Unconfigured))
	packSelected := len(a.selectedPacks()) > 0
	if a.uninstallPackButton != nil {
		a.uninstallPackButton.SetEnabled(packSelected)
	}
	creative, creativeOK := a.serverByID("creative")
	survival, survivalOK := a.serverByID("survival")
	if a.applyCreative != nil {
		a.applyCreative.SetEnabled(creativeOK && creative.State == string(supervisor.Stopped) && creative.Pending != "")
		a.cancelCreative.SetEnabled(creativeOK && creative.Pending != "")
		a.applySurvival.SetEnabled(survivalOK && survival.State == string(supervisor.Stopped) && survival.Pending != "")
		a.cancelSurvival.SetEnabled(survivalOK && survival.Pending != "")
	}
	if ok {
		go func(id string) {
			ctx, cancel := timeoutContext()
			defer cancel()
			var logText string
			if a.client.Call(ctx, "server.logs", map[string]string{"server_id": id}, &logText) == nil {
				a.sync(func() { a.serverLog.SetText(logText) })
			}
		}(row.ID)
	}
}
func (a *adminWindow) serverByID(id string) (ServerRow, bool) {
	for _, row := range a.serverModel.rows {
		if row.ID == id {
			return row, true
		}
	}
	return ServerRow{}, false
}
func (a *adminWindow) serverAction(method string) {
	row, ok := a.selectedServer()
	if !ok {
		return
	}
	if row.NeedsDisconnectConfirmation && (method == "server.stop" || method == "server.restart") {
		if walk.MsgBox(a.mw, "Confirm server action", fmt.Sprintf("%s has %d connected player(s). Continue?", row.Name, row.Players), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
			return
		}
	}
	a.runServerOperation(row, method, func(ctx context.Context) error {
		return a.client.Call(ctx, method, map[string]string{"server_id": row.ID}, nil)
	})
}

func (a *adminWindow) openServerSettings() {
	row, ok := a.selectedServer()
	if !ok {
		return
	}
	ctx, cancel := timeoutContext()
	defer cancel()
	var settings serversettings.Settings
	if err := a.client.Call(ctx, "server.settings.get", map[string]string{"server_id": row.ID}, &settings); err != nil {
		a.appendEvent("Unable to load server settings: " + err.Error())
		walk.MsgBox(a.mw, "Server Settings", "Unable to load the server settings. Check the Activity panel for details.", walk.MsgBoxIconError)
		return
	}
	updated, restart, accepted, err := serverSettingsDialog(a.mw, row.Name, settings, row.CanRestart)
	if err != nil {
		a.appendEvent("Unable to open server settings: " + err.Error())
		walk.MsgBox(a.mw, "Server Settings", "Unable to open the settings window:\r\n\r\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	if !accepted {
		return
	}
	if restart && row.Players > 0 && walk.MsgBox(a.mw, "Confirm server restart", fmt.Sprintf("%s has %d connected player(s). Save settings and restart anyway?", row.Name, row.Players), walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	method := "settings.save"
	if restart {
		method = "server.restart"
	}
	a.runServerOperation(row, method, func(ctx context.Context) error {
		err := a.client.Call(ctx, "server.settings.save", struct {
			ServerID string                  `json:"server_id"`
			Settings serversettings.Settings `json:"settings"`
		}{row.ID, updated}, nil)
		if err == nil && restart {
			err = a.client.Call(ctx, "server.restart", map[string]string{"server_id": row.ID}, nil)
		}
		return err
	})
}

func serverSettingsDialog(owner walk.Form, serverName string, current serversettings.Settings, canRestart bool) (serversettings.Settings, bool, bool, error) {
	var dlg *walk.Dialog
	var gameMode, difficulty, permission *walk.ComboBox
	var cheats, forceMode, allowList, pvp, clientChunks *walk.CheckBox
	var maxPlayers, viewDistance, tickDistance, idleTimeout *walk.LineEdit
	accepted, restart := false, false
	gameModes := []string{"survival", "creative", "adventure"}
	difficulties := []string{"peaceful", "easy", "normal", "hard"}
	permissions := []string{"visitor", "member", "operator"}
	accept := func(wantRestart bool) {
		updated, parseErr := applyServerSettingNumbers(current, maxPlayers.Text(), viewDistance.Text(), tickDistance.Text(), idleTimeout.Text())
		if parseErr != nil {
			walk.MsgBox(dlg, "Server Settings", parseErr.Error(), walk.MsgBoxIconWarning)
			return
		}
		// Capture every control while the dialog is still alive. Reading these
		// widgets after dlg.Accept destroys the native controls and can return
		// their original/default values instead of the user's selections.
		updated.GameMode = gameModes[gameMode.CurrentIndex()]
		updated.Difficulty = difficulties[difficulty.CurrentIndex()]
		updated.DefaultPermission = permissions[permission.CurrentIndex()]
		updated.AllowCheats = cheats.Checked()
		updated.ForceGameMode = forceMode.Checked()
		updated.AllowList = allowList.Checked()
		updated.PlayerVsPlayer = pvp.Checked()
		updated.ClientSideChunkGeneration = clientChunks.Checked()
		current = updated
		accepted, restart = true, wantRestart
		dlg.Accept()
	}
	_, err := (themedDialog{AssignTo: &dlg, Title: "Server Settings - " + serverName, MinSize: Size{Width: 520, Height: 520}, Layout: Grid{Columns: 2}, Children: []Widget{
		Label{Text: "Game mode:"}, ComboBox{AssignTo: &gameMode, Model: gameModes, CurrentIndex: stringIndex(gameModes, current.GameMode)},
		Label{Text: "Difficulty:"}, ComboBox{AssignTo: &difficulty, Model: difficulties, CurrentIndex: stringIndex(difficulties, current.Difficulty)},
		Label{Text: "Maximum players (1-1000):"}, LineEdit{AssignTo: &maxPlayers, Text: strconv.Itoa(current.MaxPlayers)},
		Label{Text: "View distance in chunks (5-96):"}, LineEdit{AssignTo: &viewDistance, Text: strconv.Itoa(current.ViewDistance)},
		Label{Text: "Simulation distance in chunks (4-12):"}, LineEdit{AssignTo: &tickDistance, Text: strconv.Itoa(current.TickDistance)},
		Label{Text: "Idle timeout in minutes (0-10080):"}, LineEdit{AssignTo: &idleTimeout, Text: strconv.Itoa(current.PlayerIdleTimeout)},
		Label{Text: "Default player permission:"}, ComboBox{AssignTo: &permission, Model: permissions, CurrentIndex: stringIndex(permissions, current.DefaultPermission)},
		CheckBox{AssignTo: &cheats, Text: "Allow cheats", Checked: current.AllowCheats, ColumnSpan: 2},
		CheckBox{AssignTo: &forceMode, Text: "Force selected game mode", Checked: current.ForceGameMode, ColumnSpan: 2},
		CheckBox{AssignTo: &allowList, Text: "Require allow-list", Checked: current.AllowList, ColumnSpan: 2},
		CheckBox{AssignTo: &pvp, Text: "Allow player-vs-player combat", Checked: current.PlayerVsPlayer, ColumnSpan: 2},
		CheckBox{AssignTo: &clientChunks, Text: "Allow client-side chunk generation", Checked: current.ClientSideChunkGeneration, ColumnSpan: 2},
		Label{Text: "Changes take effect after the next server restart.", ColumnSpan: 2},
		Composite{ColumnSpan: 2, Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{Text: "Save Only", OnClicked: func() { accept(false) }}, PushButton{Text: "Save and Restart", Enabled: canRestart, OnClicked: func() { accept(true) }}, PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}},
	}}).Run(owner)
	if err != nil {
		return current, false, false, err
	}
	if !accepted {
		return current, false, false, nil
	}
	return current, restart, true, nil
}

func applyServerSettingNumbers(current serversettings.Settings, maxPlayers, viewDistance, tickDistance, idleTimeout string) (serversettings.Settings, error) {
	values := []struct {
		label    string
		text     string
		min, max int
		target   *int
	}{
		{"Maximum players", maxPlayers, 1, 1000, &current.MaxPlayers},
		{"View distance", viewDistance, 5, 96, &current.ViewDistance},
		{"Simulation distance", tickDistance, 4, 12, &current.TickDistance},
		{"Idle timeout", idleTimeout, 0, 10080, &current.PlayerIdleTimeout},
	}
	for _, value := range values {
		n, err := strconv.Atoi(strings.TrimSpace(value.text))
		if err != nil || n < value.min || n > value.max {
			return current, fmt.Errorf("%s must be a whole number from %d to %d.", value.label, value.min, value.max)
		}
		*value.target = n
	}
	return current, nil
}

func stringIndex(values []string, value string) int {
	for i, v := range values {
		if v == value {
			return i
		}
	}
	return 0
}
func (a *adminWindow) importPack() {
	dlg := &walk.FileDialog{Title: "Import Bedrock add-ons", Filter: "Bedrock add-ons (*.mcaddon;*.mcpack;*.zip)|*.mcaddon;*.mcpack;*.zip"}
	ok, err := dlg.ShowOpenMultiple(a.mw)
	if err != nil {
		a.appendEvent(err.Error())
		return
	}
	if !ok {
		return
	}
	paths := append([]string(nil), dlg.FilePaths...)
	go func() {
		importedFiles, importedPacks := 0, 0
		remaining := append([]string(nil), paths...)
		failureByPath := map[string]error{}
		// Retry unsuccessful files after each successful pass. This allows a
		// behavior pack selected before its companion resource pack to resolve
		// the dependency without requiring the user to control file order.
		for len(remaining) > 0 {
			var retry []string
			progress := false
			for _, path := range remaining {
				ctx, cancel := timeoutContext()
				var imported []contentpacks.StoredPack
				err := a.client.Call(ctx, "pack.import", map[string]string{"path": path}, &imported)
				cancel()
				if err != nil {
					failureByPath[path] = err
					retry = append(retry, path)
					continue
				}
				delete(failureByPath, path)
				importedFiles++
				importedPacks += len(imported)
				progress = true
			}
			remaining = retry
			if !progress {
				break
			}
		}
		var failures []string
		for _, path := range paths {
			if err := failureByPath[path]; err != nil {
				failures = append(failures, filepath.Base(path)+": "+err.Error())
			}
		}
		a.sync(func() {
			message := fmt.Sprintf("Imported %d of %d file(s), containing %d pack(s).", importedFiles, len(paths), importedPacks)
			if len(failures) > 0 {
				message += "\r\n\r\nFailed:\r\n" + strings.Join(failures, "\r\n")
			}
			a.appendEvent(message)
			icon := walk.MsgBoxIconInformation
			if len(failures) > 0 {
				icon = walk.MsgBoxIconWarning
			}
			walk.MsgBox(a.mw, "Add-on import complete", message, icon)
			a.refresh()
		})
	}()
}
func (a *adminWindow) stagePack(serverID string, enable bool) {
	rows := a.selectedPacks()
	if len(rows) == 0 {
		return
	}
	uuids := make([]string, 0, len(rows))
	for _, row := range rows {
		uuids = append(uuids, row.UUID)
	}
	a.call("pack.stage_many", struct {
		ServerID  string   `json:"server_id"`
		PackUUIDs []string `json:"pack_uuids"`
		Enable    bool     `json:"enable"`
	}{serverID, uuids, enable}, fmt.Sprintf("%d pack change(s) staged for %s", len(rows), serverID))
}
func (a *adminWindow) applyPacks(serverID string) {
	row, ok := a.serverByID(serverID)
	if !ok {
		return
	}
	a.runServerOperation(row, "packs.apply", func(ctx context.Context) error {
		return a.client.Call(ctx, "packs.apply", map[string]string{"server_id": serverID}, nil)
	})
}
func (a *adminWindow) cancelPending(serverID string) {
	a.call("packs.cancel", map[string]string{"server_id": serverID}, "Pending pack changes cancelled for "+serverID)
}
func (a *adminWindow) stagePolicy(serverID string, required bool) {
	a.call("pack.policy", struct {
		ServerID string `json:"server_id"`
		Required bool   `json:"required"`
	}{serverID, required}, "Resource-pack policy staged for "+serverID)
}
func (a *adminWindow) uninstallPack() {
	rows := a.selectedPacks()
	if len(rows) == 0 {
		return
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UUID)
	}
	params := struct {
		PackUUIDs []string `json:"pack_uuids"`
	}{ids}
	ctx, cancel := timeoutContext()
	defer cancel()
	var plan []contentpacks.StoredPack
	if err := a.client.Call(ctx, "pack.uninstall.preview", params, &plan); err != nil {
		a.appendEvent(err.Error())
		walk.MsgBox(a.mw, "Cannot uninstall add-ons", err.Error(), walk.MsgBoxIconError)
		return
	}
	names := make([]string, 0, len(plan))
	for _, pack := range plan {
		names = append(names, "• "+pack.Name)
	}
	message := fmt.Sprintf("Remove these %d linked pack(s) from the imported library?\r\n\r\n%s\r\n\r\nWorld data is not deleted, and a rollback copy will be retained.", len(plan), strings.Join(names, "\r\n"))
	if walk.MsgBox(a.mw, "Uninstall add-ons", message, walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
		return
	}
	a.call("pack.uninstall_many", params, fmt.Sprintf("%d linked pack(s) uninstalled", len(plan)))
}
func (a *adminWindow) applyDisconnected(err error) {
	a.state = ViewState{}
	a.status.SetText("Server administrator connection unavailable - server may be stopped or running an older build")
	a.serverSummary.SetText("Creative: -  |  Survival: -  |  Active: -")
	a.accountModel.set(nil)
	a.playerModel.set(nil)
	message := err.Error()
	if message != a.lastError {
		a.lastError = message
		a.appendEvent("Disconnected: " + message)
	}
	a.updateActions()
}
func (a *adminWindow) sync(fn func()) {
	if a.closed.Load() || a.mw == nil || a.mw.IsDisposed() {
		return
	}
	a.mw.Synchronize(func() {
		if !a.closed.Load() && !a.mw.IsDisposed() {
			fn()
		}
	})
}
func (a *adminWindow) appendEvent(message string) {
	a.eventMu.Lock()
	defer a.eventMu.Unlock()
	if a.events != nil && !a.events.IsDisposed() {
		a.events.AppendText(time.Now().Format("15:04:05") + "  " + message + "\r\n")
	}
}
func (a *adminWindow) accountRow() (AccountRow, bool) {
	if a.broadcasters != nil {
		i := a.broadcasters.CurrentIndex()
		if i >= 0 && i < len(a.broadcasterModel.rows) {
			return a.broadcasterModel.rows[i], true
		}
	}
	i := a.accounts.CurrentIndex()
	if i < 0 || i >= len(a.accountModel.rows) {
		return AccountRow{}, false
	}
	return a.accountModel.rows[i], true
}
func (a *adminWindow) playerRow() (PlayerRow, bool) {
	i := a.players.CurrentIndex()
	if i < 0 || i >= len(a.playerModel.rows) {
		return PlayerRow{}, false
	}
	return a.playerModel.rows[i], true
}
func (a *adminWindow) updateActions() {
	row, selected := a.accountRow()
	ai := -1
	if selected {
		ai = accountRowIndex(a.state.Accounts, row.ID)
	}
	pi := a.players.CurrentIndex()
	hasAccount := a.state.CanEditAccount(ai)
	hasPlayer := a.state.CanUnpair(pi)
	a.add.SetEnabled(a.state.Connected)
	a.edit.SetEnabled(hasAccount)
	a.reauth.SetEnabled(hasAccount)
	a.remove.SetEnabled(a.state.CanRemoveAccount(ai))
	removeTip := "Remove the selected unpaired account from the pool."
	if hasAccount {
		if row.State == "In use" {
			removeTip = "Disconnect this account's player before removing it."
		} else if row.CurrentPlayer != "" {
			removeTip = "Unpair " + row.CurrentPlayer + " in Console Users before removing this account."
		}
	}
	a.remove.SetToolTipText(removeTip)
	a.unpair.SetEnabled(hasPlayer)
	a.reassign.SetEnabled(a.state.CanReassign(pi, ai))
	a.setRole.SetEnabled(a.state.Connected && pi >= 0 && pi < len(a.playerModel.rows))
}
func (a *adminWindow) addAccount() {
	person, email, ok := labelsDialog(a.mw, "Add Microsoft account", "", "")
	if ok {
		a.beginEnrollment("account.enroll.begin", adminapi.EnrollmentParams{PersonLabel: person, EmailLabel: email}, "", person)
	}
}
func (a *adminWindow) reauthenticate() {
	row, ok := a.accountRow()
	if ok {
		a.beginEnrollment("account.reauthenticate", map[string]string{"account_id": row.ID}, row.Gamertag, row.PersonLabel)
	}
}
func (a *adminWindow) beginEnrollment(method string, params any, expectedGamertag, label string) {
	a.appendEvent("Starting Microsoft sign-in for " + label)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		var begin enrollmentView
		if err := a.client.Call(ctx, method, params, &begin); err != nil {
			a.sync(func() {
				a.appendEvent(err.Error())
				walk.MsgBox(a.mw, "Cannot start Microsoft sign-in", err.Error(), walk.MsgBoxIconError)
			})
			return
		}
		// Synchronize queues work; it does not wait for a modal dialog to close.
		// Wait explicitly so cancellation/errors are respected before completing.
		dialogDone := make(chan error, 1)
		a.sync(func() { dialogDone <- showSignInDialog(a.mw, begin, expectedGamertag) })
		select {
		case dialogErr := <-dialogDone:
			if dialogErr != nil {
				a.sync(func() { a.appendEvent("Microsoft sign-in cancelled or closed: " + dialogErr.Error()) })
				return
			}
		case <-ctx.Done():
			return
		}
		a.sync(func() { a.appendEvent("Checking Microsoft sign-in and Xbox identity for " + label + "...") })
		err := a.client.Call(ctx, "account.enroll.complete", adminapi.EnrollmentCompleteParams{EnrollmentID: begin.EnrollmentID}, nil)
		a.sync(func() {
			if err != nil {
				a.appendEvent(err.Error())
				walk.MsgBox(a.mw, "Account sign-in failed", err.Error()+"\r\n\r\nFor reauthentication, use the same Microsoft/Xbox account shown in the sign-in window. The existing account and player assignment have been retained.", walk.MsgBoxIconError)
			} else {
				a.appendEvent("Account sign-in completed")
				walk.MsgBox(a.mw, "Account sign-in completed", "Microsoft and Xbox sign-in completed for "+label+".", walk.MsgBoxIconInformation)
				a.refresh()
			}
		})
	}()
}
func (a *adminWindow) editLabels() {
	row, ok := a.accountRow()
	if !ok {
		return
	}
	person, email, ok := labelsDialog(a.mw, "Edit account labels", row.PersonLabel, row.EmailLabel)
	if ok {
		a.call("account.labels.update", adminapi.AccountLabelsParams{AccountID: row.ID, PersonLabel: person, EmailLabel: email}, "Labels updated")
	}
}
func (a *adminWindow) removeAccount() {
	row, ok := a.accountRow()
	if !ok {
		return
	}
	if walk.MsgBox(a.mw, "Remove account", "Remove "+row.PersonLabel+" from the account pool?", walk.MsgBoxYesNo|walk.MsgBoxIconWarning) == walk.DlgCmdYes {
		a.call("account.remove", map[string]string{"account_id": row.ID}, "Account removed")
	}
}
func (a *adminWindow) reassignPlayer() {
	player, pok := a.playerRow()
	account, aok := a.accountRow()
	if !pok || !aok {
		return
	}
	params := adminapi.ReassignParams{ConsoleXUID: player.XUID, AccountID: account.ID}
	ctx, cancel := timeoutContext()
	defer cancel()
	var preview any
	if err := a.client.Call(ctx, "player.reassign.preview", params, &preview); err != nil {
		walk.MsgBox(a.mw, "Cannot reassign", err.Error(), walk.MsgBoxIconError)
		return
	}
	warning := "Assign " + player.Gamertag + " to " + account.PersonLabel + " (Xbox: " + account.Gamertag + ")?\r\n\r\nBDS will treat the player as this Xbox identity. Inventory, location, permissions and Ender Chest contents may change."
	if walk.MsgBox(a.mw, "Confirm account assignment", warning, walk.MsgBoxYesNo|walk.MsgBoxIconWarning) == walk.DlgCmdYes {
		a.call("player.reassign.commit", params, "Player assignment updated")
	}
}
func (a *adminWindow) unpairPlayer() {
	player, ok := a.playerRow()
	if !ok {
		return
	}
	if walk.MsgBox(a.mw, "Unpair console user", "Remove the saved account assignment for "+player.Gamertag+"?", walk.MsgBoxYesNo|walk.MsgBoxIconWarning) == walk.DlgCmdYes {
		a.call("player.unpair", adminapi.PlayerParams{ConsoleXUID: player.XUID}, "Player unpaired")
	}
}
func (a *adminWindow) setPlayerRole() {
	player, ok := a.playerRow()
	if !ok {
		return
	}
	role, accepted := playerRoleDialog(a.mw, player.Gamertag, player.Role)
	if !accepted {
		return
	}
	warning := fmt.Sprintf("Set %s to %s on every configured server?\r\n\r\nThe role takes effect after the player reconnects; a server restart may be required.", player.Gamertag, strings.Title(role))
	if walk.MsgBox(a.mw, "Set player role", warning, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
		return
	}
	a.call("player.role.set", struct {
		XUID string `json:"xuid"`
		Role string `json:"role"`
	}{player.XUID, role}, "Role updated for "+player.Gamertag+" on all servers")
}

func playerRoleDialog(owner walk.Form, gamertag, current string) (string, bool) {
	var dlg *walk.Dialog
	var roles *walk.ComboBox
	var accept *walk.PushButton
	values := []string{"Default", "Visitor", "Member", "Operator"}
	selected, accepted := "", false
	_, err := (themedDialog{AssignTo: &dlg, Title: "Player Role - " + gamertag, DefaultButton: &accept, MinSize: Size{Width: 420, Height: 160}, Layout: Grid{Columns: 2}, Children: []Widget{
		Label{Text: "Role on all servers:"}, ComboBox{AssignTo: &roles, Model: values, CurrentIndex: stringIndex(values, current)},
		Label{Text: "Default removes the explicit role and uses each server's default.", ColumnSpan: 2},
		Composite{ColumnSpan: 2, Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &accept, Text: "OK", OnClicked: func() { selected, accepted = strings.ToLower(values[roles.CurrentIndex()]), true; dlg.Accept() }}, PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}},
	}}).Run(owner)
	if err != nil {
		return "", false
	}
	return selected, accepted
}
func (a *adminWindow) call(method string, params any, success string) {
	go func() {
		// Mutations such as importing large addons may legitimately take minutes.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		err := a.client.Call(ctx, method, params, nil)
		a.sync(func() {
			if err != nil {
				a.appendEvent(err.Error())
			} else {
				a.appendEvent(success)
				a.refresh()
				// Account-side Online access changes also update the Online ACL table.
				// Reload it here so the Creative/Survival ticks are never left stale.
				if a.online != nil && a.online.loaded {
					a.online.load()
				}
			}
		})
	}()
}
func labelsDialog(owner walk.Form, title, person, email string) (string, string, bool) {
	var dlg *walk.Dialog
	var accept *walk.PushButton
	var personEdit, emailEdit *walk.LineEdit
	var labels dialogLabels
	accepted := false
	_, err := (themedDialog{AssignTo: &dlg, Title: title, DefaultButton: &accept, MinSize: Size{Width: 440, Height: 180}, Layout: Grid{Columns: 2}, Children: []Widget{Label{Text: "Person label:"}, LineEdit{AssignTo: &personEdit, Text: person}, Label{Text: "Email label (optional):"}, LineEdit{AssignTo: &emailEdit, Text: email}, Composite{ColumnSpan: 2, Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &accept, Text: "OK", OnClicked: func() {
		var ok bool
		labels, ok = acceptedLabels(personEdit.Text(), emailEdit.Text())
		if !ok {
			walk.MsgBox(dlg, title, "Person label is required.", walk.MsgBoxIconWarning)
			return
		}
		accepted = true
		dlg.Accept()
	}}, PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}}}}).Run(owner)
	return labels.person, labels.email, err == nil && accepted
}

type dialogLabels struct{ person, email string }

func acceptedLabels(person, email string) (dialogLabels, bool) {
	labels := dialogLabels{person: strings.TrimSpace(person), email: strings.TrimSpace(email)}
	return labels, labels.person != ""
}

func signInClipboardText(uri, code string) string {
	return "Microsoft sign-in link: " + uri + "\r\nCode: " + code
}

func showSignInDialog(owner walk.Form, enrollment enrollmentView, expectedGamertag string) error {
	var dlg *walk.Dialog
	var done *walk.PushButton
	openURI := enrollment.VerificationURIComplete
	if openURI == "" {
		openURI = enrollment.VerificationURI
	}
	copyText := func(text string) {
		if err := walk.Clipboard().SetText(text); err != nil {
			walk.MsgBox(dlg, "Copy failed", err.Error(), walk.MsgBoxIconError)
		}
	}
	children := []Widget{
		Label{Text: "Sign in with Microsoft, then return here and click I've signed in."},
		Label{Text: "Sign-in link:"},
		Composite{Layout: HBox{}, Children: []Widget{
			LineEdit{Text: enrollment.VerificationURI, ReadOnly: true, StretchFactor: 1},
			PushButton{Text: "Copy link", OnClicked: func() { copyText(enrollment.VerificationURI) }},
		}},
		Label{Text: "Code:"},
		Composite{Layout: HBox{}, Children: []Widget{
			LineEdit{Text: enrollment.UserCode, ReadOnly: true, StretchFactor: 1},
			PushButton{Text: "Copy code", OnClicked: func() { copyText(enrollment.UserCode) }},
		}},
	}
	if expectedGamertag != "" {
		children = append(children, Label{Text: "Use the same Xbox account: " + expectedGamertag})
	}
	children = append(children,
		Composite{Layout: HBox{}, Children: []Widget{
			PushButton{Text: "Open sign-in page", OnClicked: func() {
				if err := exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", openURI).Start(); err != nil {
					walk.MsgBox(dlg, "Open sign-in page", err.Error(), walk.MsgBoxIconError)
				}
			}},
			PushButton{Text: "Copy both", OnClicked: func() { copyText(signInClipboardText(enrollment.VerificationURI, enrollment.UserCode)) }},
			HSpacer{},
			PushButton{AssignTo: &done, Text: "I've signed in", OnClicked: func() { dlg.Accept() }},
		}},
	)
	result, err := (themedDialog{
		AssignTo:      &dlg,
		Title:         "Microsoft sign-in",
		DefaultButton: &done,
		MinSize:       Size{Width: 600, Height: 260},
		Layout:        VBox{},
		Children:      children,
	}).Run(owner)
	if err == nil && result != walk.DlgCmdOK {
		return context.Canceled
	}
	return err
}
