package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
)

type palette struct {
	wash, card, text, muted, edge, accent, selected ui.Color
	warn, warnSoft, success, danger                 ui.Color
}

func solidIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="none">` + body + `</svg>`))
}

func lineIcon(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var filesIcon = lineIcon(`<path d="M14 2H6v16h12V6z"/><path d="M14 2v5h4M3 6v16h12"/>`)
var gitIcon = lineIcon(`<circle cx="6" cy="5" r="3"/><circle cx="6" cy="19" r="3"/><circle cx="18" cy="5" r="3"/><path d="M6 8v8M18 8c0 6-12 1-12 8"/>`)
var searchIcon = lineIcon(`<circle cx="10" cy="10" r="7"/><path d="m15 15 6 6"/>`)
var terminalIcon = lineIcon(`<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m6 9 4 3-4 3m7 0h5"/>`)
var logIcon = lineIcon(`<path d="M3 12a9 9 0 1 0 3-6.7"/><polyline points="3 4 3 9 8 9"/><polyline points="12 8 12 12 15 14"/>`)
var outlineIcon = lineIcon(`<path d="M4 6h16M4 12h10M4 18h13"/><circle cx="18" cy="12" r="2"/>`)
var plusIcon = lineIcon(`<line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/>`)
var folderIcon = lineIcon(`<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/>`)
var globeIcon = lineIcon(`<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>`)
var monitorIcon = lineIcon(`<rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/>`)
var bottomPanelIcon = lineIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="3" y1="15" x2="21" y2="15"/>`)
var rightPanelIcon = lineIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="15" y1="3" x2="15" y2="21"/>`)
var sidebarIcon = lineIcon(`<rect x="3" y="3" width="18" height="18" rx="2"/><line x1="9" y1="3" x2="9" y2="21"/>`)
var moonIcon = lineIcon(`<path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>`)
var sunIcon = lineIcon(`<circle cx="12" cy="12" r="5"/><line x1="12" y1="1" x2="12" y2="3"/><line x1="12" y1="21" x2="12" y2="23"/><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"/><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/><line x1="1" y1="12" x2="3" y2="12"/><line x1="21" y1="12" x2="23" y2="12"/><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>`)
var filePlusIcon = lineIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="12" y1="18" x2="12" y2="12"/><line x1="9" y1="15" x2="15" y2="15"/>`)
var folderPlusIcon = lineIcon(`<path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"/><line x1="12" y1="11" x2="12" y2="17"/><line x1="9" y1="14" x2="15" y2="14"/>`)
var refreshIcon = lineIcon(`<polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>`)
var wrapIcon = lineIcon(`<line x1="3" y1="5" x2="21" y2="5"/><line x1="3" y1="19" x2="21" y2="19"/><line x1="3" y1="12" x2="13" y2="12"/><line x1="13" y1="12" x2="18" y2="12"/><polyline points="16 10 18 12 16 14"/>`)
var saveIcon = lineIcon(`<path d="M19 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11l5 5v11a2 2 0 0 1-2 2z"/><polyline points="17 21 17 13 7 13 7 21"/><polyline points="7 3 7 8 15 8"/>`)
var trashIcon = lineIcon(`<polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
var xIcon = lineIcon(`<line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/>`)
var chevronDownIcon = lineIcon(`<polyline points="6 9 12 15 18 9"/>`)
var chevronRightIcon = lineIcon(`<polyline points="9 6 15 12 9 18"/>`)
var chevronLeftIcon = lineIcon(`<polyline points="15 6 9 12 15 18"/>`)
var checkIcon = lineIcon(`<polyline points="20 6 9 17 4 12"/>`)
var tagIcon = lineIcon(`<path d="M20.59 13.41l-7.17 7.17a2 2 0 0 1-2.83 0L2 12V2h10l8.59 8.59a2 2 0 0 1 0 2.82z"/><line x1="7" y1="7" x2="7.01" y2="7"/>`)
var arrowUpIcon = lineIcon(`<line x1="12" y1="19" x2="12" y2="5"/><polyline points="5 12 12 5 19 12"/>`)
var arrowDownIcon = lineIcon(`<line x1="12" y1="5" x2="12" y2="19"/><polyline points="19 12 12 19 5 12"/>`)
var starIcon = solidIcon(`<polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/>`)
var pencilIcon = lineIcon(`<path d="M17 3a2.828 2.828 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5L17 3z"/>`)
var playIcon = lineIcon(`<polygon points="6 4 20 12 6 20 6 4"/>`)
var stopIcon = lineIcon(`<rect x="6" y="6" width="12" height="12" rx="2"/>`)
var xCircleIcon = lineIcon(`<circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/>`)
var cloudDownloadIcon = lineIcon(`<polyline points="8 17 12 21 16 17"/><line x1="12" y1="12" x2="12" y2="21"/><path d="M20.88 18.09A5 5 0 0 0 18 9h-1.26A8 8 0 1 0 3 16.29"/>`)
var downloadIcon = lineIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" y1="3" x2="12" y2="15"/>`)
var uploadIcon = lineIcon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" y1="3" x2="12" y2="15"/>`)
var fileIcon = lineIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/>`)
var diffIcon = lineIcon(`<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>`)
var minimizeIcon = lineIcon(`<line x1="5" y1="12" x2="19" y2="12"/>`)
var maximizeIcon = lineIcon(`<rect x="5" y="5" width="14" height="14" rx="1"/>`)
var minusIcon = lineIcon(`<line x1="5" y1="12" x2="19" y2="12"/>`)
var gearIcon = lineIcon(`<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>`)
var repeatIcon = lineIcon(`<polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/>`)
var taskIcon = lineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><path d="m7 9 3 3-3 3m6 0h4"/>`)
var copyIcon = lineIcon(`<rect x="9" y="9" width="11" height="11" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
var branchIcon = lineIcon(`<line x1="6" y1="3" x2="6" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/>`)
var eyeIcon = lineIcon(`<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>`)
var graphIcon = lineIcon(`<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="12" r="2.5"/><path d="M6 8.5v7M8.5 6H14a2 2 0 0 1 2 2v1.5"/>`)
var commitIcon = lineIcon(`<circle cx="12" cy="12" r="3.4"/><line x1="2" y1="12" x2="8.6" y2="12"/><line x1="15.4" y1="12" x2="22" y2="12"/>`)
var codeIcon = lineIcon(`<polyline points="8 6 3 12 8 18"/><polyline points="16 6 21 12 16 18"/>`)
var dataIcon = lineIcon(`<ellipse cx="12" cy="6" rx="7" ry="3"/><path d="M5 6v6c0 1.7 3.1 3 7 3s7-1.3 7-3V6"/><path d="M5 12v6c0 1.7 3.1 3 7 3s7-1.3 7-3v-6"/>`)
var docIcon = lineIcon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="13" y2="17"/>`)
var styleIcon = lineIcon(`<circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="3"/>`)
var markupIcon = lineIcon(`<polyline points="9 6 4 12 9 18"/><polyline points="15 6 20 12 15 18"/><line x1="13" y1="4" x2="11" y2="20"/>`)
var imageIcon = lineIcon(`<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="9" cy="10" r="2"/><path d="m21 16-5-5-8 8"/>`)
var lspIcon = lineIcon(`<polyline points="3 12 7 12 10 5 14 19 17 12 21 12"/>`)
var alertIcon = lineIcon(`<path d="M12 3 2 21h20L12 3z"/><line x1="12" y1="9" x2="12" y2="14"/><line x1="12" y1="17.5" x2="12.01" y2="17.5"/>`)
var agentIcon = lineIcon(`<path d="M12 3l1.7 4.3L18 9l-4.3 1.7L12 15l-1.7-4.3L6 9l4.3-1.7z"/><path d="M18 15l.9 2.1L21 18l-2.1.9L18 21l-.9-2.1L15 18l2.1-.9z"/>`)
var historyIcon = lineIcon(`<path d="M3 12a9 9 0 1 0 3-6.7"/><polyline points="3 4 3 9 8 9"/><polyline points="12 7 12 12 16 14"/>`)
var symbolsIcon = lineIcon(`<circle cx="12" cy="12" r="3"/><path d="M12 2v3m0 14v3M2 12h3m14 0h3M5 5l2 2m10 10 2 2M19 5l-2 2M7 17l-2 2"/>`)

