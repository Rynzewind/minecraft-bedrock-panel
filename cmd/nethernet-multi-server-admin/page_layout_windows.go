package main

import (
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// A long inactive page must not enlarge the other tabs or the whole window.
func scrollableAdminPage(page TabPage, viewport ...**walk.ScrollView) TabPage {
	view := ScrollView{Layout: page.Layout, Children: page.Children}
	if len(viewport) > 0 {
		view.AssignTo = viewport[0]
	}
	page.Children = []Widget{view}
	page.Layout = VBox{MarginsZero: true}
	return page
}
