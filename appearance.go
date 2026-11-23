package main

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// DefaultFontSize is the size, in DIPs, of the editor's and the terminal's
// text before the user zooms.
const DefaultFontSize = 13

// editorFont is the font of the text the editor, the differences and the
// terminal draw: the family the settings chose, and its size.
func (a *app) editorFont() ui.Font {
	return ui.Font{Family: a.fontFamilyName(), Size: a.fontSize}
}

// fontFamilyName is the family the settings chose, the system's monospaced
// one when none.
func (a *app) fontFamilyName() string {
	if a.fontFamily == "" {
		return "monospace"
	}
	return a.fontFamily
}

// applyFontSize tells every open editor, diff and terminal what size the
// text is now. New ones take the size as they are made.
func (a *app) applyFontSize() {
	font := a.editorFont()
	for _, f := range a.tabs {
		if f.state != nil {
			f.state.Font = font
		}
		if f.diff != nil {
			f.diff.Font = font
		}
	}
	for _, t := range a.terms {
		t.term.SetFont(terminal.Font{Family: a.fontFamilyName(), Size: a.fontSize})
	}
}

// zoom makes the editor's and the terminal's text larger or smaller.
func (a *app) zoom(delta int) {
	if a.fontSize == 0 {
		a.fontSize = DefaultFontSize
	}
	a.fontSize = max(9, min(24, a.fontSize+float32(delta)))
	a.applyFontSize()
}

// zoomReset puts the text back at its usual size.
func (a *app) zoomReset() {
	a.fontSize = DefaultFontSize
	a.applyFontSize()
}

// nextTab and prevTab go to the tab beside the one that is open.
func (a *app) nextTab() {
	if len(a.tabs) == 0 {
		return
	}
	a.selected = (a.selected + 1) % len(a.tabs)
}

func (a *app) prevTab() {
	if len(a.tabs) == 0 {
		return
	}
	a.selected = (a.selected - 1 + len(a.tabs)) % len(a.tabs)
}

// closeOtherTabs closes every tab but one, asking first when the others
// have changes of their own.
func (a *app) closeOtherTabs(keep int) {
	if keep < 0 || keep >= len(a.tabs) {
		return
	}
	dirty := false
	for i, f := range a.tabs {
		if i != keep && f.state != nil && f.state.Buffer.Modified() {
			dirty = true
		}
	}
	do := func() {
		kept := a.tabs[keep]
		a.tabs = []*fileTab{kept}
		a.selected = 0
		a.updateWatchTargets()
	}
	if dirty {
		a.confirm = &confirmation{"关闭其他标签页？", "其他标签页中未保存的修改会丢失。", do}
	} else {
		do()
	}
}

// closeAllTabs closes every tab, asking first when any of them has changes.
func (a *app) closeAllTabs() {
	dirty := a.dirty()
	do := func() {
		a.tabs = nil
		a.selected = -1
		a.updateWatchTargets()
	}
	if dirty {
		a.confirm = &confirmation{"关闭全部标签页？", "未保存的修改会丢失。", do}
	} else {
		do()
	}
}

// tabMenu is the context menu of an editor tab: it closes the tab, the
// others, or all of them, shows the file in the tree, and copies its path.
func tabMenu(c *ui.Context, a *app, tab *fileTab, index int, row ui.Element) {
	row.ContextMenu(func(m *ui.Menu) {
		if m.Item("关闭").Chosen() {
			a.closeTab(index)
		}
		if m.Item("关闭其他").Chosen() {
			a.closeOtherTabs(index)
		}
		if m.Item("关闭全部").Chosen() {
			a.closeAllTabs()
		}
		m.Separator()
		if m.Item("在文件树中显示").Chosen() {
			a.revealInTree(tab.path)
			a.sidebar = true
			a.section = "files"
		}
		if m.Item("复制路径").Chosen() {
			mygo.Clipboard.WriteText(tab.path)
		}
	})
}

// glyph is the badge of a file: what its kind is, in two letters, and the
// color it is drawn in.
type glyph struct {
	text  string
	color func(p palette) ui.Color
}