func (a *app) activityView(c *ui.Context, p palette) {
	// As wide as the strip of the tool window at the bottom.
	ui.Column(c).Width(34).Shrink(0).FillHeight().Padding(6, 2).Gap(4).Border(1, p.edge).Children(func() {
		for _, item := range panelsAt(panelLeft) {
			item := item
			active := a.sidebar && a.section == item.ID
			a.panelButton(c, p, item, active, func() {
				if active {
					a.sidebar = false
					return
				}
				a.sidebar = true
				a.section = item.ID
				if item.ID == "commit" {
					a.refreshGit()
				}
				if item.ID == "symbols" {
					a.loadSymbols()
				}
			})
		}
		// The language servers, at the bottom of the bar: a click opens
		// what each of them does, as an IDE's status widget does.
		ui.Spacer(c)
		running, problems := a.lspTotals()
		lsp := ui.ButtonBase(c).Size(28, 28).Radius(5).Center().Label("语言服务器")
		if a.lspMenuOpen {
			lsp.Background(p.selected)
		} else if lsp.Hovered() {
			lsp.Background(p.edge)
		}
		color := p.muted
		if problems > 0 {
			color = p.warn
		}
		lsp.Children(func() {
			ui.Column(c).Fill().Center().Gap(0).Children(func() {
				ui.Icon(c, lspIcon).Size(17, 17).TextColor(color)
				if problems > 0 {
					ui.Textf(c, "%d", problems).FontSize(8).FontWeight(700).
						TextColor(color).FontFeatures("tnum")
				}
			})
		})
		lsp.Tooltip(a.lspTooltip(running, problems))
		if lsp.Clicked() {
			a.lspMenuOpen = !a.lspMenuOpen
		}
		ui.Popover(c, lsp, &a.lspMenuOpen, func() { a.lspMenu(c, p) })
	})
}

// panelButton is a button of a bar of the window: the icon of a panel,
// what it counts, and what it says on hover.
func (a *app) panelButton(c *ui.Context, p palette, item panel, active bool, fn func()) {
	b := ui.ButtonBase(c).Size(28, 28).Radius(5).Center().Label(item.Tip)
	color := p.muted
	if active {
		b.Background(p.selected)
		color = p.accent
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Column(c).Fill().Center().Gap(0).Children(func() {
			ui.Icon(c, item.Icon).Size(17, 17).TextColor(color)
			if item.Badge != nil {
				if count := item.Badge(a); count != "" {
					ui.Text(c, count).FontSize(8).FontWeight(700).TextColor(color).FontFeatures("tnum")
				}
			}
		})
	})
	b.Tooltip(item.Tip)
	if b.Clicked() {
		fn()
	}
}

// rightBar is the bar of icons on the right of the window: the panels it
// holds, as the bar on the left holds the tools of the project.
func (a *app) rightBar(c *ui.Context, p palette) {
	// As wide as the strip of the tool window at the bottom.
	ui.Column(c).Width(34).Shrink(0).FillHeight().Padding(6, 2).Gap(4).Border(1, p.edge).Children(func() {
		for _, item := range panelsAt(panelRight) {
			item := item
			active := a.rightPanel && a.rightSection == item.ID
			a.panelButton(c, p, item, active, func() {
				if active {
					a.rightPanel = false
					return
				}
				a.rightSection = item.ID
				a.rightPanel = true
			})
		}
	})
}

// workspaceView is the editor with the tool window at the bottom, and the
// panels of the right, which stand beside them.
func (a *app) workspaceView(c *ui.Context, p palette) {
	ui.Row(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
		middle := func() {
			ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
				editor := func() { ui.Column(c).Fill().Background(p.card).Children(func() { a.editorView(c, p) }) }
				if a.showBottom && !a.zen {
					ui.SplitVerticalPercent(c, &a.editorShare, editor, func() { a.bottomView(c, p) }).Fill()
				} else {
					editor()
				}
			})
		}
		if a.zen {
			middle()
			return
		}
		if a.rightPanel {
			// The panels take their share of the room, on the right, and
			// the divider between them can be dragged.
			ui.SplitPercent(c, &a.rightShare, middle, func() { a.rightView(c, p) }).Grow(1).FillHeight().MinWidth(0)
		} else {
			middle()
		}
		a.rightBar(c, p)
	})
}

// showTerminal opens the terminal in the tool window at the bottom.
func (a *app) showTerminal() {
	a.showBottom = true
	a.bottomTab = "terminal"
}

// bottomView is the tool window at the bottom, which spans the whole width
// as an IDE's: the terminal, whose sessions sit in its title bar, and the
// Git log.
func (a *app) bottomView(c *ui.Context, p palette) {
	ui.Row(c).Fill().Background(p.card).Children(func() {
		a.bottomStrip(c, p)
		ui.Divider(c)
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			item := panelAt(a.bottomTab, panelBottom)
			if item == nil {
				item = defaultPanel(panelBottom)
			}
			if item == nil {
				return
			}
			ui.Row(c).FillWidth().Height(32).Padding(0, 6, 0, 8).AlignItems(ui.Center).
				BorderWidth(0, 0, 1, 0).BorderColor(p.edge).Children(func() {
				if item.Header != nil {
					item.Header(a, c, p)
				} else {
					ui.Text(c, item.Title).FontSize(12).FontWeight(700).TextColor(p.text).Padding(0, 10, 0, 8)
					ui.Box(c).Grow(1)
				}
				if iconButton(c, chevronDownIcon, "隐藏面板", false, p) {
					a.showBottom = false
				}
			})
			item.View(a, c, p)
		})
	})
}

