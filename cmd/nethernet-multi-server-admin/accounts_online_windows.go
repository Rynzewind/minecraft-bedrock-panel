package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func (a *adminWindow) grantSelectedAccountOnlineAccess() {
	row, ok := a.accountRow()
	if !ok {
		a.appendEvent("Select an account first.")
		return
	}
	if row.XUID == "" || row.Gamertag == "" {
		a.appendEvent("That account must be authenticated before Online access can be granted.")
		return
	}
	var dlg *walk.Dialog
	var creative, survival *walk.CheckBox
	var save *walk.PushButton
	accepted := false
	_, err := (themedDialog{AssignTo: &dlg, Title: "Online access - " + row.PersonLabel, DefaultButton: &save, MinSize: Size{Width: 440, Height: 190}, Layout: VBox{}, Children: []Widget{
		TextLabel{Text: "Grant this authenticated Xbox account access through the Friends gateway:"},
		CheckBox{AssignTo: &creative, Text: "Creative connection"}, CheckBox{AssignTo: &survival, Text: "Survival connection"},
		Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{AssignTo: &save, Text: "Save access", OnClicked: func() {
			if !creative.Checked() && !survival.Checked() {
				walk.MsgBox(dlg, "Online access", "Choose Creative, Survival, or both.", walk.MsgBoxIconWarning)
				return
			}
			accepted = true
			dlg.Accept()
		}}, PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}},
	}}).Run(a.mw)
	if err != nil || !accepted {
		return
	}
	servers := []string{}
	if creative.Checked() {
		servers = append(servers, "creative")
	}
	if survival.Checked() {
		servers = append(servers, "survival")
	}
	a.call("online.players.grant_account", map[string]any{"account_id": row.ID, "servers": servers}, "Online access saved for "+row.PersonLabel+".")
}