// glyphs are the badges of the file kinds the tree shows, by extension.
var glyphs = map[string]glyph{
	".go":   {"GO", func(p palette) ui.Color { return p.accent }},
	".ts":   {"TS", func(p palette) ui.Color { return p.warn }},
	".tsx":  {"TS", func(p palette) ui.Color { return p.warn }},
	".js":   {"JS", func(p palette) ui.Color { return p.warn }},
	".jsx":  {"JS", func(p palette) ui.Color { return p.warn }},
	".py":   {"PY", func(p palette) ui.Color { return p.success }},
	".rs":   {"RS", func(p palette) ui.Color { return p.warn }},
	".md":   {"MD", func(p palette) ui.Color { return p.muted }},
	".sh":   {"SH", func(p palette) ui.Color { return p.muted }},
	".json": {"{}", func(p palette) ui.Color { return p.muted }},
	".yaml": {"{}", func(p palette) ui.Color { return p.muted }},
	".yml":  {"{}", func(p palette) ui.Color { return p.muted }},
	".toml": {"{}", func(p palette) ui.Color { return p.muted }},
	".sql":  {"SQL", func(p palette) ui.Color { return p.success }},
	".css":  {"CSS", func(p palette) ui.Color { return p.accent }},
	".html": {"<>", func(p palette) ui.Color { return p.accent }},
}

// glyphFor is the badge of a file by its name, or a faint dot for a kind
// the editor has no badge for.
func glyphFor(name string) glyph {
	if g, ok := glyphs[strings.ToLower(filepath.Ext(name))]; ok {
		return g
	}
	return glyph{text: "·", color: func(p palette) ui.Color { return p.muted }}
}

// breadcrumb is the bar under the tabs that says where the file that is
// open is, as an IDE's: each part of the path is a button that shows the
// file in the tree.
func (a *app) breadcrumb(c *ui.Context, p palette) {
	f := a.active()
	if f == nil {
		return
	}
	rel := f.path
	if r, err := a.relativePath(a.root, f.path); err == nil {
		rel = r
	}
	segments := strings.Split(rel, "/")
	ui.Row(c).FillWidth().Height(26).Padding(0, 8).Gap(2).AlignItems(ui.Center).Children(func() {
		for i, segment := range segments {
			if i > 0 {
				ui.Text(c, "›").FontSize(10).TextColor(p.muted).Padding(0, 2)
			}
			segment, last := segment, i == len(segments)-1
			color := p.muted
			if last {
				color = p.text
			}
			b := ui.ButtonBase(c).Height(20).Padding(0, 5).Radius(4).Label(segment)
			b.Children(func() { ui.Text(c, segment).FontSize(11).TextColor(color).SingleLine().Ellipsis("…") })
			b.Tooltip(segment)
			if b.Clicked() {
				a.revealInTree(f.path)
				a.sidebar = true
				a.section = "files"
			}
		}
	})
}

// fontName is what a family is called in the settings: the system's
// monospaced font has a name of its own.
func (a *app) fontName() string {
	if a.fontFamilyName() == "monospace" {
		return "默认等宽"
	}
	return a.fontFamilyName()
}

// monospacedFonts lists the families the settings offer: the monospaced
// ones, which an editor is, and the app's own if it registered some.
func (a *app) monospacedFonts() []ui.FontFamily {
	if a.fontFamilies == nil {
		all := ui.FontFamilies()
		for _, family := range all {
			if family.Monospaced || family.Registered {
				a.fontFamilies = append(a.fontFamilies, family)
			}
		}
	}
	return a.fontFamilies
}

// fontMenu is the list of the fonts the editor can take.
func (a *app) fontMenu(c *ui.Context, p palette) {
	ui.Column(c).Width(280).Padding(6).Gap(4).Children(func() {
		ui.TextInput(c, &a.fontQuery).FillWidth().Height(26).Padding(2, 6).
			Placeholder("搜索字体…").Label("搜索字体").AutoFocus()
		ui.Scroll(c).FillWidth().MaxHeight(240).Children(func() {
			ui.Column(c).FillWidth().Gap(1).Children(func() {
				query := strings.ToLower(strings.TrimSpace(a.fontQuery))
				a.fontOption(c, p, "默认等宽", "monospace", query == "" || strings.Contains("默认等宽", query))
				for _, family := range a.monospacedFonts() {
					name := family.Name
					if !strings.Contains(strings.ToLower(name), query) {
						continue
					}
					a.fontOption(c, p, name, name, true)
				}
			})
		})
	})
}

// fontOption is a font of the list, chosen or not.
func (a *app) fontOption(c *ui.Context, p palette, label, family string, shown bool) {
	if !shown {
		return
	}
	chosen := a.fontFamilyName() == family
	b := ui.ButtonBase(c).FillWidth().Height(26).Padding(0, 8).Gap(6).Radius(5).
		AlignItems(ui.Center).Label("字体：" + label)
	if chosen {
		b.Background(p.selected)
	} else if b.Hovered() {
		b.Background(p.edge)
	}
	b.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(p.text).Grow(1).SingleLine().Ellipsis("…")
		if chosen {
			ui.Icon(c, checkIcon).Size(11, 11).TextColor(p.accent)
		}
	})
	if b.Clicked() {
		a.fontMenuOpen = false
		a.fontFamily = family
		a.applyFontSize()
	}
}