// rightView is the panel of the right: the outline of the file that is
// open, the agent, or one the palette opened.
func (a *app) rightView(c *ui.Context, p palette) {
	item := panelAt(a.rightSection, panelRight)
	if item == nil {
		item = defaultPanel(panelRight)
	}
	if item == nil {
		return
	}
	ui.Column(c).Fill().Background(p.card).Children(func() {
		// The title is as tall as the bar of the tabs of the editor, so
		// that their lines meet.
		ui.Row(c).FillWidth().Height(36).Padding(0, 8, 0, 10).Gap(4).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(p.edge).Children(func() {
			ui.Text(c, item.Title).FontSize(12).FontWeight(650).TextColor(p.text).Grow(1)
			if iconButton(c, xIcon, "关闭面板", false, p) {
				a.rightPanel = false
			}
		})
		if item.Header != nil {
			item.Header(a, c, p)
		}
		item.View(a, c, p)
	})
}

// iconButton is a quiet button that shows an icon and says what it does to
// assistive technology and on hover. It reports whether it was clicked.
func iconButton(c *ui.Context, icon *ui.SVG, tip string, selected bool, p palette) bool {
	b := ui.ButtonBase(c).Size(28, 28).Radius(6).Center().Label(tip)
	if selected {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() { ui.Icon(c, icon).Size(15, 15).TextColor(p.text) })
	b.Tooltip(tip)
	return b.Clicked()
}

// smallIconButton is an icon button of the tight rows, such as a change of
// the Git panel.
func smallIconButton(c *ui.Context, icon *ui.SVG, tip string, p palette) bool {
	b := ui.ButtonBase(c).Size(22, 22).Radius(4).Center().Label(tip)
	if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() { ui.Icon(c, icon).Size(12, 12).TextColor(p.text) })
	b.Tooltip(tip)
	return b.Clicked()
}

// lightButton is a traffic light of macOS: a round, colored button, at the
// left of the self-drawn title bar.
func (a *app) lightButton(c *ui.Context, color, tip string, fn func()) {
	// The label names the light for assistive technology; the empty child
	// keeps the text from being drawn inside the dot.
	b := ui.ButtonBase(c).Size(12, 12).Radius(6).Background(ui.Hex(color)).Label(tip)
	b.Children(func() { ui.Box(c) })
	b.Tooltip(tip)
	if b.Clicked() && a.win != nil {
		fn()
	}
}

// winButton is one of the window's controls of Windows and Linux.
func (a *app) winButton(c *ui.Context, icon *ui.SVG, tip string, hover ui.Color, fn func(), p palette) {
	b := ui.ButtonBase(c).FillHeight().Padding(0, 12).Label(tip)
	if b.Hovered() {
		b.Background(hover)
	}
	b.Children(func() { ui.Icon(c, icon).Size(14, 14) })
	b.Tooltip(tip)
	if b.Clicked() && a.win != nil {
		fn()
	}
}

// windowControls draws the window's controls, for the window has no title
// bar of its own: the traffic lights on macOS, and minimize, maximize and
// close elsewhere. The close button turns red while hovered, as Windows'
// does.
func (a *app) windowControls(c *ui.Context, p palette) {
	if runtime.GOOS == "darwin" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			a.lightButton(c, "#ff5f57", "关闭窗口", func() { a.win.Close() })
			a.lightButton(c, "#febc2e", "最小化窗口", func() { a.win.Minimize() })
			a.lightButton(c, "#28c840", "最大化窗口", func() { a.win.ToggleMaximize() })
		})
		return
	}
	a.winButton(c, minimizeIcon, "最小化窗口", p.edge, func() { a.win.Minimize() }, p)
	a.winButton(c, maximizeIcon, "最大化窗口", p.edge, func() { a.win.ToggleMaximize() }, p)
	a.winButton(c, xIcon, "关闭窗口", p.danger, func() { a.win.Close() }, p)
}

// editorTab is one tab of the editor: the file it shows, and the button
// that closes it. The tab of the file that is open is filled, the others
// are plain until the pointer is over them.
func (a *app) editorTab(c *ui.Context, p palette, pane, i int, tab *fileTab) {
	active := i == a.paneIndex(pane)
	name := filepath.Base(tab.path)
	b := ui.ButtonBase(c).Height(28).Padding(0, 6).Radius(6).Gap(5).Label(name)
	if active {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		fg := p.muted
		if active {
			fg = p.text
		}
		if tab.diff != nil {
			ui.Icon(c, diffIcon).Size(12, 12).TextColor(fg)
		} else {
			ui.Icon(c, fileIcon).Size(12, 12).TextColor(fg)
		}
		ui.Text(c, name).FontSize(12).TextColor(p.text).MaxWidth(180).SingleLine().Ellipsis("…")
		if tab.state != nil && tab.state.Buffer.Modified() {
			ui.Text(c, "●").FontSize(8).TextColor(p.accent)
		}
		close := ui.ButtonBase(c).Size(16, 16).Radius(4).Center().Label("关闭 " + name)
		if close.Hovered() {
			close.Background(p.edge)
		}
		close.Children(func() {
			ui.Icon(c, xIcon).Size(9, 9).TextColor(p.muted)
		})
		if close.Clicked() {
			a.closeTab(i)
		}
	})
	// The tooltip says where the file is.
	b.Tooltip(tab.path)
	if b.Clicked() {
		a.focusPaneOn(pane)
		a.focusTab(i)
	}
	tabMenu(c, a, tab, i, b)
}

func (a *app) colors() palette {
	return a.theme().Palette
}
func quiet(c *ui.Context, text string, selected bool, p palette) bool {
	return quietRow(c, text, selected, p).Clicked()
}

