package main

import (
	"strconv"

	"github.com/egoist/mygo/ui"
)

// panelPosition is where a panel stands in the window.
type panelPosition int

const (
	panelLeft panelPosition = iota
	panelRight
	panelBottom
)

// panel is a tool window of the editor: its icon and name, where it
// stands, the title of the bar above it, and what it draws. The bars of
// the window, and the shell that holds the panel, read these, so that a
// panel is one value and not a case of switches scattered about.
type panel struct {
	// ID is what the state and a saved session name the panel by.
	ID string
	// Name is what the panel is called in the window.
	Name string
	// Tip is what the button of the panel's bar says on hover, and to
	// assistive technology.
	Tip string
	// Title is what the bar above the panel says, when it draws one of
	// its own.
	Title string
	Icon  *ui.SVG
	// Position is where the panel stands: left, right or bottom.
	Position panelPosition
	// Hidden marks a panel no bar shows, which the command palette opens.
	Hidden bool
	// Header draws the bar above the panel when it needs one of its own,
	// as the sessions of the terminal or the branch of the commit panel
	// do. A panel without one gets Title.
	Header func(a *app, c *ui.Context, p palette)
	// View draws the panel itself.
	View func(a *app, c *ui.Context, p palette)
	// Badge says what the button of the panel shows under its icon, as
	// the count of changes does, or "" for nothing.
	Badge func(a *app) string
}

// panels are the tool windows of the editor, in the order the bars show
// them.
var panels = []panel{
	{
		ID: "files", Name: "文件", Tip: "文件", Icon: filesIcon, Position: panelLeft,
		Header: (*app).filesHeader, View: (*app).filesView,
	},
	{
		ID: "commit", Name: "Commit", Tip: "Commit", Icon: commitIcon, Position: panelLeft,
		Header: (*app).gitHeader, View: (*app).gitView,
		Badge: func(a *app) string {
			if a.repo == nil || len(a.repo.Changes) == 0 {
				return ""
			}
			return strconv.Itoa(len(a.repo.Changes))
		},
	},
	{
		ID: "outline", Name: "大纲", Tip: "显示/隐藏大纲", Title: "大纲", Icon: outlineIcon,
		Position: panelRight, View: (*app).outlineView,
	},
	{
		ID: "agent", Name: "Agent", Tip: "显示/隐藏 Agent", Title: "Agent", Icon: agentIcon,
		Position: panelRight, View: (*app).agentView,
	},
	{
		// A panel of a plugin, drawn by the editor from the tree the
		// plugin asks for: hidden, and opened by the palette, as a
		// prototype of what a plugin contributes.
		ID: "plugin-demo", Name: "插件面板", Title: "插件面板（原型）",
		Position: panelRight, Hidden: true, View: (*app).pluginDemoPanel,
	},
	{
		// A panel of a plugin that really runs: the tree it asked for in
		// its last answer.
		ID: "plugin", Name: "插件", Title: "插件",
		Position: panelRight, Hidden: true, View: (*app).pluginPanel,
	},
	{
		ID: "terminal", Name: "终端", Tip: "面板：终端", Icon: terminalIcon, Position: panelBottom,
		Header: (*app).terminalHeader, View: (*app).terminalView,
	},
	{
		ID: "git", Name: "Git 日志", Tip: "面板：Git", Title: "Git", Icon: gitIcon, Position: panelBottom,
		View: (*app).logView,
	},
	{
		ID: "problems", Name: "问题", Tip: "面板：问题", Title: "问题", Icon: alertIcon,
		Position: panelBottom, View: (*app).problemView,
		Badge: func(a *app) string { return a.problemBadge() },
	},
	// The panels no bar shows: the command palette opens them.
	{
		ID: "search", Name: "项目搜索", Hidden: true, Position: panelLeft,
		View: (*app).searchView,
	},
	{
		ID: "symbols", Name: "项目符号", Hidden: true, Position: panelLeft,
		View: (*app).symbolSearchView,
	},
	{
		ID: "tasks", Name: "任务", Hidden: true, Position: panelLeft,
		View: (*app).taskView,
	},
}

// panelAt is the panel of an ID at a position, or nil.
func panelAt(id string, position panelPosition) *panel {
	for i := range panels {
		if panels[i].ID == id && panels[i].Position == position {
			return &panels[i]
		}
	}
	return nil
}

// panelsAt are the panels a bar shows, in order.
func panelsAt(position panelPosition) []panel {
	var shown []panel
	for _, item := range panels {
		if item.Position == position && !item.Hidden {
			shown = append(shown, item)
		}
	}
	return shown
}

// defaultPanel is the panel of a position that opens when none is chosen.
func defaultPanel(position panelPosition) *panel {
	for i := range panels {
		if panels[i].Position == position && !panels[i].Hidden {
			return &panels[i]
		}
	}
	return nil
}
