package main

import (
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Themes: the colours of the window, and how files are marked in it. A
// theme is a value the settings choose, so that the editor can look like
// an IDE's dark theme, a light one, or a palette of its own.

// uiTheme is a theme of the window: its name, the family it belongs to and
// whether it is dark, the colours of its parts, and how files are marked
// in it.
type uiTheme struct {
	ID       string
	Name     string
	Family   string
	Dark     bool
	Palette  palette
	Icons    string
	IconName string
}

// uiThemes are the themes the settings offer.
var uiThemes = []uiTheme{
	{
		ID: "vael-dark", Name: "Vael 深色", Family: "vael", Dark: true,
		Palette: paletteOf("#141418", "#232328", "#ececf0", "#a9a9b4", "#34343d", "#6b93ff", "#30394f", "#f0a33a", "#4fd17a", "#ff6961"),
	},
	{
		ID: "vael-light", Name: "Vael 浅色", Family: "vael", Dark: false,
		Palette: paletteOf("#eceef4", "#ffffff", "#1d1d1f", "#6e6e78", "#e1e3eb", "#3b6cf6", "#e7edff", "#c27b0f", "#2fa84f", "#d93a35"),
	},
	{
		ID: "one-dark", Name: "One Dark", Family: "one", Dark: true,
		Palette: paletteOf("#21252b", "#282c34", "#abb2bf", "#7f848e", "#3b4048", "#61afef", "#31445c", "#e5c07b", "#98c379", "#e06c75"),
	},
	{
		ID: "nord", Name: "Nord", Family: "nord", Dark: true,
		Palette: paletteOf("#2e3440", "#3b4252", "#eceff4", "#a3aab8", "#434c5e", "#88c0d0", "#4c566a", "#ebcb8b", "#a3be8c", "#bf616a"),
	},
	{
		ID: "solarized-dark", Name: "Solarized 深色", Family: "solarized", Dark: true,
		Palette: paletteOf("#002b36", "#073642", "#93a1a1", "#657b83", "#0e4b59", "#268bd2", "#12505f", "#b58900", "#859900", "#dc322f"),
	},
	{
		ID: "solarized-light", Name: "Solarized 浅色", Family: "solarized", Dark: false,
		Palette: paletteOf("#fdf6e3", "#f7f1df", "#586e75", "#93a1a1", "#e4dcc8", "#268bd2", "#ece6d2", "#b58900", "#859900", "#dc322f"),
	},
	{
		ID: "github-dark", Name: "GitHub 深色", Family: "github", Dark: true,
		Palette: paletteOf("#0d1117", "#161b22", "#c9d1d9", "#8b949e", "#30363d", "#58a6ff", "#1f3352", "#d29922", "#3fb950", "#f85149"),
	},
	{
		ID: "github-light", Name: "GitHub 浅色", Family: "github", Dark: false,
		Palette: paletteOf("#f6f8fa", "#ffffff", "#1f2328", "#656d76", "#d0d7de", "#0969da", "#ddf0ff", "#9a6700", "#1a7f37", "#cf222e"),
	},
}

// paletteOf builds the colours of a theme from the colours of its parts:
// the surfaces, the words, the lines, the accent, and what means what.
func paletteOf(wash, card, text, muted, edge, accent, selected, warn, success, danger string) palette {
	return palette{
		wash: ui.Hex(wash), card: ui.Hex(card), text: ui.Hex(text), muted: ui.Hex(muted),
		edge: ui.Hex(edge), accent: ui.Hex(accent), selected: ui.Hex(selected),
		warn: ui.Hex(warn), warnSoft: ui.Hex(warn).Alpha(0.14),
		success: ui.Hex(success), danger: ui.Hex(danger),
	}
}

// themeByID is a theme, or the default one when the ID is unknown.
func themeByID(id string) uiTheme {
	for _, theme := range uiThemes {
		if theme.ID == id {
			return theme
		}
	}
	return uiThemes[0]
}

// themeFor is the theme of the app, whose colours every part draws with.
// A theme a plugin brought is used the same way as a built-in one.
func (a *app) theme() uiTheme {
	if theme := a.pluginTheme(a.themeID); theme != nil {
		return *theme
	}
	return themeByID(a.themeID)
}

// themeOfDark is the theme of a family that is dark, or light: what the
// theme does, and its other side. It finds the other side of a theme a
// plugin brought too.
func (a *app) themeOfDark(id string, dark bool) string {
	var want uiTheme
	if plugin := a.pluginTheme(id); plugin != nil {
		want = *plugin
	} else {
		want = themeByID(id)
	}
	for _, theme := range a.pluginThemes {
		if theme.Family == want.Family && theme.Dark == dark {
			return theme.ID
		}
	}
	for _, theme := range uiThemes {
		if theme.Family == want.Family && theme.Dark == dark {
			return theme.ID
		}
	}
	for _, theme := range uiThemes {
		if theme.Family == "vael" && theme.Dark == dark {
			return theme.ID
		}
	}
	return uiThemes[0].ID
}

// setTheme changes the theme of the window, and says whether the editor is
// dark now, which its own theme follows.
func (a *app) setTheme(id string) {
	if a.pluginTheme(id) == nil {
		id = themeByID(id).ID
	}
	a.themeID = id
	a.dark = a.theme().Dark
}

// toggleDark switches between the dark and light side of the same family,
// as Cmd+Shift+L does.
func (a *app) toggleDark() { a.setTheme(a.themeOfDark(a.themeID, !a.dark)) }

// editorTheme is the theme the editor, the differences and the terminal
// draw with: the palettes of the window, on the editor's own theme.
func (a *app) editorTheme() *ui.Theme {
	theme := ui.LightTheme()
	if a.dark {
		theme = ui.DarkTheme()
	}
	p := a.colors()
	theme.Background, theme.Surface, theme.Accent, theme.Text = p.card, p.card, p.accent, p.text
	return theme
}

// iconThemes are the ways the tree and the tabs mark a file: its badges,
// and their colours.
var iconThemes = []struct {
	ID   string
	Name string
}{
	{"badges", "文字徽标"},
	{"glyphs", "图形徽标"},
}

// mark is how a file is marked: its words in a badge, or an icon.
type mark struct {
	text  string
	icon  *ui.SVG
	color func(p palette) ui.Color
}

// iconThemeFor is the icon theme of the app.
func (a *app) iconThemeFor() string {
	if a.pluginIconTheme() != nil {
		return a.iconID
	}
	for _, theme := range iconThemes {
		if theme.ID == a.iconID {
			return theme.ID
		}
	}
	return iconThemes[0].ID
}

// markFor is the mark of a file, by its name, in the icon theme of the
// app.
func (a *app) markFor(name string) mark {
	// A plugin's icon theme says how its kinds of files are marked.
	if item, ok := a.pluginMark(name); ok {
		return item
	}
	badge := glyphFor(name)
	if a.iconThemeFor() != "glyphs" {
		return mark{text: badge.text, color: badge.color}
	}
	icon, color := glyphIcon(strings.ToLower(filepathExt(name)))
	if icon == nil {
		return mark{text: badge.text, color: badge.color}
	}
	return mark{icon: icon, color: color}
}

// glyphIcon is the icon of a kind of file, for the theme that draws icons:
// what it is, not what it is called.
func glyphIcon(ext string) (*ui.SVG, func(p palette) ui.Color) {
	switch ext {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".c", ".h", ".cc", ".cpp",
		".java", ".rb", ".php", ".swift", ".kt", ".scala", ".lua":
		return codeIcon, func(p palette) ui.Color { return p.accent }
	case ".json", ".yaml", ".yml", ".toml", ".sql", ".csv":
		return dataIcon, func(p palette) ui.Color { return p.success }
	case ".md", ".txt", ".rst", ".adoc":
		return docIcon, func(p palette) ui.Color { return p.muted }
	case ".sh", ".bash", ".zsh", ".fish", ".ps1":
		return terminalIcon, func(p palette) ui.Color { return p.muted }
	case ".css", ".scss", ".less":
		return styleIcon, func(p palette) ui.Color { return p.warn }
	case ".html", ".htm", ".vue", ".svelte", ".xml":
		return markupIcon, func(p palette) ui.Color { return p.warn }
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".webp", ".ico":
		return imageIcon, func(p palette) ui.Color { return p.accent }
	}
	return nil, nil
}

// filepathExt is the extension of a name, in lower case.
func filepathExt(name string) string { return strings.ToLower(path.Ext(name)) }

// markView draws the mark of a file in the room a badge has: its words, or
// its icon.
func (a *app) markView(c *ui.Context, p palette, item mark) {
	if item.icon != nil {
		ui.Icon(c, item.icon).Size(13, 13).TextColor(item.color(p))
		return
	}
	ui.Text(c, item.text).FontSize(8.5).FontWeight(700).TextColor(item.color(p)).
		FontFeatures("tnum")
}

// themeSwatch is a theme of the settings: its own colours, as a chip that
// chooses it.
func (a *app) themeSwatch(c *ui.Context, p palette, theme uiTheme) {
	chosen := a.themeID == theme.ID
	chip := ui.ButtonBase(c).Height(30).Padding(0, 8).Radius(6).Gap(6).
		AlignItems(ui.Center).Label("主题：" + theme.Name)
	if chosen {
		chip.Border(1, p.accent)
	} else if chip.Hovered() {
		chip.Background(p.edge)
	}
	chip.Children(func() {
		// A dot of the theme's accent on its own surface, so that a theme
		// looks like itself before it is chosen.
		ui.Box(c).Size(12, 12).Radius(6).Background(theme.Palette.card).
			Border(2, theme.Palette.accent)
		ui.Text(c, theme.Name).FontSize(11).TextColor(p.text).
			SingleLine().Ellipsis("…")
	})
	chip.Tooltip(theme.Name)
	if chip.Clicked() {
		a.setTheme(theme.ID)
	}
}