// quietRow is the row quiet builds, so that a caller can give it a context
// menu of its own.
func quietRow(c *ui.Context, text string, selected bool, p palette) ui.Element {
	b := ui.ButtonBase(c).Height(28).Padding(0, 9).Radius(6).Center().Label(text)
	if selected {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() { ui.Text(c, text).FontSize(12).TextColor(p.text).SingleLine() })
	return b
}
func (a *app) view(c *ui.Context) {
	p := a.colors()
	c.SetTheme(a.editorTheme())
	ui.Column(c).Fill().Background(p.wash).Children(func() {
		ui.Row(c).FillWidth().Height(36).AlignItems(ui.Center).Gap(8).Padding(0, 0, 0, 12).Children(func() {
			mac := runtime.GOOS == "darwin"
			if mac {
				a.windowControls(c, p)
				ui.Box(c).Width(6)
			}
			ui.Text(c, "V").FontSize(18).FontWeight(750).TextColor(p.accent).DragWindow()
			ui.Text(c, "VAEL").FontSize(12).FontWeight(700).LetterSpacing(2).TextColor(p.text).DragWindow()
			ui.Text(c, "/").FontSize(12).TextColor(p.muted).DragWindow()
			// The environment comes before the project: the project is
			// opened in the environment the editor runs in.
			envName, envIcon := "连接环境", globeIcon
			if a.connection != nil {
				envName = a.connection.Target
				if envName == "" {
					envName = strings.ToUpper(a.connection.Kind)
				}
				envIcon = monitorIcon
			}
			env := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Label("环境：" + envName)
			if env.Hovered() {
				env.Background(p.edge)
			}
			env.Children(func() {
				ui.Icon(c, envIcon).Size(13, 13).TextColor(p.muted)
				ui.Text(c, envName).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
				ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
			})
			env.Tooltip("工作环境")
			if env.Clicked() {
				a.envMenuOpen = !a.envMenuOpen
			}
			ui.Popover(c, env, &a.envMenuOpen, func() { a.environmentMenu(c, p) })
			project := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(5).Gap(6).AlignItems(ui.Center).Label("项目：" + filepath.Base(a.root))
			if project.Hovered() {
				project.Background(p.edge)
			}
			project.Children(func() {
				name := filepath.Base(a.root)
				if a.root == "" {
					name = "打开项目"
				}
				if a.root != "" {
					a.projectBadgeView(c, p, a.root)
				} else {
					ui.Icon(c, folderIcon).Size(13, 13).TextColor(p.muted)
				}
				ui.Text(c, name).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
				ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
			})
			project.Tooltip(a.root)
			if project.Clicked() {
				a.projectMenuOpen = !a.projectMenuOpen
			}
			ui.Popover(c, project, &a.projectMenuOpen, func() { a.projectMenu(c, p) })
			if a.repo != nil {
				branch := ui.ButtonBase(c).Height(24).Padding(0, 6).Radius(5).Gap(5).AlignItems(ui.Center).Label("分支：" + a.repo.Branch)
				if branch.Hovered() {
					branch.Background(p.edge)
				}
				branch.Children(func() {
					ui.Icon(c, gitIcon).Size(12, 12).TextColor(p.muted)
					ui.Text(c, a.repo.Branch).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					ui.Icon(c, chevronDownIcon).Size(9, 9).TextColor(p.muted)
				})
				branch.Tooltip("切换分支与 Git 操作")
				if branch.Clicked() {
					a.branchMenuOpen = !a.branchMenuOpen
					if a.branchMenuOpen {
						a.loadBranches()
					}
				}
				ui.Popover(c, branch, &a.branchMenuOpen, func() { a.branchMenu(c, p) })
			}
			ui.Box(c).Grow(1).FillHeight().DragWindow()
			if iconButton(c, searchIcon, "快速打开文件…", false, p) {
				a.quickQuery = ""
				a.quickOpen = true
			}
			if iconButton(c, folderIcon, "打开项目", false, p) {
				a.chooseProject()
			}
			if iconButton(c, sidebarIcon, "显示/隐藏侧栏", a.sidebar, p) {
				a.sidebar = !a.sidebar
			}
			if iconButton(c, bottomPanelIcon, "显示/隐藏底部面板", a.showBottom, p) {
				a.showBottom = !a.showBottom
			}
			if iconButton(c, rightPanelIcon, "显示/隐藏结构面板", a.rightPanel, p) {
				a.rightPanel = !a.rightPanel
			}
			if iconButton(c, gearIcon, "设置", false, p) {
				a.settingsOpen = true
			}
			themeTip := "切换到深色"
			themeIcon := moonIcon
			if a.dark {
				themeTip, themeIcon = "切换到浅色", sunIcon
			}
			if iconButton(c, themeIcon, themeTip, false, p) {
				a.toggleDark()
			}
			if !mac {
				ui.Box(c).Width(2)
				a.windowControls(c, p)
			}
		})
		ui.Divider(c)
		ui.Row(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			if !a.zen {
				a.activityView(c, p)
			}
			if a.sidebar && !a.zen {
				ui.Split(c, &a.sideWidth, func() { a.sidebarView(c, p) }, func() { a.workspaceView(c, p) }).Grow(1).FillHeight().MinWidth(0)
			} else {
				a.workspaceView(c, p)
			}
		})
		if !a.zen {
			ui.Divider(c)
			ui.Row(c).FillWidth().Height(24).Padding(0, 10).Gap(12).AlignItems(ui.Center).Children(func() {
				status := "本地"
				if a.connection != nil {
					status = "已连接 · " + strings.ToUpper(a.connection.Kind)
				}
				if a.disconnected {
					status = "连接已断开"
					if !a.reconnectOff {
						status += " · 自动重连中"
						if a.reconnectAttempt > 1 {
							status += fmt.Sprintf("（第 %d 次）", a.reconnectAttempt)
						}
					}
				}
				if a.connecting {
					status = "正在连接…"
				}
				stateLabel := ui.Text(c, status).FontSize(11).TextColor(p.muted)
				if a.connectionError != "" {
					stateLabel.Tooltip(a.connectionError)
				}
				if a.connection != nil && !a.connecting && a.disconnected {
					if iconButton(c, refreshIcon, "重新连接", false, p) {
						a.reconnect()
					}
					if !a.reconnectOff {
						if iconButton(c, xIcon, "停止重连", false, p) {
							a.stopReconnect()
						}
					}
				}
				if a.connecting {
					if iconButton(c, xIcon, "取消连接", false, p) {
						a.cancelConnection()
					}
				}
				if a.task.running {
					if quiet(c, "任务运行中…", false, p) {
						a.sidebar = true
						a.section = "tasks"
					}
				}
				branch := "无 Git 仓库"
				if a.repo != nil {
					branch = "⑂ " + a.repo.Branch + fmt.Sprintf("  ·  %d 项变更", len(a.repo.Changes))
				}
				ui.Text(c, branch).FontSize(11).TextColor(p.muted)
				// What the language server of the file says, and a way to
				// its settings.
				if text := a.lspStatus(); text != "" {
					label := ui.Text(c, text).FontSize(11).TextColor(p.muted)
					if problems := a.lspProblemText(); problems != "" {
						label.Tooltip(problems)
					}
					if a.lspProblems() > 0 {
						label.Cursor(ui.CursorPointer)
					}
				}
				if f := a.active(); f != nil {
					path := f.path
					if rel, err := a.relativePath(a.root, path); err == nil {
						path = rel
					}
					ui.Text(c, "› "+path).FontSize(11).TextColor(p.muted).SingleLine().Ellipsis("…")
				}
				ui.Spacer(c)
				if f := a.active(); f != nil && f.state != nil {
					pos := f.state.CursorPos()
					ui.Textf(c, "行 %d，列 %d", pos.Line+1, pos.Col+1).FontSize(11).TextColor(p.muted).FontFeatures("tnum")
					if selection := []rune(f.state.SelectedText()); len(selection) > 0 {
						ui.Textf(c, "选中 %d 字符", len(selection)).FontSize(11).TextColor(p.muted).FontFeatures("tnum")
					}
					state := "已保存"
					if f.state.Buffer.Modified() {
						state = "未保存"
					}
					if f.saving {
						state = "正在保存…"
					}
					ui.Text(c, state).FontSize(11).TextColor(p.muted)
					ui.Text(c, "UTF-8").FontSize(11).TextColor(p.muted)
					a.indentWidget(c, p, f)
					a.lineEndingWidget(c, p, f)
					ui.Text(c, strings.TrimPrefix(filepath.Ext(f.path), ".")).FontSize(11).TextColor(p.muted)
				}
			})
		}
	})
	a.workspaceSymbolPicker(c, p)
	a.actionView(c, p)
	a.referenceView(c, p)
	a.symbolPromptView(c, p)
	a.dialogs(c, p)
	a.projectPromptView(c, p)
	a.clonePromptView(c, p)
	a.branchPromptView(c, p)
	a.revisionPromptView(c, p)
	a.settingsView(c, p)
	a.quickOpenView(c, p)
	a.recentView(c, p)
	a.paletteView(c, p)
}

