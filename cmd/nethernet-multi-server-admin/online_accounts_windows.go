package main

import (
	"context"
	"fmt"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"nethernet-multi-server/internal/accountstore"
	"nethernet-multi-server/internal/adminapi"
	"time"
)

func (a *adminWindow) addBroadcasterAccount() {
	if e := requireBroadcasterRoleCapability(a.client.Call); e != nil {
		walk.MsgBox(a.mw, "Matching helper required", e.Error(), walk.MsgBoxIconWarning)
		return
	}
	person, email, ok := labelsDialog(a.mw, "Add dedicated Xbox broadcaster account", "", "")
	if ok {
		a.beginEnrollment("account.enroll.begin", adminapi.EnrollmentParams{PersonLabel: person, EmailLabel: email, Role: accountstore.RoleBroadcaster}, "", person)
	}
}
func (a *adminWindow) setAccountRole() {
	row, ok := a.accountRow()
	if !ok {
		return
	}
	var dlg *walk.Dialog
	var role *walk.ComboBox
	roles := []string{accountstore.RoleHelper, accountstore.RoleBroadcaster, accountstore.RoleUnassigned}
	index := 0
	for i, r := range roles {
		if r == row.Role {
			index = i
		}
	}
	_, e := (themedDialog{AssignTo: &dlg, Title: "Account role — " + row.PersonLabel, MinSize: Size{Width: 460, Height: 210}, Layout: VBox{}, Children: []Widget{TextLabel{Text: "Helper accounts serve existing LAN player mappings. Broadcasters are reserved for Online and cannot be leased to LAN players. Unpair mapped players and unassign Online slots before changing roles."}, ComboBox{AssignTo: &role, Model: roles, CurrentIndex: index}, Composite{Layout: HBox{}, Children: []Widget{HSpacer{}, PushButton{Text: "Save role", OnClicked: func() {
		selected := role.CurrentIndex()
		if selected < 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		e := a.client.Call(ctx, "account.role", map[string]string{"account_id": row.ID, "role": roles[selected]}, nil)
		if e != nil {
			walk.MsgBox(dlg, "Account role", e.Error(), walk.MsgBoxIconError)
			return
		}
		dlg.Accept()
		a.refresh()
	}}, PushButton{Text: "Cancel", OnClicked: func() { dlg.Cancel() }}}}}}).Run(a.mw)
	if e != nil {
		walk.MsgBox(a.mw, "Account role", e.Error(), walk.MsgBoxIconError)
	}
}

type accountRoleCapabilities struct {
	BroadcasterRoles bool `json:"broadcaster_roles"`
}

func requireBroadcasterRoleCapability(call func(context.Context, string, any, any) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var caps accountRoleCapabilities
	if e := call(ctx, "account.capabilities", struct{}{}, &caps); e != nil || !caps.BroadcasterRoles {
		return fmt.Errorf("the connected helper does not confirm broadcaster-role support; use matching panel/helper builds before enrolling a broadcaster")
	}
	return nil
}
