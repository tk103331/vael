package main

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"vael/internal/workspace"
)

// Splitting the editor: two panes, each showing a file of its own, as an
// IDE's editor does.

// splitEditor splits the editor in two, or puts it back as one.
func (a *app) splitEditor() {
	if a.split {
		a.split = false
		a.splitPath = ""
		a.focusPane = 0
		return
	}
	if len(a.tabs) == 0 {
		return
	}
	a.split = true
	if a.splitShare <= 0 || a.splitShare >= 100 {
		a.splitShare = 50
	}
	// The right pane takes the file that was open before, if it still is,
	// or the next tab, so that the two panes show different files.
	a.splitPath = ""
	if a.lastActiveTab != nil && a.lastActiveTab.state != nil && a.lastActiveTab.path != a.tabs[a.selected].path {
		for _, f := range a.tabs {
			if f.path == a.lastActiveTab.path && f.diff == nil {
				a.splitPath = f.path
				break
			}
		}
	}
	if a.splitPath == "" && len(a.tabs) > 1 {
		a.splitPath = a.tabs[(a.selected+1)%len(a.tabs)].path
	}
	if a.splitPath == "" {
		a.splitPath = a.tabs[a.selected].path
	}
	a.focusPane = 0
}

// selectedTab is the file the left pane shows.
func (a *app) selectedTab() *fileTab {
	if a.selected < 0 || a.selected >= len(a.tabs) {
		return nil
	}
	return a.tabs[a.selected]
}

// splitIndex is the tab the right pane shows, -1 when it shows none.
func (a *app) splitIndex() int {
	if !a.split || a.splitPath == "" {
		return -1
	}
	fallback := -1
	for i, f := range a.tabs {
		if f.path != a.splitPath {
			continue
		}
		if f.diff == nil {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	return fallback
}

// paneTab is the file a pane shows.
func (a *app) paneTab(pane int) *fileTab {
	if pane == 1 {
		if i := a.splitIndex(); i >= 0 {
			return a.tabs[i]
		}
		return nil
	}
	return a.selectedTab()
}

// paneIndex is the tab a pane shows among the tabs, -1 when none.
func (a *app) paneIndex(pane int) int {
	if pane == 1 {
		return a.splitIndex()
	}
	return a.selected
}

// activeIndex is the tab the pane with the keyboard shows, -1 for none.
func (a *app) activeIndex() int {
	if a.split && a.focusPane == 1 {
		if i := a.splitIndex(); i >= 0 {
			return i
		}
	}
	if a.selected >= 0 && a.selected < len(a.tabs) {
		return a.selected
	}
	return -1
}

// focusTab chooses a tab in the pane that has the keyboard, as a click on
// its strip does: the left pane's choice, or the file the right pane shows.
func (a *app) focusTab(index int) {
	if index < 0 || index >= len(a.tabs) {
		return
	}
	if a.split && a.focusPane == 1 {
		a.splitPath = a.tabs[index].path
		return
	}
	a.selected = index
}

// updateFocusPane follows the keyboard: the pane whose editor has it is the
// pane the app acts on.
func (a *app) updateFocusPane() {
	if !a.split {
		a.focusPane = 0
		return
	}
	for pane := 0; pane <= 1; pane++ {
		f := a.paneTab(pane)
		if f != nil && f.state != nil && f.state.Focused() {
			a.focusPane = pane
			return
		}
	}
}

// takeSymbol puts the caret on a declaration a quick open named, once the
// file of it is open.
func (a *app) takeSymbol(path string, tab *fileTab) {
	if a.pendingSymbol == "" || tab == nil || tab.state == nil {
		return
	}
	at := strings.Index(a.pendingSymbol, "|")
	if at < 0 {
		a.pendingSymbol = ""
		return
	}
	if a.pendingSymbol[:at] != path {
		return
	}
	name := a.pendingSymbol[at+1:]
	a.pendingSymbol = ""
	for _, symbol := range workspace.SymbolsInFile(tab.path, tab.state.Buffer.String()) {
		if strings.EqualFold(symbol.Name, name) {
			tab.state.GoTo(editor.Pos{Line: symbol.Line - 1, Col: symbol.Column - 1})
			return
		}
	}
}

// emptyPaneView is what a split pane shows without a file.
func (a *app) emptyPaneView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Grow(1).Center().Gap(10).Children(func() {
		ui.Text(c, "这一栏还没有文件").FontSize(13).TextColor(p.muted)
		if ui.Button(c, "在右栏打开…").Clicked() {
			a.focusPaneOn(1)
			a.quickQuery = ""
			a.quickOpen = true
		}
	})
}

// focusPaneOn gives the keyboard to a pane, so that what opens next opens
// there.
func (a *app) focusPaneOn(pane int) {
	a.focusPane = pane
}

// splitIcon is two panes side by side.
var splitIcon = lineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><line x1="12" y1="4" x2="12" y2="20"/>`)
