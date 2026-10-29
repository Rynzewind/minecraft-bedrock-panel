package main

import "github.com/lxn/walk"

func (a *adminWindow) accountSelectionChanged(broadcaster bool) {
	if broadcaster {
		if a.broadcasters.CurrentIndex() >= 0 {
			_ = a.accounts.SetCurrentIndex(-1)
		}
	} else if a.accounts.CurrentIndex() >= 0 && a.broadcasters != nil {
		_ = a.broadcasters.SetCurrentIndex(-1)
	}
	a.updateActions()
}

func (a *adminWindow) toggleAccountSection(broadcaster bool) {
	table, button, label := a.accounts, a.helperToggle, "Helpers"
	if broadcaster {
		table, button, label = a.broadcasters, a.broadcasterToggle, "Broadcasters"
	}
	expanded := !table.Visible()
	table.SetVisible(expanded)
	if !expanded {
		_ = table.SetCurrentIndex(-1)
	}
	state := "collapsed"
	if expanded {
		state = "expanded"
	}
	button.SetText(label + " (" + state + ")")
	a.accountPool.SetMinMaxSize(walk.Size{}, walk.Size{})
	minimum := walk.CreateLayoutItemsForContainer(a.accountPool).MinSize()
	height := ((minimum.Height+2)*96 + a.accountPool.DPI() - 1) / a.accountPool.DPI()
	a.accountPool.SetMinMaxSize(walk.Size{}, walk.Size{Height: height})
	a.updateActions()
}