// sidebarView is the panel of the left the bar chose: the files of the
// project, the commits, or one the palette opened.
func (a *app) sidebarView(c *ui.Context, p palette) {
	item := panelAt(a.section, panelLeft)
	if item == nil {
		item = defaultPanel(panelLeft)
	}
	if item == nil {
		return
	}
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		if item.Header != nil {
			item.Header(a, c, p)
		}
		item.View(a, c, p)
	})
}

// filesHeader is the bar of the files panel: what the tree holds, and the
// ways to add to it.
func (a *app) filesHeader(c *ui.Context, p palette) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, strings.ToUpper(filepath.Base(a.root))).FontSize(10.5).FontWeight(650).TextColor(p.muted).Grow(1)
		if iconButton(c, refreshIcon, "刷新文件树与仓库", false, p) {
			a.refreshIndex()
			a.loadDir(a.root)
			a.refreshGit()
			for path := range a.expanded {
				if path != a.root && a.expanded[path] {
					a.loadDir(path)
				}
			}
		}
		if iconButton(c, filePlusIcon, "新建文件", false, p) {
			a.prompt = &namePrompt{}
		}
		if iconButton(c, folderPlusIcon, "新建目录", false, p) {
			a.prompt = &namePrompt{directory: true}
		}
	})
}

// filesView is the files panel: the filter and the tree of the project.
func (a *app) filesView(c *ui.Context, p palette) {
	ui.TextInput(c, &a.filter).FillWidth().Placeholder("筛选已加载的文件…")
	ui.ScrollBoth(c).FillWidth().Grow(1).TrackScroll(&a.treeScroll).Children(func() {
		// The tree takes the room its widest row needs, and the scroll
		// shows the part the panel cannot. A grid of one track keeps
		// that width, which the rows fill.
		width := max(a.sideWidth-16, a.treeWidth(c, a.root, 0))
		ui.Grid(c).ColumnTracks(ui.Fixed(width)).Gap(2).Children(func() {
			a.tree(c, p, a.root, 0, width)
		})
	})
}

func (a *app) treeWidth(c *ui.Context, dir string, depth int) float32 {
	if depth > 40 {
		return 0
	}
	width := float32(0)
	for _, e := range a.entries[dir] {
		if !e.Dir && a.filter != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(a.filter)) {
			continue
		}
		name, _ := c.MeasureText(0, ui.Span{Text: e.Name, Size: 12})
		// The indent, the name, the icons before it, and a little room:
		// the name is shown whole, and the scroll shows the part of a
		// long one the panel cannot.
		width = max(width, float32(8+depth*13)+name+44)
		if e.Dir && a.expanded[e.Path] {
			width = max(width, a.treeWidth(c, e.Path, depth+1))
		}
	}
	return width
}

func (a *app) tree(c *ui.Context, p palette, dir string, depth int, width float32) {
	if depth > 40 {
		return
	}
	if a.loading[dir] {
		ui.Text(c, "加载中…").FontSize(11).TextColor(p.muted)
	}
	for _, e := range a.entries[dir] {
		if !e.Dir && a.filter != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(a.filter)) {
			continue
		}
		e := e
		left := float32(8 + depth*13)
		row := ui.ButtonBase(c).Width(width).Height(28).Radius(6).
			Padding(0, 8, 0, left).AlignItems(ui.Center).Label(e.Name)
		active := a.active()
		if active != nil && active.path == e.Path {
			row.Background(p.selected)
		} else if row.Hovered() {
			row.Background(p.edge)
		}
		row.Children(func() {
			if e.Dir {
				chevron := chevronRightIcon
				if a.expanded[e.Path] {
					chevron = chevronDownIcon
				}
				ui.Icon(c, chevron).Size(10, 10).TextColor(p.muted)
				ui.Icon(c, folderIcon).Size(13, 13).TextColor(p.accent)
				ui.Text(c, e.Name).FontSize(12).TextColor(p.text).Grow(1).SingleLine()
			} else {
				nameColor, tip := a.treeColor(e.Path, p)
				ui.Box(c).Width(22).Children(func() {
					a.markView(c, p, a.markFor(e.Name))
				})
				name := ui.Text(c, e.Name).FontSize(12).TextColor(nameColor).Grow(1).SingleLine()
				if tip != "" {
					name.Tooltip(e.Name + " · " + tip)
				}
			}
		})
		row.ContextMenu(func(m *ui.Menu) {
			if m.Item("打开").Chosen() {
				if e.Dir {
					a.expanded[e.Path] = true
					a.loadDir(e.Path)
				} else {
					a.openFile(e.Path)
				}
			}
			if m.Item("重命名…").Chosen() {
				a.renameTarget, a.renameValue = e.Path, e.Name
			}
			if m.Item("创建副本").Chosen() {
				a.duplicateEntry(e.Path)
			}
			m.Separator()
			if m.Item("删除").Chosen() {
				a.deleteTarget = e.Path
			}
			m.Separator()
			if m.Item("复制路径").Chosen() {
				mygo.Clipboard.WriteText(e.Path)
			}
		})
		if row.Clicked() {
			if e.Dir {
				a.expanded[e.Path] = !a.expanded[e.Path]
				if a.expanded[e.Path] {
					a.loadDir(e.Path)
				}
			} else {
				a.openFile(e.Path)
			}
		}
		if e.Dir && a.expanded[e.Path] {
			a.tree(c, p, e.Path, depth+1, width)
		}
	}
}
func (a *app) editorView(c *ui.Context, p palette) {
	a.updateFocusPane()
	if !a.split || a.zen {
		a.editorPane(c, p, 0)
		return
	}
	ui.SplitPercent(c, &a.splitShare,
		func() { a.editorPane(c, p, 0) },
		func() { a.editorPane(c, p, 1) }).Fill()
}

// editorPane is one of the editor's panes: its strip of tabs, the place of
// the file, and the file.
func (a *app) editorPane(c *ui.Context, p palette, pane int) {
	ui.Column(c).Fill().MinWidth(0).Children(func() {
		a.editorTabStrip(c, p, pane)
		if !a.zen {
			a.breadcrumbOf(c, p, a.paneTab(pane))
		}
		f := a.paneTab(pane)
		if f == nil {
			if pane == 1 {
				a.emptyPaneView(c, p)
			} else {
				a.welcomeView(c, p)
			}
			return
		}
		if note, ok := a.stale[f.path]; ok {
			ui.Row(c).FillWidth().Padding(7, 10).Gap(8).Background(p.warnSoft).AlignItems(ui.Center).Children(func() {
				ui.Text(c, note).FontSize(11.5).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
				if quiet(c, "重新加载", false, p) {
					a.reloadFile(f)
				}
				if quiet(c, "保留我的修改", false, p) {
					delete(a.stale, f.path)
				}
			})
		}
		if f.diff != nil {
			editor.DiffView(c, f.diff).FillWidth().Grow(1)
			return
		}
		a.autoSaveOnChange(f)
		a.styleEditor(f.state)
		a.lspEditor(f)
		ui.Row(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			view := editor.View(c, f.state).FillHeight().Grow(1).MinWidth(0)
			if pane == a.focusPane {
				view.AutoFocus()
			}
			if a.minimap {
				ui.Divider(c)
				a.minimapView(c, p, f)
			}
		})
	})
}

// editorTabStrip is a pane's bar of tabs, with the actions of the file the
// pane shows.
func (a *app) editorTabStrip(c *ui.Context, p palette, pane int) {
	tabCount := len(a.tabs)
	ui.Row(c).FillWidth().Height(36).Shrink(0).AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).BorderColor(p.edge).Children(func() {
		ui.ScrollHorizontal(c).FillHeight().Grow(1).Shrink(1).Children(func() {
			ui.Row(c).Height(36).Padding(2, 4).Gap(2).AlignItems(ui.Center).Children(func() {
				for i, f := range a.tabs {
					a.editorTab(c, p, pane, i, f)
				}
				if tabCount > 1 {
					overflow := ui.ButtonBase(c).Size(24, 24).Radius(6).Center().Label("全部标签页")
					overflow.Children(func() { ui.Icon(c, chevronDownIcon).Size(12, 12).TextColor(p.muted) })
					overflow.Tooltip("全部标签页")
					overflow.Menu(func(m *ui.Menu) {
						for i, tab := range a.tabs {
							name := filepath.Base(tab.path)
							if m.Item(name).Chosen() {
								a.focusPaneOn(pane)
								a.focusTab(i)
							}
						}
					})
				}
			})
		})
		ui.Row(c).Padding(0, 6).Gap(2).AlignItems(ui.Center).Children(func() {
			if f := a.paneTab(pane); f != nil && f.state != nil {
				if iconButton(c, searchIcon, "查找与替换", false, p) {
					f.state.Run(editorFind)
				}
				if iconButton(c, wrapIcon, "自动换行", a.wrap, p) {
					a.wrap = !a.wrap
				}
				if iconButton(c, saveIcon, "保存", false, p) {
					a.save()
				}
			}
			if pane == 1 {
				if iconButton(c, xIcon, "取消拆分", false, p) {
					a.splitEditor()
				}
			} else if iconButton(c, splitIcon, "拆分编辑器", a.split, p) {
				a.splitEditor()
			}
		})
	})
}

// welcomeView is what the editor shows without a file.
func (a *app) welcomeView(c *ui.Context, p palette) {
	ui.Column(c).FillWidth().Grow(1).Center().Gap(14).Children(func() {
		ui.Text(c, "Vael").FontSize(38).FontWeight(700).TextColor(p.text)
		ui.Text(c, "从一个文件，开始下一次创造。").FontSize(14).TextColor(p.muted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "打开项目").Clicked() {
				a.chooseProject()
			}
			if ui.Button(c, "新建文件").Clicked() {
				a.prompt = &namePrompt{}
			}
			if ui.Button(c, "启动终端").Clicked() {
				a.newTerminal()
			}
		})
		ui.Text(c, "原生编辑器  /  文件树  /  Git  /  终端").FontSize(11).TextColor(p.muted)
		if len(a.environments.Recent) > 0 {
			ui.Text(c, "最近项目").FontSize(12).FontWeight(650).TextColor(p.muted).Padding(12, 0, 0, 0)
			ui.Column(c).Width(480).Gap(2).Children(func() {
				for i, recent := range a.environments.Recent {
					if i >= 6 {
						break
					}
					recent := recent
					a.environmentRow(c, p, recent)
				}
			})
		}
	})
}

// styleEditor puts the settings of the editor on a file's state, so that
// a change in them shows at once in every tab.
func (a *app) styleEditor(st *editor.State) {
	st.Font = a.editorFont()
	st.SoftWrap = a.wrap
	st.TabSize = a.tabSize
	st.Indent = editor.IndentStyle(a.indent)
	st.Whitespace = editor.Whitespace(a.whitespace)
	st.LineHeight = a.editorLineHeight()
	st.NoIndentGuides = !a.guides
	st.NoAutoClose = !a.autoClose
	if a.numbers {
		st.LineNumbers = editor.LineNumbersOn
	} else {
		st.LineNumbers = editor.LineNumbersOff
	}
}

// bottomStrip is the bar on the left of the tool window at the bottom: its
// own buttons choose the panel, as an IDE's stripe.
func (a *app) bottomStrip(c *ui.Context, p palette) {
	ui.Column(c).Width(34).Shrink(0).FillHeight().Padding(6, 2).Gap(2).Children(func() {
		for _, item := range panelsAt(panelBottom) {
			item := item
			active := a.bottomTab == item.ID
			a.panelButton(c, p, item, active, func() {
				a.showBottom = true
				a.bottomTab = item.ID
			})
		}
	})
}

// terminalHeader is the title bar of the terminal: the sessions, a button
// for a new one, and the list of all of them.
func (a *app) terminalHeader(c *ui.Context, p palette) {
	ui.Text(c, "终端").FontSize(12).FontWeight(700).TextColor(p.text).Padding(0, 10, 0, 8)
	for i, t := range a.terms {
		a.terminalTab(c, p, i, t)
	}
	if iconButton(c, plusIcon, "新建终端", false, p) {
		a.newTerminal()
	}
	if len(a.terms) > 0 {
		sessions := ui.ButtonBase(c).Size(24, 24).Radius(4).Center().Label("终端会话")
		if a.termMenuOpen {
			sessions.Background(p.selected)
		} else if sessions.Hovered() {
			sessions.Background(p.edge)
		}
		sessions.Children(func() {
			ui.Icon(c, chevronDownIcon).Size(10, 10).TextColor(p.muted)
		})
		if sessions.Clicked() {
			a.termMenuOpen = !a.termMenuOpen
		}
		ui.Popover(c, sessions, &a.termMenuOpen, func() {
			ui.Column(c).Width(200).Padding(4).Gap(1).Children(func() {
				for i, t := range a.terms {
					i, t := i, t
					a.logOption(c, p, t.name, i == a.termSelected, func() {
						a.termSelected = i
						a.termMenuOpen = false
					})
				}
				ui.Divider(c)
				a.logOption(c, p, "结束当前终端", false, func() {
					a.termMenuOpen = false
					a.closeTerminal(a.termSelected)
				})
			})
		})
	}
	ui.Box(c).Grow(1)
}

func (a *app) terminalView(c *ui.Context, p palette) {
	ui.Column(c).Fill().Children(func() {
		if a.termSelected >= 0 && a.termSelected < len(a.terms) {
			terminal.View(c, a.terms[a.termSelected].term).FillWidth().Grow(1)
		} else {
			ui.Column(c).FillWidth().Grow(1).Center().Children(func() {
				if ui.Button(c, "启动项目终端").Clicked() {
					a.newTerminal()
				}
			})
		}
	})
}

// terminalTab is a session of the terminal: its name, and a close button
// that shows on the chosen tab and on hover.
func (a *app) terminalTab(c *ui.Context, p palette, index int, t *shellTab) {
	selected := index == a.termSelected
	group := ui.Row(c).Height(32).Padding(0, 2).AlignItems(ui.Center)
	if selected {
		group.BorderWidth(0, 0, 2, 0).BorderColor(p.accent)
	}
	group.Children(func() {
		name := ui.ButtonBase(c).Height(30).Padding(0, 8).Radius(4).AlignItems(ui.Center).Label(t.name)
		if !selected && name.Hovered() {
			name.Background(p.edge)
		}
		name.Children(func() {
			fg := p.muted
			if selected {
				fg = p.text
			}
			ui.Text(c, t.name).FontSize(11.5).TextColor(fg).SingleLine()
		})
		if name.Clicked() {
			a.termSelected = index
		}
		if selected || group.Hovered() {
			close := ui.ButtonBase(c).Size(18, 30).Radius(4).Center().Label("结束 " + t.name)
			if close.Hovered() {
				close.Background(p.edge)
			}
			close.Children(func() {
				ui.Icon(c, xIcon).Size(9, 9).TextColor(p.muted)
			})
			if close.Clicked() {
				a.closeTerminal(index)
			}
		}
	})
}

// closeTerminal ends a session of the terminal and takes its tab away.
func (a *app) closeTerminal(index int) {
	if index < 0 || index >= len(a.terms) {
		return
	}
	a.terms[index].term.Close()
	a.terms = append(a.terms[:index], a.terms[index+1:]...)
	if a.termSelected >= len(a.terms) {
		a.termSelected = len(a.terms) - 1
	}
	if a.termSelected < 0 && len(a.terms) > 0 {
		a.termSelected = 0
	}
}

// gitSectionCount is the title of a section of the Git panel, with how
// many things it holds.
func (a *app) gitSectionCount(c *ui.Context, p palette, title string, count int) {
	ui.Row(c).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(650).TextColor(p.text).Grow(1)
		if count > 0 {
			ui.Textf(c, "%d", count).FontSize(10.5).TextColor(p.muted).FontFeatures("tnum")
		}
	})
}

// changeCount is how many changes a section of the panel holds.
func (a *app) changeCount(staged bool) int {
	count := 0
	if a.repo == nil {
		return 0
	}
	for _, change := range a.repo.Changes {
		if staged && change.Staged() || !staged && change.Unstaged() {
			count++
		}
	}
	return count
}

// gitHeader is the bar of the commit panel: the branch the repository is
// on, and the ways its commits travel.
func (a *app) gitHeader(c *ui.Context, p palette) {
	if a.repo == nil || a.refreshing {
		return
	}
	if len(a.branches) == 0 && !a.branchesLoading {
		a.loadBranches()
	}
	ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
		if quiet(c, "⑂ "+a.repo.Branch, false, p) {
			a.branchOpen = !a.branchOpen
			if a.branchOpen {
				a.loadBranches()
			}
		}
		ui.Spacer(c)
		if !a.gitBusy && iconButton(c, cloudDownloadIcon, "从所有远端获取", false, p) {
			a.gitRemote("fetch", "--all")
		}
		if !a.gitBusy && iconButton(c, downloadIcon, "拉取", false, p) {
			a.gitRemote("pull", "--ff-only")
		}
		if !a.gitBusy && iconButton(c, uploadIcon, "推送", false, p) {
			a.gitRemote("push")
		}
	})
	if a.branchOpen {
		ui.Scroll(c).FillWidth().MaxHeight(160).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				for _, branch := range a.branches {
					branch := branch
					row := ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 6).Radius(4).Label(branch)
					if branch == a.repo.Branch {
						row.Background(p.selected)
					} else if row.Hovered() {
						row.Background(p.edge)
					}
					row.Children(func() {
						ui.Text(c, branch).FontSize(12).TextColor(p.text).SingleLine().Ellipsis("…")
					})
					if row.Clicked() {
						a.branchOpen = false
						a.switchBranch(branch)
					}
				}
				if len(a.branches) == 0 {
					ui.Text(c, "正在读取分支…").FontSize(11).TextColor(p.muted)
				}
			})
		})
	}
}

// gitView is the commit panel: the message of the next commit, the
// changes of the working tree, and the commits already made.
func (a *app) gitView(c *ui.Context, p palette) {
	if a.refreshing {
		ui.Text(c, "正在读取仓库…").FontSize(12).TextColor(p.muted)
	}
	if a.repo == nil {
		ui.Text(c, "当前文件夹没有可用的 Git 仓库。").FontSize(12).TextColor(p.muted)
		if a.gitError != "" {
			ui.Text(c, a.gitError).FontSize(11).TextColor(p.muted)
		}
		return
	}
	root := a.repo.Root
	backend := a.backend
	ctx := a.projectCtx
	if len(a.commits) == 0 && !a.commitsLoading {
		a.loadCommits()
	}
	commitInput := ui.TextInput(c, &a.commit).FillWidth().Height(36).Padding(3, 8).Radius(6).
		Placeholder("提交信息…")
	if ui.PrimaryButton(c, "提交已暂存的更改").FillWidth().Height(36).Radius(6).Disabled(a.gitBusy || strings.TrimSpace(a.commit) == "").Clicked() || commitInput.Submitted() && strings.TrimSpace(a.commit) != "" {
		message := a.commit
		a.gitAction(func() error { _, err := backend.Git(ctx, root, "commit", "-m", message); return err })
	}
	ui.Scroll(c).FillWidth().Grow(1).Children(func() {
		ui.Column(c).FillWidth().Gap(12).Children(func() {
			for _, staged := range []bool{true, false} {
				title := "工作区更改"
				if staged {
					title = "已暂存的更改"
				}
				a.gitSectionCount(c, p, title, a.changeCount(staged))
				count := 0
				for _, change := range a.repo.Changes {
					change := change
					if staged && !change.Staged() || !staged && !change.Unstaged() {
						continue
					}
					count++
					ui.Row(c).FillWidth().Gap(3).AlignItems(ui.Center).Children(func() {
						b := ui.ButtonBase(c).Grow(1).MinWidth(0).Height(30).Padding(0, 6).Radius(5).Label(change.Path)
						if b.Hovered() {
							b.Background(p.edge)
						}
						b.Children(func() {
							ui.Text(c, change.Code+"  "+change.Path).FontSize(11).TextColor(p.text).SingleLine().Ellipsis("…")
						})
						if b.Clicked() {
							a.openDiff(change, staged)
						}
						label := "+"
						if staged {
							label = "−"
						}
						if !a.gitBusy && quiet(c, label, false, p) {
							a.gitAction(func() error {
								if staged {
									return backend.Unstage(ctx, root, change)
								}
								return backend.Stage(ctx, root, change)
							})
						}
						if !staged && !a.gitBusy && quiet(c, "↺", false, p) {
							discarded := change
							a.confirm = &confirmation{"放弃更改？", "“" + discarded.Path + "”在工作区中的修改将被丢弃，无法撤销。", func() { a.discardChange(discarded) }}
						}
						if quiet(c, "↗", false, p) {
							a.openFile(a.joinPath(root, change.Path))
						}
					})
				}
				if count == 0 {
					ui.Text(c, "没有更改").FontSize(11.5).TextColor(p.muted).Padding(0, 2)
				}
			}
			a.gitSectionCount(c, p, "提交历史", 0)
			if a.commitsLoading {
				ui.Text(c, "正在读取提交…").FontSize(11.5).TextColor(p.muted).Padding(0, 2)
			}
			for _, line := range a.commits {
				line := line
				hash, subject := line, ""
				if i := strings.Index(line, " "); i > 0 {
					hash, subject = line[:i], strings.TrimSpace(line[i+1:])
				}
				ui.Row(c).FillWidth().Height(24).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, hash).Font("monospace").FontSize(10.5).TextColor(p.accent).Width(56).Shrink(0)
					ui.Text(c, subject).FontSize(11.5).TextColor(p.text).Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				})
			}
			if len(a.commits) == 0 && !a.commitsLoading {
				ui.Text(c, "没有提交").FontSize(11.5).TextColor(p.muted).Padding(0, 2)
			}
		})
	})
}
func (a *app) dialogs(c *ui.Context, p palette) {
	a.connectionDialog(c, p)
	if a.prompt != nil {
		open := true
		prompt := a.prompt
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(400).Padding(22).Gap(16).Children(func() {
				title := "新建文件"
				if prompt.directory {
					title = "新建目录"
				}
				ui.Text(c, title).FontSize(17).FontWeight(650)
				ui.Text(c, "在项目根目录创建").FontSize(12).TextColor(p.muted)
				input := ui.TextInput(c, &prompt.name).FillWidth().Placeholder("名称").AutoFocus()
				submitted := input.Submitted()
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Spacer(c)
					if ui.Button(c, "取消").Clicked() {
						open = false
					}
					if ui.PrimaryButton(c, "创建").Clicked() || submitted {
						a.create()
					}
				})
			})
		})
		if !open {
			a.prompt = nil
		}
	}
	if a.branchFrom != "" {
		open := true
		from := a.branchFrom
		short := from
		if len(short) > 8 {
			short = short[:8]
		}
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(400).Padding(22).Gap(16).Children(func() {
				ui.Text(c, "创建分支").FontSize(17).FontWeight(650)
				ui.Text(c, "从提交 "+short+" 创建").FontSize(12).TextColor(p.muted)
				input := ui.TextInput(c, &a.branchName).FillWidth().Placeholder("分支名称").AutoFocus()
				submitted := input.Submitted()
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Spacer(c)
					if ui.Button(c, "取消").Clicked() {
						open = false
					}
					if ui.PrimaryButton(c, "创建").Clicked() || submitted {
						name := strings.TrimSpace(a.branchName)
						a.branchFrom = ""
						open = false
						if name != "" {
							a.createBranchFrom(name, from)
						}
					}
				})
			})
		})
		if !open {
			a.branchFrom = ""
		}
	}
	if a.deleteTarget != "" {
		path := a.deleteTarget
		a.deleteTarget = ""
		name := filepath.Base(path)
		a.confirm = &confirmation{"删除“" + name + "”？", "此操作会永久删除该文件或目录，无法撤销。", func() { a.deleteEntry(path) }}
	}
	if a.renameTarget != "" {
		open := true
		target := a.renameTarget
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(420).Padding(22).Gap(16).Children(func() {
				ui.Text(c, "重命名").FontSize(17).FontWeight(650)
				ui.Text(c, target).FontSize(11).TextColor(p.muted)
				input := ui.TextInput(c, &a.renameValue).FillWidth().Placeholder("新名称").AutoFocus()
				submitted := input.Submitted()
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Spacer(c)
					if ui.Button(c, "取消").Clicked() {
						open = false
					}
					if ui.PrimaryButton(c, "重命名").Clicked() || submitted {
						name := a.renameValue
						a.renameTarget = ""
						open = false
						a.renameEntry(target, name)
					}
				})
			})
		})
		if !open {
			a.renameTarget = ""
			a.renameValue = ""
		}
	}
	if a.confirm != nil {
		open := true
		confirm := a.confirm
		ui.Modal(c, &open, func() {
			ui.Column(c).Width(420).Padding(22).Gap(16).Children(func() {
				ui.Text(c, confirm.title).FontSize(17).FontWeight(650)
				ui.Text(c, confirm.message).FontSize(13).TextColor(p.muted)
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					ui.Spacer(c)
					if ui.Button(c, "取消").Clicked() {
						open = false
					}
					if ui.PrimaryButton(c, "继续").Clicked() {
						a.confirm = nil
						confirm.action()
						open = false
					}
				})
			})
		})
		if !open {
			a.confirm = nil
		}
	}
	if a.message != "" {
		kind := a.toastKind()
		title := "提示"
		switch kind {
		case "success":
			title = "成功"
		case "error":
			title = "错误"
		}
		c.AddToast(ui.Toast{Title: title, Description: a.message, Type: kind})
		a.message, a.messageKind = "", ""
	}
}
